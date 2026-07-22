package deployer

import (
	"context"
	"crypto/tls"
	"crypto/x509"
	"encoding/json"
	"encoding/pem"
	"errors"
	"fmt"
	"io"
	"net/http"
	"os"
	"slices"
	"strings"
	"time"

	"github.com/stackrox/roxie/internal/k8s"
	"github.com/stackrox/roxie/internal/logger"
)

var (
	retryableSubstrings = []string{
		"connection refused",
		"connection reset",
		"connection timed out",
		"timed out",
		"timeout",
		"network is unreachable",
		"temporary failure in name resolution",
		"no route to host",
		"tls handshake timeout",
		"eof",
		"bad gateway",
		"service unavailable",
		"context deadline exceeded",
		"no such host",
	}
)

type crsGenRequest struct {
	Name string `json:"name"`
}

type crsGenResponse struct {
	CRS []byte `json:"crs"`
}

// generateCRS is a retrying wrapper on top of generateCRSOnce.
func (d *Deployer) generateCRS(ctx context.Context, clusterName string) (string, error) {
	const maxAttempts = 5
	const baseRetryDelay = 10

	client, err := d.centralHTTPClient()
	if err != nil {
		return "", fmt.Errorf("failed to create HTTP client: %w", err)
	}

	var lastErr error
	for attempt := 1; attempt <= maxAttempts; attempt++ {
		if attempt > 1 {
			waitTime := time.Duration(attempt*baseRetryDelay) * time.Second
			d.logger.Infof("Retrying CRS generation (attempt %d/%d) after %v...", attempt, maxAttempts, waitTime)
			select {
			case <-ctx.Done():
				return "", ctx.Err()
			case <-time.After(waitTime):
			}
		}

		// Include attempt in name for uniqueness, in case a failure is between addition to DB and reading response.
		crsName := fmt.Sprintf("%s-crs-%d", clusterName, attempt)

		crsContent, err := d.generateCRSOnce(ctx, client, crsName)
		if err != nil {
			if d.isRetryableError(err) {
				d.logger.Warningf("Transient error generating CRS: %v", err)
				lastErr = err
				continue
			}
			return "", fmt.Errorf("CRS generation failed with non-retryable error: %w", err)
		}

		d.logger.Success("✓ CRS generated")
		return crsContent, nil
	}

	return "", fmt.Errorf("CRS generation failed after %d attempts: %w", maxAttempts, lastErr)
}

// centralHTTPClient returns an HTTP client configured for talking to Central.
// When a CA cert file is available, it uses a narrow trust pool with a custom
// chain-only verifier (see centralVerifyFunc). Otherwise it falls back to Go's
// default TLS verification with the system root CAs and full hostname checking.
func (d *Deployer) centralHTTPClient() (*http.Client, error) {
	tlsConfig := &tls.Config{}

	if d.roxCACertFile != "" {
		pemData, err := os.ReadFile(d.roxCACertFile)
		if err != nil {
			return nil, fmt.Errorf("reading CA cert file %s: %w", d.roxCACertFile, err)
		}
		pool := x509.NewCertPool()
		var caCertsAdded int
		for block, rest := pem.Decode(pemData); block != nil; block, rest = pem.Decode(rest) {
			cert, err := x509.ParseCertificate(block.Bytes)
			if err != nil {
				return nil, fmt.Errorf("parsing CA certificate from %s: %w", d.roxCACertFile, err)
			}
			pool.AddCert(cert)
			caCertsAdded++
			d.logger.Dimf("CA cert #%d: Subject.CN=%q, Issuer.CN=%q, SubjectKeyId=%x", caCertsAdded, cert.Subject.CommonName, cert.Issuer.CommonName, cert.SubjectKeyId)
		}
		d.logger.Infof("Loaded %d CA certificate(s) from %q", caCertsAdded, d.roxCACertFile)
		tlsConfig.RootCAs = pool
		tlsConfig.InsecureSkipVerify = true
		tlsConfig.VerifyPeerCertificate = centralVerifyFunc(d.logger, tlsConfig)
	}

	return &http.Client{
		Timeout: 30 * time.Second,
		Transport: &http.Transport{
			TLSClientConfig: tlsConfig,
		},
	}, nil
}

// generateCRSOnce generates a Cluster Registration Secret via Central's REST API.
func (d *Deployer) generateCRSOnce(ctx context.Context, client *http.Client, crsName string) (string, error) {
	d.logger.Infof("Generating CRS named %q via Central API...", crsName)

	reqBody, err := json.Marshal(crsGenRequest{Name: crsName})
	if err != nil {
		return "", fmt.Errorf("failed to marshal CRS request: %w", err)
	}

	url := fmt.Sprintf("https://%s/v1/cluster-init/crs", d.centralEndpoint)

	req, err := http.NewRequestWithContext(ctx, http.MethodPost, url, strings.NewReader(string(reqBody)))
	if err != nil {
		return "", fmt.Errorf("failed to create HTTP request: %w", err)
	}
	req.Header.Set("Content-Type", "application/json")
	req.SetBasicAuth(AdminUsername, d.centralPassword)

	resp, err := client.Do(req)
	if err != nil {
		return "", fmt.Errorf("executing HTTP request: %w", err)
	}

	body, readErr := io.ReadAll(resp.Body)
	closeErr := resp.Body.Close()
	if readErr != nil {
		return "", fmt.Errorf("failed to read CRS response body: %w", readErr)
	}
	if closeErr != nil {
		return "", fmt.Errorf("failed to close CRS response body: %w", closeErr)
	}

	if resp.StatusCode != http.StatusOK {
		return "", fmt.Errorf("server returned status %s: %s", resp.Status, string(body))
	}

	var crsResp crsGenResponse
	if err := json.Unmarshal(body, &crsResp); err != nil {
		return "", fmt.Errorf("failed to parse CRS response: %w", err)
	}

	crsContent := strings.TrimSpace(string(crsResp.CRS))
	if crsContent == "" {
		return "", errors.New("CRS content is empty")
	}

	return crsContent, nil
}

func (d *Deployer) isRetryableError(err error) bool {
	errLower := strings.ToLower(err.Error())
	return slices.ContainsFunc(retryableSubstrings, func(sub string) bool {
		return strings.Contains(errLower, sub)
	})
}

// centralVerifyFunc returns a custom TLS peer certificate verifier for Central.
//
// Go's default TLS verifier is disabled (InsecureSkipVerify) because roxie
// often connects via port-forward to 127.0.0.1, which never matches any SAN.
// This function performs certificate chain verification in its place.
//
// We deliberately do not check the DNSName, because the user can configure
// custom TLS certificates for Central and in several scenarios (port-forwarding,
// GKE load balancers) roxie only has an IP address -- no hostname to check against.
//
// Why chain-only is sufficient: the trust pool is narrow and cluster-scoped. It
// contains the internal StackRox CA (from central-tls) plus, when a custom
// defaultTLSSecret is configured, all certs from that secret (including the leaf).
// Forging a certificate trusted by this pool requires the CA private key, which
// is stored in the central-tls secret on the cluster. Reading that secret requires
// kubectl access — at which point the attacker can read the admin password directly.
func centralVerifyFunc(log *logger.Logger, conf *tls.Config) func([][]byte, [][]*x509.Certificate) error {
	return func(rawCerts [][]byte, _ [][]*x509.Certificate) error {
		if len(rawCerts) == 0 {
			return errors.New("remote peer presented no certificates")
		}

		certs := make([]*x509.Certificate, 0, len(rawCerts))
		for _, rawCert := range rawCerts {
			cert, err := x509.ParseCertificate(rawCert)
			if err != nil {
				return fmt.Errorf("failed to parse peer certificate: %w", err)
			}
			certs = append(certs, cert)
		}

		leaf := certs[0]
		intermediates := x509.NewCertPool()
		for _, cert := range certs[1:] {
			intermediates.AddCert(cert)
		}

		log.Dimf("Leaf cert: Subject.CN=%q, Issuer.CN=%q, AuthorityKeyId=%x",
			leaf.Subject.CommonName, leaf.Issuer.CommonName, leaf.AuthorityKeyId)
		log.Dim("Verifying certificate chain only")

		_, err := leaf.Verify(x509.VerifyOptions{
			// We deliberately do not check the DNSName here, see comment above.
			Intermediates: intermediates,
			Roots:         conf.RootCAs,
		})
		return err
	}
}

// applyCRS applies the CRS content to the sensor namespace
func (d *Deployer) applyCRS(ctx context.Context, crsContent string) error {
	d.logger.Info("Applying CRS to sensor namespace")

	result, err := d.runKubectl(ctx, k8s.KubectlOptions{
		Args:  []string{"apply", "-n", d.config.SecuredCluster.Namespace, "-f", "-"},
		Stdin: strings.NewReader(crsContent),
	})
	if err != nil {
		d.logger.Errorf("kubectl stdout: %s", result.Stdout)
		d.logger.Errorf("kubectl stderr: %s", result.Stderr)
		return fmt.Errorf("failed to apply CRS: %w\nStderr: %s", err, result.Stderr)
	}

	d.logger.Success("✓ CRS applied")
	return nil
}
