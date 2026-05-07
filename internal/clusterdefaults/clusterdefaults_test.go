package clusterdefaults

import (
	"testing"

	"github.com/stackrox/roxie/internal/deployer"
	"github.com/stackrox/roxie/internal/types"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"k8s.io/utils/ptr"
)

func TestClusterDefaults(t *testing.T) {
	tests := []struct {
		name        string
		clusterType types.ClusterType
		config      deployer.Config
		wantConfig  deployer.Config
	}{
		{
			name:        "kind cluster with default params",
			clusterType: types.ClusterTypeKind,
			wantConfig: deployer.Config{
				Central: deployer.CentralConfig{
					Exposure:       ptr.To(types.ExposureNone),
					PortForwarding: ptr.To(true),
				},
			},
		},
		{
			name:        "kind cluster with already correct params",
			clusterType: types.ClusterTypeKind,
			wantConfig: deployer.Config{
				Central: deployer.CentralConfig{
					Exposure:       ptr.To(types.ExposureNone),
					PortForwarding: ptr.To(true),
				},
			},
		},
		{
			name:        "kind cluster with partial match",
			clusterType: types.ClusterTypeKind,
			wantConfig: deployer.Config{
				Central: deployer.CentralConfig{
					Exposure:       ptr.To(types.ExposureNone),
					PortForwarding: ptr.To(true),
				},
			},
		},
		{
			name:        "unknown cluster type",
			clusterType: types.ClusterTypeUnknown,
			wantConfig:  deployer.Config{},
		},
		{
			name:        "minikube cluster",
			clusterType: types.ClusterTypeMinikube,
			wantConfig: deployer.Config{
				Central: deployer.CentralConfig{
					Exposure:       ptr.To(types.ExposureNone),
					PortForwarding: ptr.To(true),
				},
			},
		},
		{
			name:        "crc cluster",
			clusterType: types.ClusterTypeCRC,
			wantConfig: deployer.Config{
				Central: deployer.CentralConfig{
					Exposure:       ptr.To(types.ExposureNone),
					PortForwarding: ptr.To(true),
				},
			},
		},
		{
			name:        "gke cluster",
			clusterType: types.ClusterTypeInfraGKE,
			wantConfig: deployer.Config{
				Central: deployer.CentralConfig{
					Exposure:       ptr.To(types.ExposureLoadBalancer),
					PortForwarding: ptr.To(false),
				},
			},
		},
		{
			name:        "openshift cluster",
			clusterType: types.ClusterTypeInfraOpenShift4,
			wantConfig: deployer.Config{
				Central: deployer.CentralConfig{
					Exposure:       ptr.To(types.ExposureLoadBalancer),
					PortForwarding: ptr.To(false),
				},
			},
		},
		{
			name:        "cluster does not override existing values",
			clusterType: types.ClusterTypeInfraGKE,
			config: deployer.Config{
				Central: deployer.CentralConfig{
					Exposure:       ptr.To(types.ExposureNone),
					PortForwarding: ptr.To(true),
				},
			},
			wantConfig: deployer.Config{
				Central: deployer.CentralConfig{
					Exposure:       ptr.To(types.ExposureNone),
					PortForwarding: ptr.To(true),
				},
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			config := tt.config
			_, err := ApplyClusterDefaults(tt.clusterType, &config)
			require.NoError(t, err)

			if tt.wantConfig.Central.Exposure == nil {
				assert.Nil(t, config.Central.Exposure, "central exposure is not nil")
			} else {
				require.NotNil(t, config.Central.Exposure, "central exposure is nil")
				assert.Equal(t, *tt.wantConfig.Central.Exposure, *config.Central.Exposure,
					"exposure = %v, want %v", *config.Central.Exposure, *tt.wantConfig.Central.Exposure)
			}

			if tt.wantConfig.Central.PortForwarding == nil {
				assert.Nil(t, config.Central.PortForwarding, "central port forwarding is not nil")
			} else {
				require.NotNil(t, config.Central.PortForwarding, "central port forwarding is nil")
				assert.Equal(t, *tt.wantConfig.Central.PortForwarding, *config.Central.PortForwarding,
					"portForward = %v, want %v", *config.Central.PortForwarding, *tt.wantConfig.Central.PortForwarding)
			}
		})
	}
}

func TestResolveAutoResourceProfile(t *testing.T) {
	tests := []struct {
		name        string
		clusterType types.ClusterType
		want        types.ResourceProfile
	}{
		{
			name:        "kind cluster",
			clusterType: types.ClusterTypeKind,
			want:        types.ResourceProfileSmall,
		},
		{
			name:        "minikube cluster",
			clusterType: types.ClusterTypeMinikube,
			want:        types.ResourceProfileSmall,
		},
		{
			name:        "k3s cluster",
			clusterType: types.ClusterTypeK3s,
			want:        types.ResourceProfileSmall,
		},
		{
			name:        "crc cluster",
			clusterType: types.ClusterTypeCRC,
			want:        types.ResourceProfileSmall,
		},
		{
			name:        "gke cluster",
			clusterType: types.ClusterTypeInfraGKE,
			want:        types.ResourceProfileMedium,
		},
		{
			name:        "openshift cluster",
			clusterType: types.ClusterTypeInfraOpenShift4,
			want:        types.ResourceProfileMedium,
		},
		{
			name:        "unknown cluster type",
			clusterType: types.ClusterTypeUnknown,
			want:        types.ResourceProfileAcsDefaults,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := ResolveAutoResourceProfile(tt.clusterType)
			assert.Equal(t, tt.want, got)
		})
	}
}
