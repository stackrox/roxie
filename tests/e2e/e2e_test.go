//go:build e2e

package e2e

import (
	"fmt"
	"os"
	"testing"
)

func TestMain(m *testing.M) {
	// Setup: verify prerequisites
	if err := requireBinary("kubectl"); err != nil {
		fmt.Fprintf(os.Stderr, "kubectl not found: %v\n", err)
		os.Exit(1)
	}

	// Set default MAIN_IMAGE_TAG if not set
	if os.Getenv("MAIN_IMAGE_TAG") == "" {
		os.Setenv("MAIN_IMAGE_TAG", defaultMainImageTag)
	}

	// Verify kubectl context
	ctx, err := requireKubeContext()
	if err != nil {
		fmt.Fprintf(os.Stderr, "kubectl context check failed: %v\n", err)
		os.Exit(1)
	}
	fmt.Printf("Using kubectl context: %s\n", ctx)

	// Teardown all deployments before running tests
	if err := teardownAllDeployments(); err != nil {
		fmt.Fprintf(os.Stderr, "Warning: teardown all deployments failed: %v\n", err)
		os.Exit(1)
	}

	// Run tests
	os.Exit(m.Run())
}

func TestDeployBothComponentsTogetherInSingleNamespace(t *testing.T) {
	dumpClusterOnFailure(t)

	// Create temporary envrc file.
	envrcFile, err := os.CreateTemp(t.TempDir(), ".envrc.roxie-test-*")
	if err != nil {
		t.Fatalf("Failed to create temp envrc: %v", err)
	}
	envrcPath := envrcFile.Name()
	envrcFile.Close()

	t.Log("=== Deploying both components in single namespace ===")
	args := append([]string{roxieBinary, "deploy", "both", "--single-namespace", "--early-readiness", "--envrc", envrcPath}, commonDeployArgsNoPortForward...)
	runCommand(t, deployTimeout*2, nil, args...)

	verifyCentralInstalled(t, "stackrox")
	verifySecuredClusterInstalled(t, "stackrox")

	t.Log("=== Tearing down both components in single namespace ===")
	args = []string{roxieBinary, "teardown", "--single-namespace"}
	runCommand(t, teardownTimeout, nil, args...)

	verifyCentralNotInstalled(t, "stackrox")
	verifySecuredClusterNotInstalled(t, "stackrox")
}
