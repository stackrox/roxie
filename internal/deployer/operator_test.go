package deployer

import (
	"context"
	"testing"

	"github.com/stackrox/roxie/internal/constants"
	"github.com/stackrox/roxie/internal/types"
	"github.com/stretchr/testify/assert"
)

func TestNeedsPullSecretsForOperatorManager(t *testing.T) {
	tests := []struct {
		name          string
		instance      OperatorInstanceConfig
		roxieConfig   RoxieConfig
		repoAuthCache map[string]bool
		expected      bool
	}{
		{
			name:        "default registry, non-Konflux: operator image is public, no pull secrets",
			instance:    OperatorInstanceConfig{ImageRegistry: constants.DefaultRegistry},
			roxieConfig: RoxieConfig{ImageRegistry: constants.DefaultRegistry, ClusterType: types.ClusterTypeGKE},
			repoAuthCache: map[string]bool{
				constants.DefaultRegistry + "/stackrox-operator": false,
			},
			expected: false,
		},
		{
			name:        "default registry, Konflux: operator image is private, pull secrets needed",
			instance:    OperatorInstanceConfig{KonfluxImages: new(true), ImageRegistry: constants.DefaultRegistry},
			roxieConfig: RoxieConfig{ImageRegistry: constants.DefaultRegistry, ClusterType: types.ClusterTypeGKE},
			repoAuthCache: map[string]bool{
				constants.DefaultRegistry + "/release-operator": true,
			},
			expected: true,
		},
		{
			name:        "default registry, Konflux on a cluster type that auto-configures credentials: no pull secrets",
			instance:    OperatorInstanceConfig{KonfluxImages: new(true), ImageRegistry: constants.DefaultRegistry},
			roxieConfig: RoxieConfig{ImageRegistry: constants.DefaultRegistry, ClusterType: types.ClusterTypeInfraOpenShift4},
			repoAuthCache: map[string]bool{
				constants.DefaultRegistry + "/release-operator": true,
			},
			expected: false,
		},
		{
			name:        "private custom registry: pull secrets needed",
			instance:    OperatorInstanceConfig{ImageRegistry: "quay.io/stackrox-io"},
			roxieConfig: RoxieConfig{ImageRegistry: "quay.io/stackrox-io", ClusterType: types.ClusterTypeGKE},
			repoAuthCache: map[string]bool{
				"quay.io/stackrox-io/stackrox-operator": true,
			},
			expected: true,
		},
		{
			name:        "private custom registry on InfraOpenShift4: pull secrets still needed",
			instance:    OperatorInstanceConfig{ImageRegistry: "quay.io/stackrox-io"},
			roxieConfig: RoxieConfig{ImageRegistry: "quay.io/stackrox-io", ClusterType: types.ClusterTypeInfraOpenShift4},
			repoAuthCache: map[string]bool{
				"quay.io/stackrox-io/stackrox-operator": true,
			},
			expected: true,
		},
		{
			name:        "public custom registry: no pull secrets needed",
			instance:    OperatorInstanceConfig{ImageRegistry: "quay.io/stackrox-io"},
			roxieConfig: RoxieConfig{ImageRegistry: "quay.io/stackrox-io", ClusterType: types.ClusterTypeGKE},
			repoAuthCache: map[string]bool{
				"quay.io/stackrox-io/stackrox-operator": false,
			},
			expected: false,
		},
		{
			name:        "registry with port: correctly strips tag, not port",
			instance:    OperatorInstanceConfig{ImageRegistry: "registry.io:5000/org"},
			roxieConfig: RoxieConfig{ImageRegistry: "registry.io:5000/org", ClusterType: types.ClusterTypeGKE},
			repoAuthCache: map[string]bool{
				"registry.io:5000/org/stackrox-operator": true,
			},
			expected: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			d := &Deployer{
				config:        Config{Roxie: tt.roxieConfig},
				repoAuthCache: tt.repoAuthCache,
			}
			result, err := d.needsPullSecretsForOperatorManager(context.Background(), tt.instance)
			assert.NoError(t, err)
			assert.Equal(t, tt.expected, result)
		})
	}
}

func TestRepoFromImage(t *testing.T) {
	tests := []struct {
		image    string
		expected string
		wantErr  bool
	}{
		{image: "quay.io/rhacs-eng/stackrox-operator:v5.0.0", expected: "quay.io/rhacs-eng/stackrox-operator"},
		{image: "registry.io:5000/org/operator:v1.0", expected: "registry.io:5000/org/operator"},
		{image: "quay.io/rhacs-eng/stackrox-operator", wantErr: true},
		{image: "quay.io", wantErr: true},
	}
	for _, tt := range tests {
		t.Run(tt.image, func(t *testing.T) {
			repo, err := repoFromImage(tt.image)
			if tt.wantErr {
				assert.Error(t, err)
			} else {
				assert.NoError(t, err)
				assert.Equal(t, tt.expected, repo)
			}
		})
	}
}

func TestDeployer_NeedsPullSecrets(t *testing.T) {
	tests := []struct {
		name          string
		roxie         RoxieConfig
		repoAuthCache map[string]bool
		expected      bool
	}{
		{
			name:  "default registry, InfraOpenShift4: no pull secrets",
			roxie: RoxieConfig{ImageRegistry: constants.DefaultRegistry, ClusterType: types.ClusterTypeInfraOpenShift4},
			repoAuthCache: map[string]bool{
				constants.DefaultRegistry + "/main": true,
			},
			expected: false,
		},
		{
			name:  "default registry, GKE: pull secrets needed",
			roxie: RoxieConfig{ImageRegistry: constants.DefaultRegistry, ClusterType: types.ClusterTypeGKE},
			repoAuthCache: map[string]bool{
				constants.DefaultRegistry + "/main": true,
			},
			expected: true,
		},
		{
			name:  "private custom registry on InfraOpenShift4: pull secrets still needed",
			roxie: RoxieConfig{ImageRegistry: "quay.io/stackrox-io", ClusterType: types.ClusterTypeInfraOpenShift4},
			repoAuthCache: map[string]bool{
				"quay.io/stackrox-io/main": true,
			},
			expected: true,
		},
		{
			name:  "public custom registry: no pull secrets needed",
			roxie: RoxieConfig{ImageRegistry: "quay.io/stackrox-io", ClusterType: types.ClusterTypeInfraOpenShift4},
			repoAuthCache: map[string]bool{
				"quay.io/stackrox-io/main": false,
			},
			expected: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			d := &Deployer{
				config:        Config{Roxie: tt.roxie},
				repoAuthCache: tt.repoAuthCache,
			}
			assert.Equal(t, tt.expected, d.NeedsPullSecrets(context.Background()))
		})
	}
}
