//go:build e2e

package e2e

import (
	"fmt"
	"os/exec"
	"path/filepath"
	"strings"
	"sync"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// TestMultipleSecuredClusters verifies that several SecuredClusters can be deployed side by side,
// that teardown with an explicit namespace only removes the SecuredCluster in that namespace, and
// that teardown without configured namespaces discovers and removes everything.
func TestMultipleSecuredClusters(t *testing.T) {
	modifiedCommonDeployArgs := append(commonDeployArgs, "--resources=tiny")
	const nsA, nsB = "acs-sensor-a", "acs-sensor-b"
	// roxie teardown removes the SecuredCluster resources but not the namespaces they
	// lived in; delete the test-specific namespaces so they don't pile up on the cluster.
	// Registered before registerClusterDumpAndCleanup, so that (cleanups running in reverse
	// order) it only runs after the failure dump and roxie teardown have completed.
	t.Cleanup(func() {
		_ = exec.Command("kubectl", "delete", "namespace", nsA, nsB, "--ignore-not-found").Run()
	})
	registerClusterDumpAndCleanup(t)

	// TODO: Drop the pinned tag (and use the default MAIN_IMAGE_TAG like other tests) once
	// ACS 5.1.0 is released. Deploying multiple SecuredClusters into one cluster requires the
	// operator to support the platform.stackrox.io/namespace-prefix-global-resources annotation
	// (stackrox/stackrox#23099), which avoids name clashes of cluster-scoped resources. No released
	// version supports it yet; the e2e suite otherwise defaults to the latest release.
	const nightlyTag = "5.1.x-nightly-20261117"

	// Deploy Central.
	envrcPath := filepath.Join(t.TempDir(), ".envrc")
	runCommand(t, deployTimeout, nil, append([]string{roxieBinary, "deploy", "--early-readiness", "central",
		"--tag", nightlyTag, "--envrc", envrcPath}, modifiedCommonDeployArgs...)...)

	// Deploy two SecuredClusters.
	// TODO, using async waitgroup.
	envrcEnv := loadEnvrcFile(t, envrcPath)
	securedClusterCounter := 0
	deploySecuredCluster := func(ns string) {
		t.Logf("=== Deploying SecuredCluster into namespace %s ===", ns)
		securedClusterCounter++
		clusterName := fmt.Sprintf("sc-%d", securedClusterCounter) // For redeployment test we need fresh cluster names.
		roxieCmd := append([]string{roxieBinary, "deploy", "--early-readiness", "secured-cluster", "--tag", nightlyTag,
			"--set", `securedCluster.metadata.annotations.platform\.stackrox\.io/namespace-prefix-global-resources=true`,
			"--set", "securedCluster.namespace=" + ns, "--set", "securedCluster.spec.clusterName=" + clusterName},
			modifiedCommonDeployArgs...)
		runCommandPrefixed(t, ns+" ", deployTimeout, envrcEnv, roxieCmd...)
	}
	var wg sync.WaitGroup
	for _, ns := range []string{nsA, nsB} {
		wg.Go(func() {
			deploySecuredCluster(ns)
		})
	}
	wg.Wait()
	assert.ElementsMatch(t, []string{nsA, nsB}, crNamespaces(t, "securedcluster"))
	uidB := securedClusterUID(t, nsB)

	// Redeploying A must not touch B: B's CR keeps its UID (i.e. was not deleted and recreated).
	deploySecuredCluster(nsA)
	assert.ElementsMatch(t, []string{nsA, nsB}, crNamespaces(t, "securedcluster"))
	assert.Equal(t, uidB, securedClusterUID(t, nsB), "SecuredCluster in %s was replaced by redeploying %s", nsB, nsA)

	// Tear down SC in nsA.
	t.Logf("=== Tearing down SecuredCluster in namespace %s only ===", nsA)
	runCommand(t, teardownTimeout, nil, roxieBinary, "teardown", "--skip-user-config", "secured-cluster", "--set", "securedCluster.namespace="+nsA)
	assert.Equal(t, []string{nsB}, crNamespaces(t, "securedcluster"))

	// Tear down everything.
	t.Log("=== Tearing down everything without configured namespaces ===")
	runCommand(t, teardownTimeout, nil, roxieBinary, "teardown", "--skip-user-config", "all")
	assert.Empty(t, crNamespaces(t, "securedcluster"))
	assert.Empty(t, crNamespaces(t, "central"))
}

// crNamespaces returns the namespaces containing a CR of the given type.
func crNamespaces(t *testing.T, crType string) []string {
	t.Helper()

	output, err := exec.Command("kubectl", "get", crType, "-A", "-o", "jsonpath={.items[*].metadata.namespace}").Output()
	require.NoError(t, err, "Failed to list %s CRs", crType)
	return strings.Fields(string(output))
}

// securedClusterUID returns the UID of the SecuredCluster CR in the given namespace.
func securedClusterUID(t *testing.T, namespace string) string {
	t.Helper()

	output, err := exec.Command("kubectl", "get", "securedcluster", "stackrox-secured-cluster-services",
		"-n", namespace, "-o", "jsonpath={.metadata.uid}").Output()
	require.NoError(t, err, "Failed to get SecuredCluster UID in namespace %s", namespace)
	return string(output)
}
