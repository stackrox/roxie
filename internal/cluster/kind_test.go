package cluster

import (
	"testing"
)

func TestIsKindCluster(t *testing.T) {
	tests := []struct {
		name        string
		contextName string
		expected    bool
	}{
		{
			name:        "kind cluster with prefix",
			contextName: "kind-acs",
			expected:    true,
		},
		{
			name:        "kind cluster just kind",
			contextName: "kind",
			expected:    true,
		},
		{
			name:        "non-kind cluster",
			contextName: "gke_project_zone_cluster",
			expected:    false,
		},
		{
			name:        "empty context",
			contextName: "",
			expected:    false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := isKindContext(tt.contextName)
			if result != tt.expected {
				t.Errorf("isKindContext(%q) = %v, want %v", tt.contextName, result, tt.expected)
			}
		})
	}
}

func TestExtractKindClusterName(t *testing.T) {
	tests := []struct {
		name        string
		contextName string
		expected    string
	}{
		{
			name:        "kind with cluster name",
			contextName: "kind-acs",
			expected:    "acs",
		},
		{
			name:        "just kind",
			contextName: "kind",
			expected:    "kind",
		},
		{
			name:        "kind with dashes",
			contextName: "kind-my-cluster-name",
			expected:    "my-cluster-name",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := ExtractKindClusterName(tt.contextName)
			if result != tt.expected {
				t.Errorf("ExtractKindClusterName(%q) = %v, want %v", tt.contextName, result, tt.expected)
			}
		})
	}
}
