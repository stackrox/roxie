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
	if os.Getenv("SKIP_OPERATOR_TESTS") != "" {
		t.Skip("SKIP_OPERATOR_TESTS is set")
	}

	// Create temporary envrc file
	envrcFile, err := os.CreateTemp("", ".envrc.roxie-test-*")
	if err != nil {
		t.Fatalf("Failed to create temp envrc: %v", err)
	}
	envrcPath := envrcFile.Name()
	envrcFile.Close()
	defer os.Remove(envrcPath)

	t.Log("=== Deploying both components together ===")
	args := append([]string{roxieBinary, "deploy", "both", "--envrc", envrcPath}, commonDeployArgsNoPortForward...)
	runCommand(t, deployTimeout*2, nil, args...)

	// Verify namespaces exist
	t.Log("Verifying namespace: acs-central")
	verifyNamespaceExists(t, "acs-central")

	t.Log("Verifying namespace: acs-sensor")
	verifyNamespaceExists(t, "acs-sensor")

	// Brief pause before cleanup
	time.Sleep(5 * time.Second)

	// Cleanup
	t.Log("=== Cleaning up ===")
	teardownArgs := []string{roxieBinary, "teardown", "both"}
	runCommand(t, teardownTimeout, nil, teardownArgs...)

	// Verify namespaces are deleted
	t.Log("Verifying namespaces are removed")
	verifyNamespaceAbsent(t, "acs-central")
	verifyNamespaceAbsent(t, "acs-sensor")
}
