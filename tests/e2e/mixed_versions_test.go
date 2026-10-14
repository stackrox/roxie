//go:build e2e
// +build e2e

package e2e

import (
	"context"
	"fmt"
	"testing"
	"time"

	"github.com/stackrox/roxie/internal/constants"
	"github.com/stackrox/roxie/internal/ocihelper"
	"github.com/stackrox/roxie/internal/stackroxversions"
)

const (
	nonExistentTag = "0.0.1-override-test"
)

// lookupTwoReleasedTags returns two different pullable release tags.
func lookupTwoReleasedTags(t *testing.T) (string, string) {
	t.Helper()

	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Minute)
	defer cancel()

	tags, err := stackroxversions.LookupLatestReleaseTagsViaGitHub(ctx, 5)
	if err != nil {
		t.Fatalf("Failed to look up release tags: %v", err)
	}

	var verified []string
	for _, tag := range tags {
		mainImage := fmt.Sprintf("%s/main:%s", constants.DefaultRegistry, tag)
		if err := ocihelper.VerifyImageExistence(ctx, mainImage); err != nil {
			continue
		}
		verified = append(verified, tag)
		if len(verified) == 2 {
			break
		}
	}
	if len(verified) < 2 {
		t.Fatalf("Could not find two pullable release tags (found: %v)", verified)
	}
	return verified[0], verified[1]
}

// TestMixedVersionOperatorDeploy tests deploying two operators with different versions
// (mixed-version mode) and then switching back to a single operator.
func TestMixedVersionOperatorDeploy(t *testing.T) {
	dumpClusterStateOnFailure(t)

	mainTag, centralTag := lookupTwoReleasedTags(t)

	t.Logf("Using --tag=%s --central-tag=%s", mainTag, centralTag)

	// Step 1: Deploy operator with mixed versions
	t.Log("=== Step 1: Deploy operator with mixed versions ===")
	args := append([]string{roxieBinary, "deploy", "operator", "--tag", mainTag, "--central-tag", centralTag}, commonDeployArgs...)
	runCommand(t, operatorDeployTimeout, nil, args...)

	verifyOperatorDeploymentExists(t, operatorCentralNamespace)
	verifyOperatorDeploymentExists(t, operatorSensorNamespace)
	verifyOperatorNotInNamespace(t, operatorSystemNamespace)

	verifyOperatorVersion(t, operatorCentralNamespace, centralTag)
	verifyOperatorVersion(t, operatorSensorNamespace, mainTag)

	t.Log("=== Step 2: Deploy single-version operator with operand tag overrides ===")
	// Switch back to single-version operator, with operand tag overrides.
	// This verifies that populateOperatorEnvVars injects RELATED_IMAGE_* env vars
	// into the non-OLM operator Deployment when operandTag / operandTagOverrides
	// differ from the operator version.
	singleArgs := append([]string{
		roxieBinary, "deploy", "operator",
		"--tag", mainTag, "--central-tag", mainTag, "--secured-cluster-tag", mainTag,
		"--set", "operator.operandTag=" + centralTag,
		"--set", "operator.operandTagOverrides.central-db=" + nonExistentTag,
	}, commonDeployArgs...)
	runCommand(t, operatorDeployTimeout, nil, singleArgs...)

	verifyOperatorDeploymentExists(t, operatorSystemNamespace)
	verifyOperatorNotInNamespace(t, operatorCentralNamespace)
	verifyOperatorNotInNamespace(t, operatorSensorNamespace)

	// operandTag applies to all images by default.
	verifyOperatorDeploymentEnvVar(t,
		operatorSystemNamespace, "RELATED_IMAGE_MAIN", fmt.Sprintf("quay.io/rhacs-eng/main:%s", centralTag))
	// operandTagOverrides takes precedence for the specific image.
	verifyOperatorDeploymentEnvVar(t,
		operatorSystemNamespace, "RELATED_IMAGE_CENTRAL_DB", fmt.Sprintf("quay.io/rhacs-eng/central-db:%s", nonExistentTag))

	// Step 3: Switch from single back to mixed
	t.Log("=== Step 3: Deploy operator with mixed versions again (switch from single to mixed) ===")
	runCommand(t, operatorDeployTimeout, nil, args...)

	verifyOperatorDeploymentExists(t, operatorCentralNamespace)
	verifyOperatorDeploymentExists(t, operatorSensorNamespace)
	verifyOperatorNotInNamespace(t, operatorSystemNamespace)

	// Cleanup
	t.Log("=== Cleaning up ===")
	teardownArgs := []string{roxieBinary, "teardown", "--skip-user-config"}
	runCommand(t, teardownTimeout, nil, teardownArgs...)

	verifyOperatorNotInNamespace(t, operatorCentralNamespace)
	verifyOperatorNotInNamespace(t, operatorSensorNamespace)
	verifyOperatorNotInNamespace(t, operatorSystemNamespace)
}
