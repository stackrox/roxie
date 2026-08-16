//go:build e2e

package e2e

import (
	"os/exec"
	"strings"
	"testing"

	"github.com/stretchr/testify/require"
)

// TestCentralDeployWithStackroxIORegistry verifies that roxie can deploy Central using
// the public quay.io/stackrox-io registry instead of the default quay.io/rhacs-eng.
func TestCentralDeployWithStackroxIORegistry(t *testing.T) {
	dumpClusterStateOnFailure(t)

	const stackroxIORegistry = "quay.io/stackrox-io"

	t.Log("=== Deploying central with quay.io/stackrox-io registry ===")
	args := append([]string{
		roxieBinary, "deploy", "--early-readiness", "central",
		"--set", "roxie.imageRegistry=" + stackroxIORegistry,
		"--envrc", "/dev/null",
	}, commonDeployArgs...)
	runCommand(t, deployTimeout, nil, args...)

	verifyOperatorDeploymentExists(t, operatorSystemNamespace)
	verifyOperatorImageRegistry(t, operatorSystemNamespace, stackroxIORegistry)
	verifyCentralInstalled(t, centralNamespace)

	t.Log("=== Cleaning up ===")
	teardownArgs := []string{roxieBinary, "teardown", "--skip-user-config", "central"}
	runCommand(t, teardownTimeout, nil, teardownArgs...)

	verifyCentralNotInstalled(t, centralNamespace)
}

// verifyOperatorImageRegistry asserts that the operator deployment's image is
// hosted on the expected registry.
func verifyOperatorImageRegistry(t *testing.T, namespace, expectedRegistry string) {
	t.Helper()

	cmd := exec.Command("kubectl", "get", "deployment", operatorDeploymentName, "-n", namespace,
		"-o", "jsonpath={.spec.template.spec.containers[0].image}")
	output, err := cmd.Output()
	require.NoErrorf(t, err, "Failed to get operator image in namespace %s", namespace)

	image := strings.TrimSpace(string(output))
	require.Truef(t, strings.HasPrefix(image, expectedRegistry+"/"),
		"Expected operator image to be pulled from %s, got: %s", expectedRegistry, image)
	t.Logf("✓ Operator image %s uses registry %s", image, expectedRegistry)
}
