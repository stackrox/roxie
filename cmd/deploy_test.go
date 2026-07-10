package main

import (
	"os"
	"path/filepath"
	"reflect"
	"testing"
	"time"

	"dario.cat/mergo"
	"github.com/stackrox/roxie/internal/deployer"
	"github.com/stackrox/roxie/internal/logger"
	"github.com/stackrox/roxie/internal/paths"
	"github.com/stackrox/roxie/internal/types"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"gopkg.in/yaml.v3"
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
				assert.True(t, cfg.Central.EarlyReadinessEnabled(), "Central.EarlyReadiness mismatch")
				assert.True(t, cfg.SecuredCluster.EarlyReadinessEnabled(), "SecuredCluster.EarlyReadiness mismatch")
			},
		},
		{
			name: "disable early-readiness",
			args: []string{"--early-readiness=false"},
			assert: func(t *testing.T, cfg deployer.Config) {
				assert.False(t, cfg.Central.EarlyReadinessEnabled(), "Central.EarlyReadiness mismatch")
				assert.False(t, cfg.SecuredCluster.EarlyReadinessEnabled(), "SecuredCluster.EarlyReadiness mismatch")
			},
		},
		{
			name: "pause-reconciliation",
			args: []string{"--pause-reconciliation"},
			assert: func(t *testing.T, cfg deployer.Config) {
				assert.True(t, cfg.Central.PauseReconciliationEnabled(), "Central.PauseReconciliation mismatch")
				assert.True(t, cfg.SecuredCluster.PauseReconciliationEnabled(), "SecuredCluster.PauseReconciliation mismatch")
			},
		},
		{
			name: "olm",
			args: []string{"--olm"},
			assert: func(t *testing.T, cfg deployer.Config) {
				assert.True(t, cfg.Operator.DeployViaOlmEnabled(), "Operator.DeployViaOlm mismatch")
			},
		},
		{
			name: "disable deploy-operator",
			args: []string{"--deploy-operator=false"},
			assert: func(t *testing.T, cfg deployer.Config) {
				assert.True(t, cfg.Operator.SkipDeploymentEnabled(), "Operator.SkipDeployment mismatch")
			},
		},
		{
			name: "multiple flags combined",
			args: []string{"--tag", "4.7.0", "--exposure", "loadbalancer", "--early-readiness", "--resources", "small"},
			assert: func(t *testing.T, cfg deployer.Config) {
				assert.Equal(t, "4.7.0", cfg.Roxie.Version, "Roxie.Version mismatch")
				require.NotNil(t, cfg.Central.Exposure, "Central.Exposure should be set")
				assert.Equal(t, types.ExposureLoadBalancer, *cfg.Central.Exposure, "Central.Exposure mismatch")
				assert.True(t, cfg.Central.EarlyReadinessEnabled(), "Central.EarlyReadiness mismatch")
				assert.True(t, cfg.SecuredCluster.EarlyReadinessEnabled(), "SecuredCluster.EarlyReadiness mismatch")
				assert.Equal(t, types.ResourceProfileSmall, cfg.Central.ResourceProfile, "Central.ResourceProfile mismatch")
			},
		},
		{
			name: "operator-env single",
			args: []string{"--operator-env", "RELATED_IMAGE_MAIN=quay.io/main:4.7.0"},
			assert: func(t *testing.T, cfg deployer.Config) {
				require.NotNil(t, cfg.Operator.EnvVars, "Operator.EnvVars should be set")
				assert.Equal(t, "quay.io/main:4.7.0", cfg.Operator.EnvVars["RELATED_IMAGE_MAIN"])
			},
		},
		{
			name: "operator-env containing commas",
			args: []string{"--operator-env", "FOO=bar,BAZ=qux,quux"},
			assert: func(t *testing.T, cfg deployer.Config) {
				require.NotNil(t, cfg.Operator.EnvVars, "Operator.EnvVars should be set")
				assert.Equal(t, "bar,BAZ=qux,quux", cfg.Operator.EnvVars["FOO"])
				assert.NotContains(t, cfg.Operator.EnvVars, "BAZ")
			},
		},
		{
			name: "operator-env multiple flags",
			args: []string{"--operator-env", "FOO=bar", "--operator-env", "BAZ=qux"},
			assert: func(t *testing.T, cfg deployer.Config) {
				require.NotNil(t, cfg.Operator.EnvVars, "Operator.EnvVars should be set")
				assert.Equal(t, "bar", cfg.Operator.EnvVars["FOO"])
				assert.Equal(t, "qux", cfg.Operator.EnvVars["BAZ"])
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
			name: "config file with operator env vars",
			config: `
operator:
  envVars:
    RELATED_IMAGE_MAIN: quay.io/rhacs-eng/main:4.7.0
    RELATED_IMAGE_SCANNER: quay.io/rhacs-eng/scanner:4.7.0
`,
			args: []string{"--config", configFilePath},
			assert: func(t *testing.T, cfg deployer.Config) {
				require.NotNil(t, cfg.Operator.EnvVars, "Operator.EnvVars should be set")
				assert.Equal(t, "quay.io/rhacs-eng/main:4.7.0", cfg.Operator.EnvVars["RELATED_IMAGE_MAIN"])
				assert.Equal(t, "quay.io/rhacs-eng/scanner:4.7.0", cfg.Operator.EnvVars["RELATED_IMAGE_SCANNER"])
			},
		},

		{
			name: "config file can disable early-readiness",
			config: `
central:
  earlyReadiness: false
`,
			args: []string{"--config", configFilePath},
			assert: func(t *testing.T, cfg deployer.Config) {
				assert.False(t, cfg.Central.EarlyReadinessEnabled(), "Central.EarlyReadiness should be false")
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
		{
			name: "set expressions support array index notation",
			args: []string{
				"--set", "central.spec.central.defaultTLSSecret[0].name=frontend-cert",
			},
			assert: func(t *testing.T, cfg deployer.Config) {
				assert.Equal(t,
					map[string]interface{}{
						"central": map[string]interface{}{
							"defaultTLSSecret": []interface{}{
								map[string]interface{}{
									"name": "frontend-cert",
								},
							},
						},
					},
					cfg.Central.Spec,
					"Central.Spec mismatch",
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

func TestNewDeployCmd_SetRejectsSpec(t *testing.T) {
	tests := []struct {
		name string
		args []string
	}{
		{
			name: "bare spec key",
			args: []string{"--set", "spec.central.image=foo"},
		},
		{
			name: "spec key in multi-assignment",
			args: []string{"--set", "central.deployTimeout=4m,spec.central.image=foo"},
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			cfg := deployer.NewConfig()
			cmd := newDeployCmd(&cfg)
			err := cmd.ParseFlags(tt.args)
			require.Error(t, err)
			assert.Contains(t, err.Error(), "spec")
		})
	}
}

func TestApplyUserDefaults(t *testing.T) {
	log := logger.New()

	tests := []struct {
		name     string
		config   deployer.Config
		user     deployer.Config
		expected deployer.Config
	}{
		{
			name: "empty user config leaves config unchanged",
			config: deployer.Config{
				Roxie: deployer.RoxieConfig{Version: "4.5.0"},
				Central: deployer.CentralConfig{
					Namespace: "custom-namespace",
				},
			},
			expected: deployer.Config{
				Roxie: deployer.RoxieConfig{Version: "4.5.0"},
				Central: deployer.CentralConfig{
					Namespace: "custom-namespace",
				},
			},
		},
		{
			name:   "fills empty fields from user defaults",
			config: deployer.Config{},
			user: deployer.Config{
				Roxie:    deployer.RoxieConfig{Version: "4.5.0"},
				Operator: deployer.OperatorConfig{DeployViaOlm: new(true)},
			},
			expected: deployer.Config{
				Roxie:    deployer.RoxieConfig{Version: "4.5.0"},
				Operator: deployer.OperatorConfig{DeployViaOlm: new(true)},
			},
		},
		{
			name: "user config overrides any config fields including config defaults",
			config: deployer.Config{
				Roxie: deployer.RoxieConfig{
					Version: "4.9.2",
				},
				Central: deployer.CentralConfig{
					EarlyReadiness: new(true),
				},
			},
			user: deployer.Config{
				Roxie: deployer.RoxieConfig{
					Version: "4.5.0",
				},
				Operator: deployer.OperatorConfig{
					DeployViaOlm: new(true),
				},
				Central: deployer.CentralConfig{
					Namespace:      "custom-namespace",
					EarlyReadiness: new(false),
				},
			},
			expected: deployer.Config{
				Roxie: deployer.RoxieConfig{
					Version: "4.5.0",
				},
				Operator: deployer.OperatorConfig{
					DeployViaOlm: new(true),
				},
				Central: deployer.CentralConfig{
					Namespace:      "custom-namespace",
					EarlyReadiness: new(false),
				},
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			tmpDir := t.TempDir()
			t.Setenv("XDG_CONFIG_HOME", tmpDir)
			t.Setenv("HOME", tmpDir) // For non-Unix systems.

			if !reflect.DeepEqual(tt.user, deployer.Config{}) {
				configPath, err := paths.UserConfigPath(false)
				require.NoError(t, err)
				require.NoError(t, os.MkdirAll(filepath.Dir(configPath), 0o755))
				data, err := yaml.Marshal(tt.user)
				require.NoError(t, err)
				require.NoError(t, os.WriteFile(configPath, data, 0o644))
			}

			cfg := deployer.NewConfig()
			require.NoError(t, mergo.Merge(&cfg, &tt.config, mergo.WithOverride, mergo.WithoutDereference))
			require.NoError(t, tryApplyUserDefaults(log, &cfg))

			expected := deployer.NewConfig()
			require.NoError(t, mergo.Merge(&expected, &tt.expected, mergo.WithOverride, mergo.WithoutDereference))

			assert.True(t, reflect.DeepEqual(expected, cfg), "expected %+v, got %+v", expected, cfg)
		})
	}

	t.Run("returns error on invalid yaml", func(t *testing.T) {
		tmpDir := t.TempDir()
		t.Setenv("XDG_CONFIG_HOME", tmpDir)
		t.Setenv("HOME", tmpDir) // For non-Unix systems.

		configPath, err := paths.UserConfigPath(false)
		require.NoError(t, err)
		require.NoError(t, os.MkdirAll(filepath.Dir(configPath), 0o755))
		require.NoError(t, os.WriteFile(configPath, []byte(`invalid: [yaml`), 0o644))

		cfg := deployer.NewConfig()
		assert.Error(t, tryApplyUserDefaults(log, &cfg))
	})
}
