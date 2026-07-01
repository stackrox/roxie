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
		{ClusterTypeInfraGKE, "InfraGKE"},
		{ClusterTypeInfraOpenShift4, "InfraOpenShift4"},
		{ClusterTypeOpenShift4, "OpenShift4"},
		{ClusterTypeKind, "Kind"},
		{ClusterTypeMinikube, "Minikube"},
		{ClusterTypeK3s, "K3s"},
		{ClusterTypeCRC, "CRC"},
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
		{"InfraGKE", ClusterTypeInfraGKE},
		{"InfraOpenShift4", ClusterTypeInfraOpenShift4},
		{"OpenShift4", ClusterTypeOpenShift4},
		{"Kind", ClusterTypeKind},
		{"Minikube", ClusterTypeMinikube},
		{"K3s", ClusterTypeK3s},
		{"CRC", ClusterTypeCRC},
		{"", ClusterTypeUnknown},
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
