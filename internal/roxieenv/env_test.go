package roxieenv

import (
	"maps"
	"testing"

	"github.com/stackrox/roxie/internal/types"
	"github.com/stretchr/testify/assert"
)

func TestAssembleRoxieEnvironment(t *testing.T) {
	tests := []struct {
		name       string
		info       types.CentralDeploymentInfo
		wantKeys   map[string]string
		absentKeys []string
	}{
		{
			name: "all fields populated",
			info: types.CentralDeploymentInfo{
				Endpoint:    "localhost:8443",
				Username:    "admin",
				Password:    "secret123",
				KubeContext: "kind-kind",
				CACertFile:  "/tmp/ca.pem",
			},
			wantKeys: map[string]string{
				"API_ENDPOINT":       "localhost:8443",
				"ROX_ENDPOINT":       "localhost:8443",
				"ROX_BASE_URL":       "https://localhost:8443",
				"ROX_ADMIN_PASSWORD": "secret123",
				"ROX_CA_CERT_FILE":   "/tmp/ca.pem",
				"ROX_USERNAME":       "admin",
			},
		},
		{
			name: "empty endpoint omits endpoint keys",
			info: types.CentralDeploymentInfo{
				Username:   "admin",
				Password:   "secret123",
				CACertFile: "/tmp/ca.pem",
			},
			wantKeys: map[string]string{
				"ROX_ADMIN_PASSWORD": "secret123",
				"ROX_CA_CERT_FILE":   "/tmp/ca.pem",
				"ROX_USERNAME":       "admin",
			},
			absentKeys: []string{"API_ENDPOINT", "ROX_ENDPOINT", "ROX_BASE_URL"},
		},
		{
			name: "empty password omits password key",
			info: types.CentralDeploymentInfo{
				Endpoint: "localhost:8443",
			},
			wantKeys: map[string]string{
				"API_ENDPOINT": "localhost:8443",
			},
			absentKeys: []string{"ROX_ADMIN_PASSWORD"},
		},
		{
			name: "empty ca cert file omits cert key",
			info: types.CentralDeploymentInfo{
				Endpoint: "localhost:8443",
			},
			absentKeys: []string{"ROX_CA_CERT_FILE"},
		},
		{
			name:       "all fields empty",
			info:       types.CentralDeploymentInfo{},
			absentKeys: []string{"API_ENDPOINT", "ROX_ENDPOINT", "ROX_BASE_URL", "ROX_ADMIN_PASSWORD", "ROX_CA_CERT_FILE"},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			env := maps.Collect(AssembleRoxieEnvironment(tt.info).Export())
			for key, want := range tt.wantKeys {
				assert.Equal(t, want, env[key], "key %s", key)
			}
			for _, key := range tt.absentKeys {
				_, exists := env[key]
				assert.False(t, exists, "key %s should be absent", key)
			}
		})
	}
}
