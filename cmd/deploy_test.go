package main

import (
	"os"
	"path/filepath"
	"reflect"
	"testing"
	"time"

	"github.com/stackrox/roxie/internal/deployer"
	"github.com/stackrox/roxie/internal/types"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestNewDeployCmd_Flags(t *testing.T) {
	configFilePath := filepath.Join(t.TempDir(), "config.yaml")

	tests := []struct {
		name   string
		config string
		args   []string
		assert func(t *testing.T, cfg deployer.Config)
	}{
		{
			name: "exposure loadbalancer",
			args: []string{"--exposure", "loadbalancer"},
			assert: func(t *testing.T, cfg deployer.Config) {
				require.NotNil(t, cfg.Central.Exposure, "Central.Exposure should be set")
				assert.Equal(t, types.ExposureLoadBalancer, *cfg.Central.Exposure, "Central.Exposure mismatch")
			},
		},
		{
			name: "resources small",
			args: []string{"--resources", "small"},
			assert: func(t *testing.T, cfg deployer.Config) {
				assert.Equal(t, types.ResourceProfileSmall, cfg.Central.ResourceProfile, "Central.ResourceProfile mismatch")
				assert.Equal(t, types.ResourceProfileSmall, cfg.SecuredCluster.ResourceProfile, "SecuredCluster.ResourceProfile mismatch")
			},
		},
		{
			name: "tag short flag",
			args: []string{"-t", "4.7.0"},
			assert: func(t *testing.T, cfg deployer.Config) {
				assert.Equal(t, "4.7.0", cfg.Roxie.Version, "Roxie.Version mismatch")
			},
		},
		{
			name: "tag long flag",
			args: []string{"--tag", "4.7.0"},
			assert: func(t *testing.T, cfg deployer.Config) {
				assert.Equal(t, "4.7.0", cfg.Roxie.Version, "Roxie.Version mismatch")
			},
		},
		{
			name: "port-forwarding enabled",
			args: []string{"--port-forwarding"},
			assert: func(t *testing.T, cfg deployer.Config) {
				require.NotNil(t, cfg.Central.PortForwarding, "Central.PortForwarding should be set")
				assert.True(t, *cfg.Central.PortForwarding, "Central.PortForwarding mismatch")
			},
		},
		{
			name: "port-forwarding disabled",
			args: []string{"--port-forwarding=false"},
			assert: func(t *testing.T, cfg deployer.Config) {
				require.NotNil(t, cfg.Central.PortForwarding, "Central.PortForwarding should be set")
				assert.False(t, *cfg.Central.PortForwarding, "Central.PortForwarding mismatch")
			},
		},
		{
			name: "single-namespace",
			args: []string{"--single-namespace"},
			assert: func(t *testing.T, cfg deployer.Config) {
				assert.Equal(t, "stackrox", cfg.Central.Namespace, "Central.Namespace mismatch")
				assert.Equal(t, "stackrox", cfg.SecuredCluster.Namespace, "SecuredCluster.Namespace mismatch")
			},
		},
		{
			name: "central-wait",
			args: []string{"--central-wait", "10m"},
			assert: func(t *testing.T, cfg deployer.Config) {
				assert.Equal(t, 10*time.Minute, cfg.Central.DeployTimeout, "Central.DeployTimeout mismatch")
			},
		},
		{
			name: "secured-cluster-wait",
			args: []string{"--secured-cluster-wait", "7m"},
			assert: func(t *testing.T, cfg deployer.Config) {
				assert.Equal(t, 7*time.Minute, cfg.SecuredCluster.DeployTimeout, "SecuredCluster.DeployTimeout mismatch")
			},
		},
		{
			name: "early-readiness",
			args: []string{"--early-readiness"},
			assert: func(t *testing.T, cfg deployer.Config) {
				assert.True(t, cfg.Central.EarlyReadiness, "Central.EarlyReadiness mismatch")
				assert.True(t, cfg.SecuredCluster.EarlyReadiness, "SecuredCluster.EarlyReadiness mismatch")
			},
		},
		{
			name: "disable early-readiness",
			args: []string{"--early-readiness=false"},
			assert: func(t *testing.T, cfg deployer.Config) {
				assert.False(t, cfg.Central.EarlyReadiness, "Central.EarlyReadiness mismatch")
				assert.False(t, cfg.SecuredCluster.EarlyReadiness, "SecuredCluster.EarlyReadiness mismatch")
			},
		},
		{
			name: "pause-reconciliation",
			args: []string{"--pause-reconciliation"},
			assert: func(t *testing.T, cfg deployer.Config) {
				assert.True(t, cfg.Central.PauseReconciliation, "Central.PauseReconciliation mismatch")
				assert.True(t, cfg.SecuredCluster.PauseReconciliation, "SecuredCluster.PauseReconciliation mismatch")
			},
		},
		{
			name: "olm",
			args: []string{"--olm"},
			assert: func(t *testing.T, cfg deployer.Config) {
				assert.True(t, cfg.Operator.DeployViaOlm, "Operator.DeployViaOlm mismatch")
			},
		},
		{
			name: "disable deploy-operator",
			args: []string{"--deploy-operator=false"},
			assert: func(t *testing.T, cfg deployer.Config) {
				assert.True(t, cfg.Operator.SkipDeployment, "Operator.SkipDeployment mismatch")
			},
		},
		{
			name: "multiple flags combined",
			args: []string{"--tag", "4.7.0", "--exposure", "loadbalancer", "--early-readiness", "--resources", "small"},
			assert: func(t *testing.T, cfg deployer.Config) {
				assert.Equal(t, "4.7.0", cfg.Roxie.Version, "Roxie.Version mismatch")
				require.NotNil(t, cfg.Central.Exposure, "Central.Exposure should be set")
				assert.Equal(t, types.ExposureLoadBalancer, *cfg.Central.Exposure, "Central.Exposure mismatch")
				assert.True(t, cfg.Central.EarlyReadiness, "Central.EarlyReadiness mismatch")
				assert.Equal(t, types.ResourceProfileSmall, cfg.Central.ResourceProfile, "Central.ResourceProfile mismatch")
			},
		},
		{
			name: "config file can be used",
			config: `
roxie:
  version: 1.2.3
securedCluster:
  spec:
    foo: bar
`,
			args: []string{"--config", configFilePath},
			assert: func(t *testing.T, cfg deployer.Config) {
				assert.Equal(t, "1.2.3", cfg.Roxie.Version, "Roxie.Version mismatch")
				assert.True(t,
					reflect.DeepEqual(cfg.SecuredCluster.Spec,
						map[string]interface{}{
							"foo": "bar",
						}),
					"SecuredCluster.Spec mismatch",
				)
			},
		},

		{
			name: "flags can override earlier specified config file",
			config: `
central:
  resourceProfile: small
  portForwarding: true
`,
			args: []string{"--config", configFilePath, "--port-forwarding=false", "--resources=medium"},
			assert: func(t *testing.T, cfg deployer.Config) {
				assert.Equal(t, types.ResourceProfileMedium, cfg.Central.ResourceProfile, "Central.ResourceProfile mismatch")
				require.NotNil(t, cfg.Central.PortForwarding, "Central.PortForwarding should be set")
				assert.False(t, *cfg.Central.PortForwarding, "Central.PortForwarding mismatch")
			},
		},

		{
			name: "set expressions can be used",
			args: []string{"--set", "roxie.version=0.99.1", "--set", "central.deployTimeout=4m", "--set", "securedCluster.spec.clusterName=foo"},
			assert: func(t *testing.T, cfg deployer.Config) {
				assert.Equal(t, "0.99.1", cfg.Roxie.Version, "version mismatch")
				assert.Equal(t, 4*time.Minute, cfg.Central.DeployTimeout, "Central.DeployTimeout mismatch")
				assert.Equal(t,
					map[string]interface{}{
						"clusterName": "foo",
					},
					cfg.SecuredCluster.Spec,
					"SecuredCluster.Spec mismatch",
				)
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if tt.config != "" {
				require.NoError(t, os.WriteFile(configFilePath, []byte(tt.config), 0o644))
			}
			cfg := deployer.NewConfig()
			cmd := newDeployCmd(&cfg)
			require.NoError(t, cmd.ParseFlags(tt.args))
			tt.assert(t, cfg)
		})
	}
}
