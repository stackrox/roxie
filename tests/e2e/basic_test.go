//go:build e2e
// +build e2e

package e2e

import (
	"os"
	"testing"
	"time"
)

// TestDeployBothSimple tests deploying both components together (simplest scenario)
func TestDeployBothSimple(t *testing.T) {
	dumpClusterOnFailure(t)

	// Create temporary envrc file
	envrcFile, err := os.CreateTemp(t.TempDir(), ".envrc.roxie-test-*")
	if err != nil {
		t.Fatalf("Failed to create temp envrc: %v", err)
	}
	envrcPath := envrcFile.Name()
	envrcFile.Close()

	t.Log("=== Deploying both components together ===")
	args := append([]string{roxieBinary, "deploy", "--early-readiness", "both", "--envrc", envrcPath}, commonDeployArgsNoPortForward...)
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
