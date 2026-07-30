//go:build e2e

package e2e

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/stackrox/roxie/internal/helm"
	"github.com/stackrox/roxie/internal/logger"
	"github.com/stretchr/testify/require"
)

func TestHelmChartAddOns(t *testing.T) {
	dumpClusterStateOnFailure(t)

	configDir := t.TempDir()

	configContent := `
central:
  addOns:
    test-chart: true
  availableAddOns:
    test-chart:
      helmChart:
        repo: https://prometheus-community.github.io/helm-charts
        chart: kube-state-metrics
        version: "5.27.0"
`
	configFile := filepath.Join(configDir, "addons-test-config.yaml")
	require.NoError(t, os.WriteFile(configFile, []byte(configContent), 0644))

	envrcFile, err := os.CreateTemp(t.TempDir(), ".envrc.roxie-test-*")
	require.NoError(t, err)
	envrcPath := envrcFile.Name()
	envrcFile.Close()

	t.Log("=== Deploying Central with add-on ===")
	args := append([]string{roxieBinary, "deploy", "central",
		"--early-readiness",
		"--envrc", envrcPath,
		"--config", configFile,
	}, commonDeployArgs...)
	runCommand(t, deployTimeout, nil, args...)

	helmCtx := helm.HelmCtx{
		Ctx:     t.Context(),
		Log:     logger.New(),
		Verbose: true,
	}

	verifyCentralInstalled(t, "acs-central")
	verifyHelmReleaseExists(t, helmCtx, "roxie-addon-test-chart", "acs-central")

	t.Log("=== Tearing down Central ===")
	teardownArgs := []string{roxieBinary, "teardown", "--skip-user-config", "central"}
	runCommand(t, teardownTimeout, nil, teardownArgs...)

	verifyHelmReleaseNotExists(t, helmCtx, "roxie-addon-test-chart", "acs-central")
}
