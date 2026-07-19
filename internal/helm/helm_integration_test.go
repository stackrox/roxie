//go:build integration

package helm

import (
	"os/exec"
	"strings"
	"testing"

	"github.com/stackrox/roxie/internal/logger"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

const (
	testChartPath = "testdata/minimal-test-chart"
)

func TestInstallAndUninstall(t *testing.T) {
	ctx := t.Context()
	helmCtx := HelmCtx{
		Ctx:     ctx,
		Log:     logger.New(),
		Verbose: true,
	}

	namespace := createTestNamespace(t)
	releaseName := "helm-integ-lifecycle"
	opts := InstallOptions{
		ReleaseName: releaseName,
		ChartPath:   testChartPath,
		Namespace:   namespace,
	}
	err := Install(helmCtx, opts)
	require.NoError(t, err, "initial install")

	releases, err := ListByPrefix(helmCtx, releaseName, namespace)
	require.NoError(t, err)
	assert.Len(t, releases, 1, "multiple releases listed")
	assert.Contains(t, releases, releaseName, "release should exist after install")

	err = Install(helmCtx, opts)
	require.NoError(t, err, "idempotent re-install")

	releases, err = ListByPrefix(helmCtx, releaseName, namespace)
	require.NoError(t, err)
	assert.Len(t, releases, 1, "multiple releases listed")
	assert.Contains(t, releases, releaseName, "release should exist after install")

	err = Uninstall(helmCtx, releaseName, namespace)
	require.NoError(t, err, "uninstall")

	releases, err = ListByPrefix(helmCtx, releaseName, namespace)
	require.NoError(t, err)
	assert.NotContains(t, releases, releaseName, "release should be gone after uninstall")
	assert.Empty(t, releases, "releases still listed")

	err = Uninstall(helmCtx, releaseName, namespace)
	require.NoError(t, err, "uninstall of already-removed release should succeed")
}

func TestInstallWithValues(t *testing.T) {
	ctx := t.Context()
	helmCtx := HelmCtx{
		Ctx:     ctx,
		Log:     logger.New(),
		Verbose: true,
	}

	namespace := createTestNamespace(t)
	releaseName := "helm-integ-values"
	opts := InstallOptions{
		ReleaseName: releaseName,
		ChartPath:   testChartPath,
		Namespace:   namespace,
		Values:      map[string]any{"target": "mundo"},
	}

	err := Install(helmCtx, opts)
	require.NoError(t, err)

	out, err := exec.Command("kubectl", "get", "configmap", releaseName+"-cm",
		"-n", namespace, "-o", "jsonpath={.data.target}").CombinedOutput()
	require.NoError(t, err, "getting configmap: %s", string(out))
	assert.Equal(t, "mundo", string(out))
}

func TestListByPrefix_NoMatches(t *testing.T) {
	ctx := t.Context()
	helmCtx := HelmCtx{
		Ctx:     ctx,
		Log:     logger.New(),
		Verbose: true,
	}
	releases, err := ListByPrefix(helmCtx, "nonexistent-prefix-xyz-", "default")
	require.NoError(t, err)
	assert.Empty(t, releases)
}

func createTestNamespace(t *testing.T) string {
	t.Helper()
	ns := getTestNamespaceName(t)
	out, err := exec.Command("kubectl", "create", "namespace", ns).CombinedOutput()
	require.NoError(t, err, "creating namespace: %s", string(out))
	t.Cleanup(func() {
		_ = exec.Command("kubectl", "delete", "namespace", ns, "--ignore-not-found").Run()
	})
	return ns
}

func getTestNamespaceName(t *testing.T) string {
	name := t.Name()
	name = strings.ReplaceAll(name, "/", "-")
	name = strings.ReplaceAll(name, "_", "-")
	name = strings.ToLower(name)
	name = "helm-test-" + name
	name = name[:63]
	name = strings.TrimSuffix(name, "-")
	return name
}
