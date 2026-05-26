package deployer

import (
	"testing"

	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
)

func TestConfigureSpec_CentralEndpoint(t *testing.T) {
	tests := []struct {
		name             string
		spec             map[string]interface{}
		centralNamespace string
		expected         string
	}{
		{
			name:             "sets internal endpoint when not provided",
			spec:             map[string]interface{}{},
			centralNamespace: "acs-central",
			expected:         "central.acs-central.svc:443",
		},
		{
			name:             "sets internal endpoint with custom namespace",
			spec:             map[string]interface{}{},
			centralNamespace: "stackrox",
			expected:         "central.stackrox.svc:443",
		},
		{
			name:             "preserves user-provided endpoint",
			spec:             map[string]interface{}{"centralEndpoint": "central.example.com:443"},
			centralNamespace: "acs-central",
			expected:         "central.example.com:443",
		},
		{
			name:             "user-provided endpoint takes precedence over internal default",
			spec:             map[string]interface{}{"centralEndpoint": "10.0.0.1:443"},
			centralNamespace: "stackrox",
			expected:         "10.0.0.1:443",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			sc := &SecuredClusterConfig{
				Spec: tt.spec,
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
