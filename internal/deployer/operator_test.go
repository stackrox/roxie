package deployer

import (
	"testing"

	"github.com/stackrox/roxie/internal/constants"
	"github.com/stackrox/roxie/internal/types"
	"github.com/stretchr/testify/assert"
)

func TestNeedsOperatorPullSecrets(t *testing.T) {
	tests := []struct {
		name                       string
		instance                   OperatorInstanceConfig
		roxieConfig                RoxieConfig
		customRegistryAuthRequired *bool
		expected                   bool
	}{
		{
			name:        "default registry, non-Konflux: no pull secrets",
			instance:    OperatorInstanceConfig{},
			roxieConfig: RoxieConfig{ImageRegistry: constants.DefaultRegistry, ClusterType: types.ClusterTypeGKE},
			expected:    false,
		},
		{
			name:        "Konflux images: pull secrets needed",
			instance:    OperatorInstanceConfig{KonfluxImages: new(true)},
			roxieConfig: RoxieConfig{ImageRegistry: constants.DefaultRegistry, ClusterType: types.ClusterTypeGKE},
			expected:    true,
		},
		{
			name:        "Konflux images on a cluster type that auto-configures default-registry credentials: no pull secrets",
			instance:    OperatorInstanceConfig{KonfluxImages: new(true)},
			roxieConfig: RoxieConfig{ImageRegistry: constants.DefaultRegistry, ClusterType: types.ClusterTypeInfraOpenShift4},
			expected:    false,
		},
		{
			name:                       "private custom registry, non-Konflux: pull secrets needed",
			instance:                   OperatorInstanceConfig{},
			roxieConfig:                RoxieConfig{ImageRegistry: "quay.io/stackrox-io", ClusterType: types.ClusterTypeGKE},
			customRegistryAuthRequired: new(true),
			expected:                   true,
		},
		{
			name:                       "private custom registry is never auto-configured, even on a cluster type that auto-configures the default registry",
			instance:                   OperatorInstanceConfig{},
			roxieConfig:                RoxieConfig{ImageRegistry: "quay.io/stackrox-io", ClusterType: types.ClusterTypeInfraOpenShift4},
			customRegistryAuthRequired: new(true),
			expected:                   true,
		},
		{
			name:                       "public custom registry: no pull secrets needed",
			instance:                   OperatorInstanceConfig{},
			roxieConfig:                RoxieConfig{ImageRegistry: "quay.io/stackrox-io", ClusterType: types.ClusterTypeGKE},
			customRegistryAuthRequired: new(false),
			expected:                   false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			d := &Deployer{
				config:                     Config{Roxie: tt.roxieConfig},
				customRegistryAuthRequired: tt.customRegistryAuthRequired,
			}
			assert.Equal(t, tt.expected, d.needsOperatorPullSecrets(tt.instance))
		})
	}
}

func TestDeployer_NeedsPullSecrets(t *testing.T) {
	tests := []struct {
		name                       string
		roxie                      RoxieConfig
		customRegistryAuthRequired *bool
		expected                   bool
	}{
		{
			name:     "default registry on a cluster type that auto-configures credentials",
			roxie:    RoxieConfig{ImageRegistry: constants.DefaultRegistry, ClusterType: types.ClusterTypeInfraOpenShift4},
			expected: false,
		},
		{
			name:     "default registry on a cluster type that doesn't auto-configure credentials",
			roxie:    RoxieConfig{ImageRegistry: constants.DefaultRegistry, ClusterType: types.ClusterTypeGKE},
			expected: true,
		},
		{
			name:                       "private custom registry, even on a cluster type that auto-configures default-registry credentials",
			roxie:                      RoxieConfig{ImageRegistry: "quay.io/stackrox-io", ClusterType: types.ClusterTypeInfraOpenShift4},
			customRegistryAuthRequired: new(true),
			expected:                   true,
		},
		{
			name:                       "public custom registry: no pull secrets needed",
			roxie:                      RoxieConfig{ImageRegistry: "quay.io/stackrox-io", ClusterType: types.ClusterTypeInfraOpenShift4},
			customRegistryAuthRequired: new(false),
			expected:                   false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			d := &Deployer{
				config:                     Config{Roxie: tt.roxie},
				customRegistryAuthRequired: tt.customRegistryAuthRequired,
			}
			assert.Equal(t, tt.expected, d.NeedsPullSecrets())
		})
	}
}
