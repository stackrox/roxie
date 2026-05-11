//go:build e2e
// +build e2e

package e2e

import (
	"os"
	"strconv"
	"strings"
	"syscall"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// TestDeployBothSimple tests deploying both components together (simplest scenario)
func TestDeployBothSimple(t *testing.T) {
	dumpClusterStateOnFailure(t)

	// Create temporary envrc file
	envrcFile, err := os.CreateTemp(t.TempDir(), ".envrc.roxie-test-*")
	if err != nil {
		t.Fatalf("Failed to create temp envrc: %v", err)
	}
	envrcPath := envrcFile.Name()
	envrcFile.Close()

	t.Log("=== Deploying both components together ===")
	args := append([]string{roxieBinary, "deploy", "--early-readiness", "both", "--envrc", envrcPath}, commonDeployArgs...)
	runCommand(t, deployTimeout*2, nil, args...)

	// Verify namespaces exist and have managed-by labels
	t.Log("Verifying namespace: acs-central")
	verifyNamespaceExists(t, "acs-central")
	verifyNamespaceHasLabel(t, "acs-central", "app.kubernetes.io/managed-by", "roxie")

	t.Log("Verifying namespace: acs-sensor")
	verifyNamespaceExists(t, "acs-sensor")
	verifyNamespaceHasLabel(t, "acs-sensor", "app.kubernetes.io/managed-by", "roxie")

	// Brief pause before cleanup
	time.Sleep(5 * time.Second)

	// Cleanup
	t.Log("=== Cleaning up ===")
	teardownArgs := []string{roxieBinary, "teardown", "both"}
	runCommand(t, teardownTimeout, nil, teardownArgs...)

	t.Log("Verifying components are removed")
	verifyCentralNotInstalled(t, "acs-central")
	verifySecuredClusterNotInstalled(t, "acs-sensor")
}

// TestDetachedPortForwarding tests the detached port-forwarding mode for central.
func TestDetachedPortForwarding(t *testing.T) {
	dumpClusterStateOnFailure(t)

	envrcFile, err := os.CreateTemp(t.TempDir(), ".envrc.roxie-test-*")
	if err != nil {
		t.Fatalf("Failed to create temp envrc: %v", err)
	}
	envrcPath := envrcFile.Name()
	envrcFile.Close()

	t.Log("=== Deploying central without exposure and with port-forwarding and envrc ===")
	args := append([]string{roxieBinary, "deploy", "--early-readiness", "central", "--exposure=none", "--port-forwarding", "--envrc", envrcPath}, commonDeployArgs...)
	runCommand(t, deployTimeout, nil, args...)

	env, err := loadEnvrcFile(envrcPath)
	require.NoError(t, err, "Failed to load envrc file")
	pidStr, ok := env["ROXIE_PORT_FORWARD_PID"]
	require.True(t, ok, "ROXIE_PORT_FORWARD_PID not set in envrc")
	pid, err := strconv.Atoi(pidStr)
	require.NoError(t, err, "ROXIE_PORT_FORWARD_PID is not a valid integer: %s", pidStr)
	t.Logf("Port-forward PID: %d", pid)

	require.NoError(t, syscall.Kill(pid, 0), "Port-forward process (PID %d) does not exist", pid)

	endpoint, ok := env["API_ENDPOINT"]
	require.True(t, ok, "API_ENDPOINT not set in envrc")
	require.True(t, strings.HasPrefix(endpoint, "127.0.0.1:"),
		"Expected localhost endpoint, got: %s", endpoint)

	caCertFile, ok := env["ROX_CA_CERT_FILE"]
	require.True(t, ok, "ROX_CA_CERT_FILE not set in envrc")

	testCentralAPI(t, endpoint, caCertFile)

	t.Log("=== Cleaning up ===")
	teardownArgs := []string{roxieBinary, "teardown", "central"}
	runCommand(t, teardownTimeout, env, teardownArgs...)

	assert.Eventually(t, func() bool {
		return syscall.Kill(pid, 0) != nil
	}, 10*time.Second, 200*time.Millisecond, "Port-forward process (PID %d) should not exist after teardown", pid)
}
