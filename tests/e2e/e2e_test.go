//go:build e2e

package e2e

import (
	"context"
	"fmt"
	"os"
	"testing"
	"time"

	"github.com/stackrox/roxie/internal/helpers"
	"github.com/stackrox/roxie/internal/logger"
)

func TestMain(m *testing.M) {
	// Setup: verify prerequisites
	if err := requireBinary("kubectl"); err != nil {
		fmt.Fprintf(os.Stderr, "kubectl not found: %v\n", err)
		os.Exit(1)
	}

	// Use the most recent released ACS version if MAIN_IMAGE_TAG is not set.
	if os.Getenv("MAIN_IMAGE_TAG") == "" {
		mainImageTag, err := lookupLatestTag()
		if err != nil {
			fmt.Fprintf(os.Stderr, "failed to lookup latest tag: %v\n", err)
			os.Exit(1)
		}
		os.Setenv("MAIN_IMAGE_TAG", mainImageTag)
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

func lookupLatestTag() (string, error) {
	log := logger.New()
	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Minute)
	defer cancel()
	tag, err := helpers.LookupLatestTag(ctx, log)
	if err != nil {
		return "", err
	}
	return tag, nil
}

func TestDeployBothComponentsTogetherInSingleNamespace(t *testing.T) {
	dumpClusterStateOnFailure(t)

	// Create temporary envrc file.
	envrcFile, err := os.CreateTemp(t.TempDir(), ".envrc.roxie-test-*")
	if err != nil {
		t.Fatalf("Failed to create temp envrc: %v", err)
	}
	envrcPath := envrcFile.Name()
	envrcFile.Close()

	t.Log("=== Deploying both components in single namespace ===")
	args := append([]string{roxieBinary, "deploy", "both", "--single-namespace", "--early-readiness", "--envrc", envrcPath}, commonDeployArgs...)
	runCommand(t, deployTimeout*2, nil, args...)

	verifyCentralInstalled(t, "stackrox")
	verifySecuredClusterInstalled(t, "stackrox")

	t.Log("=== Tearing down both components in single namespace ===")
	args = []string{roxieBinary, "teardown", "--skip-user-config", "--single-namespace"}
	runCommand(t, teardownTimeout, nil, args...)

	verifyCentralNotInstalled(t, "stackrox")
	verifySecuredClusterNotInstalled(t, "stackrox")
}
