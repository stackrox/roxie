//go:build e2e

package e2e

import (
	"crypto/ecdsa"
	"crypto/elliptic"
	"crypto/rand"
	"crypto/x509"
	"crypto/x509/pkix"
	"encoding/pem"
	"math/big"
	"net"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/stretchr/testify/require"
)

func TestDeployWithCustomTLSCert(t *testing.T) {
	dumpClusterStateOnFailure(t)

	const (
		sharedNamespace = "stackrox"
		secretName      = "custom-central-tls"
	)

	caCertPEM, certChainPEM, keyPEM := generateCentralTLSCert(t)

	ensureNamespace(t, sharedNamespace)
	createTLSSecret(t, sharedNamespace, secretName, certChainPEM, keyPEM)

	envrcFile, err := os.CreateTemp(t.TempDir(), ".envrc.roxie-test-*")
	require.NoError(t, err)
	envrcPath := envrcFile.Name()
	envrcFile.Close()

	t.Log("=== Deploying both components with custom TLS certificate ===")
	args := append([]string{
		roxieBinary, "deploy", "--single-namespace", "--early-readiness", "both",
		"--exposure=none", "--port-forwarding",
		"--set", "central.spec.central.defaultTLSSecret.name=" + secretName,
		"--envrc", envrcPath,
	}, commonDeployArgs...)
	runCommand(t, deployTimeout*2, nil, args...)

	verifyCentralInstalled(t, sharedNamespace)
	verifySecuredClusterInstalled(t, sharedNamespace)

	env, err := loadEnvrcFile(envrcPath)
	require.NoError(t, err, "Failed to load envrc file")
	require.NotEmpty(t, env["ROX_CA_CERT_FILE"], "ROX_CA_CERT_FILE should be set in envrc")

	verifyCACertFileContainsCustomCA(t, env["ROX_CA_CERT_FILE"], caCertPEM)

	t.Log("=== Cleaning up ===")
	teardownArgs := []string{roxieBinary, "teardown", "--single-namespace", "--skip-user-config", "both"}
	runCommand(t, teardownTimeout, env, teardownArgs...)

	verifyCentralNotInstalled(t, sharedNamespace)
	verifySecuredClusterNotInstalled(t, sharedNamespace)
}

// generateCentralTLSCert creates a self-signed CA and a leaf certificate for
// Central's HTTPS endpoint. It returns PEM-encoded bytes: the CA certificate,
// the full certificate chain (leaf + CA), and the leaf private key.
func generateCentralTLSCert(t *testing.T) (caCertPEM, certChainPEM, keyPEM []byte) {
	t.Helper()

	caKey, err := ecdsa.GenerateKey(elliptic.P256(), rand.Reader)
	require.NoError(t, err)

	caTemplate := &x509.Certificate{
		SerialNumber: big.NewInt(1),
		Subject: pkix.Name{
			CommonName: "roxie-test-ca",
		},
		NotBefore:             time.Now().Add(-time.Hour),
		NotAfter:              time.Now().Add(time.Hour),
		KeyUsage:              x509.KeyUsageCertSign | x509.KeyUsageCRLSign,
		BasicConstraintsValid: true,
		IsCA:                  true,
	}

	caCertDER, err := x509.CreateCertificate(rand.Reader, caTemplate, caTemplate, &caKey.PublicKey, caKey)
	require.NoError(t, err)

	caCert, err := x509.ParseCertificate(caCertDER)
	require.NoError(t, err)

	leafKey, err := ecdsa.GenerateKey(elliptic.P256(), rand.Reader)
	require.NoError(t, err)

	leafTemplate := &x509.Certificate{
		SerialNumber: big.NewInt(2),
		Subject: pkix.Name{
			CommonName: "custom-central.arthur.dent",
		},
		DNSNames: []string{
			"custom-central.arthur.dent",
		},
		IPAddresses:           []net.IP{net.IPv4(127, 0, 0, 1)},
		NotBefore:             time.Now().Add(-time.Hour),
		NotAfter:              time.Now().Add(24 * time.Hour),
		KeyUsage:              x509.KeyUsageDigitalSignature,
		ExtKeyUsage:           []x509.ExtKeyUsage{x509.ExtKeyUsageServerAuth},
		BasicConstraintsValid: true,
	}

	leafCertDER, err := x509.CreateCertificate(rand.Reader, leafTemplate, caCert, &leafKey.PublicKey, caKey)
	require.NoError(t, err)

	caCertPEM = pem.EncodeToMemory(&pem.Block{Type: "CERTIFICATE", Bytes: caCertDER})
	leafCertPEM := pem.EncodeToMemory(&pem.Block{Type: "CERTIFICATE", Bytes: leafCertDER})

	leafKeyDER, err := x509.MarshalECPrivateKey(leafKey)
	require.NoError(t, err)
	keyPEM = pem.EncodeToMemory(&pem.Block{Type: "EC PRIVATE KEY", Bytes: leafKeyDER})

	certChainPEM = append(leafCertPEM, caCertPEM...)
	return caCertPEM, certChainPEM, keyPEM
}

func ensureNamespace(t *testing.T, name string) {
	t.Helper()
	cmd := exec.Command("kubectl", "create", "namespace", name)
	output, err := cmd.CombinedOutput()
	if err != nil && !strings.Contains(string(output), "already exists") {
		t.Fatalf("Failed to create namespace %s: %s", name, string(output))
	}
}

func createTLSSecret(t *testing.T, namespace, name string, certPEM, keyPEM []byte) {
	t.Helper()
	dir := t.TempDir()
	certFile := filepath.Join(dir, "tls.crt")
	keyFile := filepath.Join(dir, "tls.key")
	require.NoError(t, os.WriteFile(certFile, certPEM, 0600))
	require.NoError(t, os.WriteFile(keyFile, keyPEM, 0600))

	// Delete any leftover secret from a previous run.
	del := exec.Command("kubectl", "-n", namespace, "delete", "secret", name, "--ignore-not-found")
	del.CombinedOutput()

	cmd := exec.Command("kubectl", "-n", namespace, "create", "secret", "tls", name,
		"--cert="+certFile, "--key="+keyFile)
	output, err := cmd.CombinedOutput()
	require.NoError(t, err, "Failed to create TLS secret: %s", string(output))
}

func verifyCACertFileContainsCustomCA(t *testing.T, caCertFile string, expectedCAPEM []byte) {
	t.Helper()
	contents, err := os.ReadFile(caCertFile)
	require.NoError(t, err, "Failed to read CA cert file %s", caCertFile)
	require.Contains(t, string(contents), string(expectedCAPEM),
		"CA cert file should contain the custom CA certificate")
}
