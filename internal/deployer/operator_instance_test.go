package deployer

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestEffectiveVersions_DefaultToRoxieVersion(t *testing.T) {
	cfg := Config{
		Roxie: RoxieConfig{Version: "4.9.0"},
	}
	assert.Equal(t, "4.9.0", cfg.EffectiveCentralVersion())
	assert.Equal(t, "4.9.0", cfg.EffectiveSecuredClusterVersion())
	assert.False(t, cfg.HasMixedVersions())
}

func TestEffectiveVersions_CentralOverride(t *testing.T) {
	cfg := Config{
		Roxie:   RoxieConfig{Version: "4.9.0"},
		Central: CentralConfig{Version: "4.8.0"},
	}
	assert.Equal(t, "4.8.0", cfg.EffectiveCentralVersion())
	assert.Equal(t, "4.9.0", cfg.EffectiveSecuredClusterVersion())
	assert.True(t, cfg.HasMixedVersions())
}

func TestEffectiveVersions_SecuredClusterOverride(t *testing.T) {
	cfg := Config{
		Roxie:          RoxieConfig{Version: "4.9.0"},
		SecuredCluster: SecuredClusterConfig{Version: "4.7.0"},
	}
	assert.Equal(t, "4.9.0", cfg.EffectiveCentralVersion())
	assert.Equal(t, "4.7.0", cfg.EffectiveSecuredClusterVersion())
	assert.True(t, cfg.HasMixedVersions())
}

func TestEffectiveVersions_BothOverridesSame_NoMixed(t *testing.T) {
	cfg := Config{
		Roxie:          RoxieConfig{Version: "4.9.0"},
		Central:        CentralConfig{Version: "4.8.0"},
		SecuredCluster: SecuredClusterConfig{Version: "4.8.0"},
	}
	assert.Equal(t, "4.8.0", cfg.EffectiveCentralVersion())
	assert.Equal(t, "4.8.0", cfg.EffectiveSecuredClusterVersion())
	assert.False(t, cfg.HasMixedVersions())

	instances := cfg.OperatorInstances()
	require.Len(t, instances, 1)
	assert.Equal(t, "4.8.0", instances[0].Version, "operator should use the override version, not Roxie.Version")
}

func TestOperatorInstances_SingleVersion(t *testing.T) {
	cfg := Config{
		Roxie: RoxieConfig{Version: "4.9.0-dirty"},
		Operator: OperatorConfig{
			EnvVars: map[string]string{"FOO": "bar"},
		},
	}

	instances := cfg.OperatorInstances()
	require.Len(t, instances, 1)
	assert.Equal(t, "4.9.0", instances[0].Version)
	assert.Equal(t, operatorNamespaceSystem, instances[0].Namespace)
	assert.Equal(t, "", instances[0].RoleNameSuffix)
	assert.Equal(t, "rhacs-operator-manager-role", instances[0].ClusterRoleName())
	assert.Equal(t, "rhacs-operator-manager-rolebinding", instances[0].ClusterRoleBindingName())
	assert.Equal(t, "bar", instances[0].EnvVars["FOO"])
	assert.NotContains(t, instances[0].EnvVars, envCentralReconcilerEnabled)
	assert.NotContains(t, instances[0].EnvVars, envSecuredClusterReconcilerEnabled)
}

func TestOperatorInstances_MixedVersions(t *testing.T) {
	cfg := Config{
		Roxie: RoxieConfig{Version: "4.9.0"},
		Central: CentralConfig{
			Version: "4.8.0",
		},
		SecuredCluster: SecuredClusterConfig{
			Version: "4.9.0",
		},
		Operator: OperatorConfig{
			EnvVars: map[string]string{"CUSTOM": "1"},
		},
	}

	instances := cfg.OperatorInstances()
	require.Len(t, instances, 2)

	central := instances[0]
	assert.Equal(t, "4.8.0", central.Version)
	assert.Equal(t, operatorNamespaceCentral, central.Namespace)
	assert.Equal(t, "central", central.RoleNameSuffix)
	assert.Equal(t, "rhacs-operator-manager-role-central", central.ClusterRoleName())
	assert.Equal(t, "rhacs-operator-manager-rolebinding-central", central.ClusterRoleBindingName())
	assert.Equal(t, "1", central.EnvVars["CUSTOM"])
	assert.Equal(t, "false", central.EnvVars[envSecuredClusterReconcilerEnabled])
	assert.NotContains(t, central.EnvVars, envCentralReconcilerEnabled)

	sensor := instances[1]
	assert.Equal(t, "4.9.0", sensor.Version)
	assert.Equal(t, operatorNamespaceSensor, sensor.Namespace)
	assert.Equal(t, "sensor", sensor.RoleNameSuffix)
	assert.Equal(t, "rhacs-operator-manager-role-sensor", sensor.ClusterRoleName())
	assert.Equal(t, "rhacs-operator-manager-rolebinding-sensor", sensor.ClusterRoleBindingName())
	assert.Equal(t, "1", sensor.EnvVars["CUSTOM"])
	assert.Equal(t, "false", sensor.EnvVars[envCentralReconcilerEnabled])
	assert.NotContains(t, sensor.EnvVars, envSecuredClusterReconcilerEnabled)

	// Env var maps must be independent copies.
	central.EnvVars["CUSTOM"] = "changed"
	assert.Equal(t, "1", sensor.EnvVars["CUSTOM"])
	assert.Equal(t, "1", cfg.Operator.EnvVars["CUSTOM"])
}

func TestNewestOperatorVersion(t *testing.T) {
	t.Run("single version", func(t *testing.T) {
		cfg := Config{Roxie: RoxieConfig{Version: "4.9.0"}}
		assert.Equal(t, "4.9.0", cfg.NewestOperatorVersion())
	})

	t.Run("secured cluster newer", func(t *testing.T) {
		cfg := Config{
			Roxie:          RoxieConfig{Version: "4.11.1"},
			Central:        CentralConfig{Version: "4.10.0"},
			SecuredCluster: SecuredClusterConfig{Version: "4.11.1"},
		}
		assert.Equal(t, "4.11.1", cfg.NewestOperatorVersion())
	})

	t.Run("central newer", func(t *testing.T) {
		cfg := Config{
			Roxie:          RoxieConfig{Version: "4.11.1"},
			Central:        CentralConfig{Version: "4.11.1"},
			SecuredCluster: SecuredClusterConfig{Version: "4.10.0"},
		}
		assert.Equal(t, "4.11.1", cfg.NewestOperatorVersion())
	})

	t.Run("build suffix uses leading semver", func(t *testing.T) {
		cfg := Config{
			Roxie:          RoxieConfig{Version: "4.10.0"},
			Central:        CentralConfig{Version: "4.10.0"},
			SecuredCluster: SecuredClusterConfig{Version: "4.11.0-937-gf0da38f1a"},
		}
		assert.Equal(t, "4.11.0-937-gf0da38f1a", cfg.NewestOperatorVersion())
	})
}

func TestImagesForConfig_MixedVersions(t *testing.T) {
	cfg := Config{
		Roxie:          RoxieConfig{Version: "4.9.0"},
		Central:        CentralConfig{Version: "4.8.0"},
		SecuredCluster: SecuredClusterConfig{Version: "4.9.0"},
	}

	images := imagesForConfig(cfg)
	assert.Contains(t, images, "quay.io/rhacs-eng/main:4.8.0")
	assert.Contains(t, images, "quay.io/rhacs-eng/main:4.9.0")
	assert.Contains(t, images, "quay.io/rhacs-eng/stackrox-operator:4.8.0")
	assert.Contains(t, images, "quay.io/rhacs-eng/stackrox-operator:4.9.0")
	assert.Contains(t, images, "quay.io/rhacs-eng/stackrox-operator-bundle:v4.8.0")
	assert.Contains(t, images, "quay.io/rhacs-eng/stackrox-operator-bundle:v4.9.0")
}

func TestImagesForConfig_SingleVersionUnchanged(t *testing.T) {
	cfg := Config{
		Roxie:    RoxieConfig{Version: "4.9.0"},
		Operator: OperatorConfig{Version: "4.9.0"},
	}

	images := imagesForConfig(cfg)
	assert.Contains(t, images, "quay.io/rhacs-eng/main:4.9.0")
	assert.Contains(t, images, "quay.io/rhacs-eng/stackrox-operator:4.9.0")
	assert.Contains(t, images, "quay.io/rhacs-eng/stackrox-operator-bundle:v4.9.0")

	mainCount := 0
	for _, img := range images {
		if img == "quay.io/rhacs-eng/main:4.9.0" {
			mainCount++
		}
	}
	assert.Equal(t, 1, mainCount, "main image should appear once")
}
