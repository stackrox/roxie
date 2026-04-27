package clusterdefaults

import (
	"testing"

	"github.com/stackrox/roxie/internal/deployer"
	"github.com/stackrox/roxie/internal/logger"
	"github.com/stackrox/roxie/internal/types"
	"github.com/stretchr/testify/require"
)

func TestClusterDefaults(t *testing.T) {
	tests := []struct {
		name                string
		clusterType         types.ClusterType
		wantResourceProfile types.ResourceProfile
		wantExposure        types.Exposure
		wantPortForwarding  bool
	}{
		{
			name:                "kind cluster with default params",
			clusterType:         types.ClusterTypeKind,
			wantResourceProfile: types.ResourceProfileSmall,
			wantExposure:        types.ExposureNone,
			wantPortForwarding:  true,
		},
		{
			name:                "kind cluster with already correct params",
			clusterType:         types.ClusterTypeKind,
			wantResourceProfile: types.ResourceProfileSmall,
			wantExposure:        types.ExposureNone,
			wantPortForwarding:  true,
		},
		{
			name:                "kind cluster with partial match",
			clusterType:         types.ClusterTypeKind,
			wantResourceProfile: types.ResourceProfileSmall,
			wantExposure:        types.ExposureNone,
			wantPortForwarding:  true,
		},
		{
			name:                "unknown cluster type",
			clusterType:         types.ClusterTypeUnknown,
			wantResourceProfile: types.ResourceProfileAcsDefaults,
			wantExposure:        types.ExposureNone,
			wantPortForwarding:  false,
		},
		{
			name:                "minikube cluster",
			clusterType:         types.ClusterTypeMinikube,
			wantResourceProfile: types.ResourceProfileSmall,
			wantExposure:        types.ExposureNone,
			wantPortForwarding:  true,
		},
		{
			name:                "crc cluster",
			clusterType:         types.ClusterTypeCRC,
			wantResourceProfile: types.ResourceProfileSmall,
			wantExposure:        types.ExposureNone,
			wantPortForwarding:  true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			config := deployer.Config{}
			err := ApplyClusterDefaults(logger.New(), tt.clusterType, &config)
			require.NoError(t, err)

			gotResourceProfile := ResolveAutoResourceProfile(tt.clusterType)
			if gotResourceProfile != tt.wantResourceProfile {
				t.Errorf("Apply() resources = %v, want %v", gotResourceProfile, tt.wantResourceProfile)
			}
			if config.Central.Exposure != tt.wantExposure {
				t.Errorf("Apply() exposure = %v, want %v", config.Central.Exposure, tt.wantExposure)
			}
			if config.Central.PortForwardingEnabled() != tt.wantPortForwarding {
				t.Errorf("Apply() portForward = %v, want %v", config.Central.PortForwardingEnabled(), tt.wantPortForwarding)
			}
		})
	}
}
