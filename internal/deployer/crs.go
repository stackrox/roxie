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
)

type crsGenRequest struct {
	Name string `json:"name"`
}

type crsGenResponse struct {
	CRS []byte `json:"crs"`
}

// generateCRS generates a Cluster Registration Secret via Central's REST API.
func (d *Deployer) generateCRS(ctx context.Context, clusterName string) (string, error) {
	const maxAttempts = 5
	const baseRetryDelay = 10

	retryableSubstrings := []string{
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

	client, err := d.centralHTTPClient()
	if err != nil {
		return "", fmt.Errorf("failed to create HTTP client: %w", err)
	}

	url := fmt.Sprintf("https://%s/v1/cluster-init/crs", d.centralEndpoint)

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
		d.logger.Infof("Generating CRS named %q via Central API...", crsName)

		reqBody, err := json.Marshal(crsGenRequest{Name: crsName})
		if err != nil {
			return "", fmt.Errorf("failed to marshal CRS request: %w", err)
		}

		req, err := http.NewRequestWithContext(ctx, http.MethodPost, url, strings.NewReader(string(reqBody)))
		if err != nil {
			return "", fmt.Errorf("failed to create HTTP request: %w", err)
		}
		req.Header.Set("Content-Type", "application/json")
		req.SetBasicAuth(AdminUsername, d.centralPassword)

		resp, err := client.Do(req)
		if err != nil {
			lastErr = err
			errLower := strings.ToLower(err.Error())
			if attempt < maxAttempts && slices.ContainsFunc(retryableSubstrings, func(sub string) bool {
				return strings.Contains(errLower, sub)
			}) {
				d.logger.Warningf("Transient error generating CRS: %v", err)
				continue
			}
			return "", fmt.Errorf("failed to generate CRS: %w", err)
		}

		body, readErr := io.ReadAll(resp.Body)
		closeErr := resp.Body.Close()
		if readErr != nil {
			return "", fmt.Errorf("failed to read CRS response body: %w", readErr)
		}
		if closeErr != nil {
			return "", fmt.Errorf("failed to close CRS response body: %w", closeErr)
		}

		if resp.StatusCode == http.StatusBadGateway || resp.StatusCode == http.StatusServiceUnavailable {
			lastErr = fmt.Errorf("server returned %s", resp.Status)
			if attempt < maxAttempts {
				d.logger.Warningf("Transient HTTP error generating CRS: %s", resp.Status)
				continue
			}
			return "", lastErr
		}

		if resp.StatusCode != http.StatusOK {
			return "", fmt.Errorf("CRS generation failed with status %s: %s", resp.Status, string(body))
		}

		var crsResp crsGenResponse
		if err := json.Unmarshal(body, &crsResp); err != nil {
			return "", fmt.Errorf("failed to parse CRS response: %w", err)
		}

		crsContent := strings.TrimSpace(string(crsResp.CRS))
		if crsContent == "" {
			return "", errors.New("CRS content is empty")
		}

		d.logger.Success("✓ CRS generated")
		return crsContent, nil
	}

	return "", fmt.Errorf("CRS generation failed after %d attempts: %w", maxAttempts, lastErr)
}

// centralHTTPClient returns an HTTP client configured for talking to Central.
func (d *Deployer) centralHTTPClient() (*http.Client, error) {
	tlsConfig := &tls.Config{}

	if d.roxCACertFile != "" {
		pemData, err := os.ReadFile(d.roxCACertFile)
		if err != nil {
			return nil, fmt.Errorf("reading CA cert file %s: %w", d.roxCACertFile, err)
		}
		pool := x509.NewCertPool()
		for block, rest := pem.Decode(pemData); block != nil; block, rest = pem.Decode(rest) {
			cert, err := x509.ParseCertificate(block.Bytes)
			if err != nil {
				return nil, fmt.Errorf("parsing CA certificate from %s: %w", d.roxCACertFile, err)
			}
			pool.AddCert(cert)
		}
		tlsConfig.RootCAs = pool
	}

	return &http.Client{
		Timeout: 30 * time.Second,
		Transport: &http.Transport{
			TLSClientConfig: tlsConfig,
		},
	}, nil
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
