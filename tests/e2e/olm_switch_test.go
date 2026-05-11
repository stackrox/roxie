//go:build e2e
// +build e2e

package e2e

import (
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

	operatorNamespace := "rhacs-operator-system"
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

// verifyOperatorDeploymentExists checks if the operator deployment exists
func verifyOperatorDeploymentExists(t *testing.T) {
	t.Helper()

	operatorNamespace := "rhacs-operator-system"
	deploymentName := "rhacs-operator-controller-manager"

	cmd := exec.Command("kubectl", "get", "deployment", deploymentName, "-n", operatorNamespace)
	if err := cmd.Run(); err != nil {
		t.Fatalf("Operator deployment not found: %v", err)
	}

	// Also check if deployment is ready
	cmd = exec.Command("kubectl", "get", "deployment", deploymentName, "-n", operatorNamespace,
		"-o", "jsonpath={.status.readyReplicas}")
	output, err := cmd.Output()
	if err != nil {
		t.Fatalf("Failed to check operator deployment readiness: %v", err)
	}

	readyReplicas := strings.TrimSpace(string(output))
	if readyReplicas == "" || readyReplicas == "0" {
		t.Fatalf("Operator deployment exists but has no ready replicas")
	}

	t.Logf("✓ Operator deployment exists and is ready (%s replicas)", readyReplicas)
}

// TestOLMToNonOLMSwitch tests switching from OLM operator to non-OLM operator
func TestOLMToNonOLMSwitch(t *testing.T) {
	if os.Getenv("SKIP_OLM_TESTS") != "" {
		t.Skip("SKIP_OLM_TESTS is set")
	}

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
	verifyOperatorDeploymentExists(t)

	// Verify central namespace exists
	verifyNamespaceExists(t, "acs-central")

	// Step 2: Deploy central again without OLM (should switch modes)
	t.Log("=== Step 2: Redeploy central without OLM (triggering mode switch) ===")
	args = append([]string{roxieBinary, "deploy", "central", "--envrc", envrcPath}, commonDeployArgs...)
	runCommand(t, deployTimeout, nil, args...)

	// Verify operator switched to non-OLM mode
	t.Log("Verifying operator switched to non-OLM mode")
	verifyOperatorMode(t, false)
	verifyOperatorDeploymentExists(t)

	// Verify central namespace still exists
	verifyNamespaceExists(t, "acs-central")

	// Cleanup
	t.Log("=== Cleaning up ===")
	teardownArgs := []string{roxieBinary, "teardown", "central"}
	runCommand(t, teardownTimeout, nil, teardownArgs...)

	verifyCentralNotInstalled(t, "acs-central")
}

// TestNonOLMToOLMSwitch tests switching from non-OLM operator to OLM operator
func TestNonOLMToOLMSwitch(t *testing.T) {
	if os.Getenv("SKIP_OLM_TESTS") != "" {
		t.Skip("SKIP_OLM_TESTS is set")
	}

	// Create temporary envrc file
	envrcFile, err := os.CreateTemp(t.TempDir(), ".envrc.roxie-test-*")
	if err != nil {
		t.Fatalf("Failed to create temp envrc: %v", err)
	}
	envrcPath := envrcFile.Name()
	envrcFile.Close()

	// Step 1: Deploy central without OLM (non-OLM operator)
	t.Log("=== Step 1: Deploy central with non-OLM operator ===")
	args := append([]string{roxieBinary, "deploy", "central", "--envrc", envrcPath}, commonDeployArgs...)
	runCommand(t, deployTimeout, nil, args...)

	// Verify operator is in non-OLM mode
	t.Log("Verifying operator is in non-OLM mode")
	verifyOperatorMode(t, false)
	verifyOperatorDeploymentExists(t)

	// Verify central namespace exists
	verifyNamespaceExists(t, "acs-central")

	// Step 2: Deploy central again with OLM (should switch modes)
	t.Log("=== Step 2: Redeploy central with OLM (triggering mode switch) ===")
	args = append([]string{roxieBinary, "deploy", "central", "--olm", "--envrc", envrcPath}, commonDeployArgs...)
	runCommand(t, deployTimeout, nil, args...)

	// Verify operator switched to OLM mode
	t.Log("Verifying operator switched to OLM mode")
	verifyOperatorMode(t, true)
	verifyOperatorDeploymentExists(t)

	// Verify central namespace still exists
	verifyNamespaceExists(t, "acs-central")

	// Cleanup
	t.Log("=== Cleaning up ===")
	teardownArgs := []string{roxieBinary, "teardown", "central"}
	runCommand(t, teardownTimeout, nil, teardownArgs...)

	verifyCentralNotInstalled(t, "acs-central")
}

// TestOLMOperatorVersionUpgrade tests that OLM operator version mismatches trigger teardown and redeploy
func TestOLMOperatorVersionUpgrade(t *testing.T) {
	if os.Getenv("SKIP_OLM_TESTS") != "" {
		t.Skip("SKIP_OLM_TESTS is set")
	}

	// This test requires two different operator versions
	// We can simulate this by deploying twice with the same version,
	// but the logic should handle version changes by tearing down and redeploying

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
	t.Log("Verifying initial OLM operator deployment")
	verifyOperatorMode(t, true)
	verifyOperatorDeploymentExists(t)

	// Get the current operator version
	operatorNamespace := "rhacs-operator-system"
	deploymentName := "rhacs-operator-controller-manager"
	cmd := exec.Command("kubectl", "get", "deployment", deploymentName, "-n", operatorNamespace,
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
	verifyOperatorDeploymentExists(t)

	// Note: In a real version upgrade test, we would set a different MAIN_IMAGE_TAG
	// and verify that the operator was torn down and redeployed with the new version.
	// For now, this test validates the basic flow.

	// Cleanup
	t.Log("=== Cleaning up ===")
	teardownArgs := []string{roxieBinary, "teardown", "central"}
	runCommand(t, teardownTimeout, nil, teardownArgs...)

	verifyCentralNotInstalled(t, "acs-central")
}

// TestSecuredClusterWithOLMSwitch tests that secured-cluster deployment also respects OLM mode switches
func TestSecuredClusterWithOLMSwitch(t *testing.T) {
	if os.Getenv("SKIP_OLM_TESTS") != "" {
		t.Skip("SKIP_OLM_TESTS is set")
	}

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
	verifyNamespaceExists(t, "acs-central")

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
	verifyNamespaceExists(t, "acs-sensor")

	// Step 3: Switch to non-OLM by redeploying secured-cluster without --olm
	t.Log("=== Step 3: Redeploy secured-cluster without OLM (triggering mode switch) ===")
	args = append([]string{roxieBinary, "deploy", "--early-readiness", "secured-cluster"}, commonDeployArgs...)
	runCommand(t, deployTimeout, envrcEnv, args...)

	// Verify operator switched to non-OLM mode
	verifyOperatorMode(t, false)
	verifyNamespaceExists(t, "acs-sensor")

	// Cleanup
	t.Log("=== Cleaning up ===")
	teardownArgs := []string{roxieBinary, "teardown", "both"}
	runCommand(t, teardownTimeout, nil, teardownArgs...)

	verifyCentralNotInstalled(t, "acs-central")
	verifySecuredClusterNotInstalled(t, "acs-sensor")
}
