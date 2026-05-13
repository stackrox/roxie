package deployer

import (
	"testing"

	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
)

func TestConfigureSpec_CentralEndpoint(t *testing.T) {
	tests := []struct {
		name             string
		centralEndpoint  string
		centralNamespace string
		expected         string
	}{
		{
			name:             "falls back to internal endpoint",
			centralEndpoint:  "",
			centralNamespace: "acs-central",
			expected:         "central.acs-central.svc:443",
		},
		{
			name:             "falls back to internal endpoint with custom namespace",
			centralEndpoint:  "",
			centralNamespace: "stackrox",
			expected:         "central.stackrox.svc:443",
		},
		{
			name:             "uses provided central endpoint",
			centralEndpoint:  "central.example.com:443",
			centralNamespace: "acs-central",
			expected:         "central.example.com:443",
		},
		{
			name:             "provided endpoint takes precedence over namespace",
			centralEndpoint:  "10.0.0.1:443",
			centralNamespace: "stackrox",
			expected:         "10.0.0.1:443",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			sc := &SecuredClusterConfig{
				CentralEndpoint: tt.centralEndpoint,
				Spec:            make(map[string]interface{}),
			}
			roxie := &RoxieConfig{FeatureFlags: make(map[string]bool)}
			central := &CentralConfig{Namespace: tt.centralNamespace}

			if err := sc.ConfigureSpec(roxie, central); err != nil {
				t.Fatalf("ConfigureSpec failed: %v", err)
			}

			got, found, err := unstructured.NestedString(sc.Spec, "centralEndpoint")
			if err != nil {
				t.Fatalf("failed to get centralEndpoint from spec: %v", err)
			}
			if !found {
				t.Fatal("centralEndpoint not found in spec")
			}
			if got != tt.expected {
				t.Errorf("got %q, want %q", got, tt.expected)
			}
		})
	}
}
