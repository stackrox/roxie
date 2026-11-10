package types

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"gopkg.in/yaml.v3"
)

func TestClusterTypeMarshalYAML(t *testing.T) {
	tests := []struct {
		clusterType ClusterType
		expected    string
	}{
		{ClusterTypeGKE, "GKE"},
		{ClusterTypeInfraGKE, "InfraGKE"},
		{ClusterTypeInfraOpenShift4, "InfraOpenShift4"},
		{ClusterTypeOpenShift4, "OpenShift4"},
		{ClusterTypeKind, "Kind"},
		{ClusterTypeMinikube, "Minikube"},
		{ClusterTypeK3s, "K3s"},
		{ClusterTypeCRC, "CRC"},
		{ClusterTypeAKS, "AKS"},
		{ClusterTypeEKS, "EKS"},
	}
	for _, tt := range tests {
		t.Run(tt.expected, func(t *testing.T) {
			out, err := yaml.Marshal(tt.clusterType)
			require.NoError(t, err)
			assert.Equal(t, tt.expected+"\n", string(out))
		})
	}
}

func TestClusterTypeUnmarshalYAML(t *testing.T) {
	tests := []struct {
		input    string
		expected ClusterType
	}{
		{"GKE", ClusterTypeGKE},
		{"InfraGKE", ClusterTypeInfraGKE},
		{"InfraOpenShift4", ClusterTypeInfraOpenShift4},
		{"OpenShift4", ClusterTypeOpenShift4},
		{"Kind", ClusterTypeKind},
		{"Minikube", ClusterTypeMinikube},
		{"K3s", ClusterTypeK3s},
		{"CRC", ClusterTypeCRC},
		{"Unknown", ClusterTypeUnknown},
		{"AKS", ClusterTypeAKS},
		{"EKS", ClusterTypeEKS},
	}
	for _, tt := range tests {
		t.Run(tt.input, func(t *testing.T) {
			var ct ClusterType
			err := yaml.Unmarshal([]byte(tt.input), &ct)
			require.NoError(t, err)
			assert.Equal(t, tt.expected, ct)
		})
	}
}

func TestClusterTypeClassification(t *testing.T) {
	tests := []struct {
		clusterType                     ClusterType
		isOpenShift                     bool
		isLocal                         bool
		supportsLoadBalancer            bool
		needsDefaultRegistryPullSecrets bool
	}{
		{
			clusterType:                     ClusterTypeCRC,
			isOpenShift:                     true,
			isLocal:                         true,
			supportsLoadBalancer:            false,
			needsDefaultRegistryPullSecrets: true,
		},
		{
			clusterType:                     ClusterTypeOpenShift4,
			isOpenShift:                     true,
			isLocal:                         false,
			supportsLoadBalancer:            true,
			needsDefaultRegistryPullSecrets: true,
		},
		{
			clusterType:                     ClusterTypeInfraOpenShift4,
			isOpenShift:                     true,
			isLocal:                         false,
			supportsLoadBalancer:            true,
			needsDefaultRegistryPullSecrets: false,
		},
		{
			clusterType:                     ClusterTypeKind,
			isOpenShift:                     false,
			isLocal:                         true,
			supportsLoadBalancer:            false,
			needsDefaultRegistryPullSecrets: true,
		},
		{
			clusterType:                     ClusterTypeMinikube,
			isOpenShift:                     false,
			isLocal:                         true,
			supportsLoadBalancer:            false,
			needsDefaultRegistryPullSecrets: true,
		},
		{
			clusterType:                     ClusterTypeGKE,
			isOpenShift:                     false,
			isLocal:                         false,
			supportsLoadBalancer:            true,
			needsDefaultRegistryPullSecrets: true,
		},
		{
			clusterType:                     ClusterTypeAKS,
			isOpenShift:                     false,
			isLocal:                         false,
			supportsLoadBalancer:            true,
			needsDefaultRegistryPullSecrets: true,
		},
		{
			clusterType:                     ClusterTypeEKS,
			isOpenShift:                     false,
			isLocal:                         false,
			supportsLoadBalancer:            true,
			needsDefaultRegistryPullSecrets: true,
		},
	}
	for _, tt := range tests {
		t.Run(string(tt.clusterType), func(t *testing.T) {
			assert.Equal(t, tt.isOpenShift, tt.clusterType.IsOpenShift(), "IsOpenShift")
			assert.Equal(t, tt.isLocal, tt.clusterType.IsLocal(), "IsLocal")
			assert.Equal(t, tt.supportsLoadBalancer, tt.clusterType.SupportsLoadBalancer(), "SupportsLoadBalancer")
			assert.Equal(t, tt.needsDefaultRegistryPullSecrets, tt.clusterType.NeedsDefaultRegistryPullSecrets(), "NeedsDefaultRegistryPullSecrets")
		})
	}
}

func TestClusterTypeUnmarshalYAML_Invalid(t *testing.T) {
	var ct ClusterType
	err := yaml.Unmarshal([]byte("bogus"), &ct)
	assert.ErrorContains(t, err, "unknown cluster type identifier")
}

func TestClusterTypeRoundTrip(t *testing.T) {
	for _, ct := range AllClusterTypes() {
		t.Run(ct.String(), func(t *testing.T) {
			out, err := yaml.Marshal(ct)
			require.NoError(t, err)

			var parsed ClusterType
			require.NoError(t, yaml.Unmarshal(out, &parsed))
			assert.Equal(t, ct, parsed)
		})
	}
}
