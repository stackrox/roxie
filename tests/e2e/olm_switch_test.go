//go:build e2e
// +build e2e

package e2e

import (
	"fmt"
	"os"
	"os/exec"
	"strings"
	"testing"
)

// Note: All e2e tests run sequentially via the -parallel=1 flag in the Makefile.
// This prevents conflicts when multiple tests modify shared cluster resources.

// verifyOperatorMode checks if the operator is deployed in the expected mode (OLM or non-OLM)
func verifyOperatorMode(t *testing.T, expectOLM bool) {
	t.Helper()

	operatorNamespace := operatorSystemNamespace
	subscriptionName := "stackrox-operator-subscription"

	// Check for OLM Subscription (OLM-specific resource)
	cmd := exec.Command("kubectl", "get", "subscription", subscriptionName, "-n", operatorNamespace)
	err := cmd.Run()
	olmExists := (err == nil)

	if expectOLM && !olmExists {
		t.Fatalf("Expected OLM operator but Subscription not found")
	}
	if !expectOLM && olmExists {
		t.Fatalf("Expected non-OLM operator but Subscription found (indicates OLM deployment)")
	}

	t.Logf("✓ Operator is deployed in expected mode (OLM: %v)", expectOLM)
}

// TestOLMToNonOLMSwitch tests switching from OLM operator to non-OLM operator
func TestOLMToNonOLMSwitch(t *testing.T) {
	skipUnlessOLM(t)
	ensureCleanCluster(t)
	dumpClusterStateOnFailure(t)

	// Create temporary envrc file
	envrcFile, err := os.CreateTemp(t.TempDir(), ".envrc.roxie-test-*")
	if err != nil {
		t.Fatalf("Failed to create temp envrc: %v", err)
	}
	envrcPath := envrcFile.Name()
	envrcFile.Close()

	// Step 1: Deploy central with OLM operator
	t.Log("=== Step 1: Deploy central with OLM operator ===")
	args := append([]string{roxieBinary, "deploy", "central", "--olm", "--envrc", envrcPath}, commonDeployArgs...)
	runCommand(t, deployTimeout, nil, args...)

	// Verify operator is in OLM mode
	t.Log("Verifying operator is in OLM mode")
	verifyOperatorMode(t, true)
	verifyOperatorDeploymentExists(t, operatorSystemNamespace)

	// Verify central namespace exists
	verifyNamespaceExists(t, centralNamespace)

	// Step 2: Deploy central again without OLM (should switch modes)
	t.Log("=== Step 2: Redeploy central without OLM (triggering mode switch) ===")
	// Note: we need to *explicitly* set OLM because of an pre-existing issue where the on-cluster manifest influences
	// the effective deployment configuration on a re-deployment.
	args = append([]string{roxieBinary, "deploy", "central", "--olm=false", "--envrc", envrcPath}, commonDeployArgs...)
	runCommand(t, deployTimeout, nil, args...)

	// Verify operator switched to non-OLM mode
	t.Log("Verifying operator switched to non-OLM mode")
	verifyOperatorMode(t, false)
	verifyOperatorDeploymentExists(t, operatorSystemNamespace)

	// Verify central namespace still exists
	verifyNamespaceExists(t, centralNamespace)
}

// TestNonOLMToOLMSwitch tests switching from non-OLM operator to OLM operator
func TestNonOLMToOLMSwitch(t *testing.T) {
	skipUnlessOLM(t)
	ensureCleanCluster(t)
	dumpClusterStateOnFailure(t)

	// Create temporary envrc file
	envrcFile, err := os.CreateTemp(t.TempDir(), ".envrc.roxie-test-*")
	if err != nil {
		t.Fatalf("Failed to create temp envrc: %v", err)
	}
	envrcPath := envrcFile.Name()
	envrcFile.Close()

	// Step 1: Deploy central without OLM (non-OLM operator)
	t.Log("=== Step 1: Deploy central with non-OLM operator ===")
	// Note: we need to *explicitly* set OLM because of an pre-existing issue where the on-cluster manifest influences
	// the effective deployment configuration on a re-deployment.
	args := append([]string{roxieBinary, "deploy", "central", "--olm=false", "--envrc", envrcPath}, commonDeployArgs...)
	runCommand(t, deployTimeout, nil, args...)

	// Verify operator is in non-OLM mode
	t.Log("Verifying operator is in non-OLM mode")
	verifyOperatorMode(t, false)
	verifyOperatorDeploymentExists(t, operatorSystemNamespace)

	// Verify central namespace exists
	verifyNamespaceExists(t, centralNamespace)

	// Step 2: Deploy central again with OLM (should switch modes)
	t.Log("=== Step 2: Redeploy central with OLM (triggering mode switch) ===")
	args = append([]string{roxieBinary, "deploy", "central", "--olm", "--envrc", envrcPath}, commonDeployArgs...)
	runCommand(t, deployTimeout, nil, args...)

	// Verify operator switched to OLM mode
	t.Log("Verifying operator switched to OLM mode")
	verifyOperatorMode(t, true)
	verifyOperatorDeploymentExists(t, operatorSystemNamespace)

	// Verify central namespace still exists
	verifyNamespaceExists(t, centralNamespace)
}

// TestOLMOperatorVersionUpgrade tests that OLM operator version mismatches trigger teardown and redeploy,
// and that operand tag overrides propagate RELATED_IMAGE_* env vars to the OLM Subscription.
func TestOLMOperatorVersionUpgrade(t *testing.T) {
	skipUnlessOLM(t)
	ensureCleanCluster(t)
	dumpClusterStateOnFailure(t)

	// Look up two release tags different from the main tag. We use one as a blanket
	// operand tag override and the other as a per-image override for central-db.
	tag1, tag2 := lookupTwoReleasedTags(t)
	mainTag := os.Getenv("MAIN_IMAGE_TAG")
	operandTag, perImageTag := tag1, tag2
	if operandTag == mainTag {
		operandTag, perImageTag = tag2, tag1
	}
	t.Logf("Main tag: %s, operandTag: %s, per-image override (central-db): %s", mainTag, operandTag, perImageTag)

	// Create temporary envrc file
	envrcFile, err := os.CreateTemp(t.TempDir(), ".envrc.roxie-test-*")
	if err != nil {
		t.Fatalf("Failed to create temp envrc: %v", err)
	}
	envrcPath := envrcFile.Name()
	envrcFile.Close()

	t.Log("=== Step 1: Deploy central with OLM operator and operand tag overrides ===")
	args := append([]string{
		roxieBinary, "deploy", "central", "--olm", "--envrc", envrcPath,
		"--set", "operator.operandTag=" + operandTag,
		"--set", "operator.operandTagOverrides.central-db=" + perImageTag,
	}, commonDeployArgs...)
	runCommand(t, deployTimeout, nil, args...)

	// Verify operator is in OLM mode
	t.Log("Verifying initial OLM operator deployment")
	verifyOperatorMode(t, true)
	verifyOperatorDeploymentExists(t, operatorSystemNamespace)

	// Verify that operandTag caused RELATED_IMAGE_MAIN to use the blanket override.
	verifyOperatorDeploymentEnvVar(t,
		operatorSystemNamespace, "RELATED_IMAGE_MAIN", fmt.Sprintf("quay.io/rhacs-eng/main:%s", operandTag))

	// Verify that operandTagOverrides caused RELATED_IMAGE_CENTRAL_DB to use
	// the per-image tag instead of the blanket operandTag.
	if perImageTag != mainTag {
		verifyOperatorDeploymentEnvVar(t, operatorSystemNamespace,
			"RELATED_IMAGE_CENTRAL_DB", fmt.Sprintf("quay.io/rhacs-eng/central-db:%s", perImageTag))
	}

	// Get the current operator version
	cmd := exec.Command("kubectl", "get", "deployment", operatorDeploymentName, "-n", operatorSystemNamespace,
		"-o", "jsonpath={.spec.template.spec.containers[0].image}")
	output, err := cmd.Output()
	if err != nil {
		t.Fatalf("Failed to get operator image: %v", err)
	}
	initialImage := strings.TrimSpace(string(output))
	t.Logf("Initial operator image: %s", initialImage)

	// Step 2: Redeploy with same version (should skip if version matches)
	t.Log("=== Step 2: Redeploy with same version (should detect correct version) ===")
	args = append([]string{roxieBinary, "deploy", "central", "--olm", "--envrc", envrcPath}, commonDeployArgs...)
	runCommand(t, deployTimeout, nil, args...)

	// Verify operator is still in OLM mode and deployment exists
	t.Log("Verifying operator is still deployed correctly")
	verifyOperatorMode(t, true)
	verifyOperatorDeploymentExists(t, operatorSystemNamespace)

	// Note: In a real version upgrade test, we would set a different MAIN_IMAGE_TAG
	// and verify that the operator was torn down and redeployed with the new version.
	// For now, this test validates the basic flow.
}

// TestSecuredClusterWithOLMSwitch tests that secured-cluster deployment also respects OLM mode switches
func TestSecuredClusterWithOLMSwitch(t *testing.T) {
	skipUnlessOLM(t)
	ensureCleanCluster(t)
	dumpClusterStateOnFailure(t)

	// Create temporary envrc file
	envrcFile, err := os.CreateTemp(t.TempDir(), ".envrc.roxie-test-*")
	if err != nil {
		t.Fatalf("Failed to create temp envrc: %v", err)
	}
	envrcPath := envrcFile.Name()
	envrcFile.Close()

	// Step 1: Deploy central with OLM
	t.Log("=== Step 1: Deploy central with OLM ===")
	args := append([]string{roxieBinary, "deploy", "--early-readiness", "central", "--olm", "--envrc", envrcPath}, commonDeployArgs...)
	runCommand(t, deployTimeout, nil, args...)

	verifyOperatorMode(t, true)
	verifyNamespaceExists(t, centralNamespace)

	// Load envrc for secured-cluster
	envrcEnv, err := loadEnvrcFile(envrcPath)
	if err != nil {
		t.Fatalf("Failed to load envrc file: %v", err)
	}

	// Step 2: Deploy secured-cluster (should reuse OLM operator)
	t.Log("=== Step 2: Deploy secured-cluster (should reuse OLM operator) ===")
	args = append([]string{roxieBinary, "deploy", "--early-readiness", "secured-cluster", "--olm"}, commonDeployArgs...)
	runCommand(t, deployTimeout, envrcEnv, args...)

	// Verify operator is still in OLM mode
	verifyOperatorMode(t, true)
	verifyNamespaceExists(t, sensorNamespace)

	// Step 3: Switch to non-OLM by redeploying secured-cluster without --olm
	t.Log("=== Step 3: Redeploy secured-cluster without OLM (triggering mode switch) ===")
	// Note: we need to *explicitly* set OLM because of an pre-existing issue where the on-cluster manifest influences
	// the effective deployment configuration on a re-deployment.
	args = append([]string{roxieBinary, "deploy", "--early-readiness", "--olm=false", "secured-cluster"}, commonDeployArgs...)
	runCommand(t, deployTimeout, envrcEnv, args...)

	// Verify operator switched to non-OLM mode
	verifyOperatorMode(t, false)
	verifyNamespaceExists(t, sensorNamespace)
}
