package clusterdefaults

import (
	"testing"

	"github.com/stackrox/roxie/internal/deployer"
	"github.com/stackrox/roxie/internal/logger"
	"github.com/stackrox/roxie/internal/types"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"k8s.io/utils/ptr"
)

func TestClusterDefaults(t *testing.T) {
	tests := []struct {
		name                string
		clusterType         types.ClusterType
		wantResourceProfile types.ResourceProfile
		wantExposure        *types.Exposure
		wantPortForwarding  *bool
	}{
		{
			name:                "kind cluster with default params",
			clusterType:         types.ClusterTypeKind,
			wantResourceProfile: types.ResourceProfileSmall,
			wantExposure:        ptr.To(types.ExposureNone),
			wantPortForwarding:  ptr.To(true),
		},
		{
			name:                "kind cluster with already correct params",
			clusterType:         types.ClusterTypeKind,
			wantResourceProfile: types.ResourceProfileSmall,
			wantExposure:        ptr.To(types.ExposureNone),
			wantPortForwarding:  ptr.To(true),
		},
		{
			name:                "kind cluster with partial match",
			clusterType:         types.ClusterTypeKind,
			wantResourceProfile: types.ResourceProfileSmall,
			wantExposure:        ptr.To(types.ExposureNone),
			wantPortForwarding:  ptr.To(true),
		},
		{
			name:                "unknown cluster type",
			clusterType:         types.ClusterTypeUnknown,
			wantResourceProfile: types.ResourceProfileAcsDefaults,
			wantExposure:        nil,
			wantPortForwarding:  nil,
		},
		{
			name:                "minikube cluster",
			clusterType:         types.ClusterTypeMinikube,
			wantResourceProfile: types.ResourceProfileSmall,
			wantExposure:        ptr.To(types.ExposureNone),
			wantPortForwarding:  ptr.To(true),
		},
		{
			name:                "crc cluster",
			clusterType:         types.ClusterTypeCRC,
			wantResourceProfile: types.ResourceProfileSmall,
			wantExposure:        ptr.To(types.ExposureNone),
			wantPortForwarding:  ptr.To(true),
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			config := deployer.NewConfig()
			err := ApplyClusterDefaults(logger.New(), tt.clusterType, &config)
			require.NoError(t, err)

			gotResourceProfile := ResolveAutoResourceProfile(tt.clusterType)
			if gotResourceProfile != tt.wantResourceProfile {
				t.Errorf("Apply() resources = %v, want %v", gotResourceProfile, tt.wantResourceProfile)
			}

			if tt.wantExposure == nil {
				assert.Nil(t, config.Central.Exposure, "central exposure is not nil")
			} else {
				require.NotNil(t, config.Central.Exposure, "central exposure is nil")
				assert.Equal(t, *tt.wantExposure, *config.Central.Exposure,
					"exposure = %v, want %v", *config.Central.Exposure, *tt.wantExposure)
			}

			if tt.wantPortForwarding == nil {
				assert.Nil(t, config.Central.PortForwarding, "central port forwarding is not nil")
			} else {
				require.NotNil(t, config.Central.PortForwarding, "central port forwarding is nil")
				assert.Equal(t, *tt.wantPortForwarding, *config.Central.PortForwarding,
					"portForward = %v, want %v", *config.Central.PortForwarding, *tt.wantPortForwarding)
			}
		})
	}
}
