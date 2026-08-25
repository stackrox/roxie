//go:build e2e

package e2e

import (
	"fmt"
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
	verifyCentralInstalled(t, centralNamespace)
	verifyContainerImageRegistry(t, operatorSystemNamespace, operatorDeploymentName, "manager", stackroxIORegistry)
	verifyContainerImageRegistry(t, centralNamespace, "central", "central", stackroxIORegistry)

	t.Log("=== Cleaning up ===")
	teardownArgs := []string{roxieBinary, "teardown", "--skip-user-config", "all"}
	runCommand(t, teardownTimeout, nil, teardownArgs...)

	verifyCentralNotInstalled(t, centralNamespace)
}

// verifyContainerImageRegistry asserts that the given container's image in the
// given deployment is hosted on the expected registry.
func verifyContainerImageRegistry(t *testing.T, namespace, deployment, container, expectedRegistry string) {
	t.Helper()

	jsonpath := fmt.Sprintf("{.spec.template.spec.containers[?(@.name==%q)].image}", container)
	cmd := exec.Command("kubectl", "get", "deployment", deployment, "-n", namespace,
		"-o", "jsonpath="+jsonpath)
	output, err := cmd.Output()
	require.NoErrorf(t, err, "Failed to get image for container %s in deployment %s/%s", container, namespace, deployment)

	image := strings.TrimSpace(string(output))
	require.NotEmptyf(t, image, "Container %s not found in deployment %s/%s", container, namespace, deployment)
	require.Truef(t, strings.HasPrefix(image, expectedRegistry+"/"),
		"Expected %s/%s container %s image to be from %s, got: %s", namespace, deployment, container, expectedRegistry, image)
	t.Logf("✓ %s/%s container %s image %s uses registry %s", namespace, deployment, container, image, expectedRegistry)
}
