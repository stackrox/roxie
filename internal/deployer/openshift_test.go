package deployer

import (
	"encoding/base64"
	"encoding/json"
	"testing"

	"github.com/stackrox/roxie/internal/dockerauth"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	v1 "k8s.io/api/core/v1"
)

func TestInjectRegistryCredentialsIntoSecret(t *testing.T) {
	const (
		registryUsername = "user"
		registryPassword = "pass"
	)

	makeSecret := func(credentials map[string]map[string]string) *v1.Secret {
		data, err := json.Marshal(map[string]any{
			"auths": credentials,
		})
		require.NoError(t, err)
		return &v1.Secret{Data: map[string][]byte{dockerConfigJsonKey: data}}
	}

	encodeCredentials := func(username, password string) string {
		return base64.StdEncoding.EncodeToString([]byte(username + ":" + password))
	}

	tests := []struct {
		name              string
		secret            *v1.Secret
		expectModified    bool
		expectError       bool
		expectCredentials map[string]map[string]string
	}{
		{
			name:           "injects into empty auths",
			secret:         makeSecret(nil),
			expectModified: true,
			expectCredentials: map[string]map[string]string{
				registryForDownstreamImages: {
					"auth": encodeCredentials(registryUsername, registryPassword),
				},
			},
		},
		{
			name: "preserves existing entries",
			secret: makeSecret(map[string]map[string]string{
				"registry.example.com": {
					"auth": encodeCredentials("other", "secret"),
				},
			}),
			expectModified: true,
			expectCredentials: map[string]map[string]string{
				"registry.example.com": {
					"auth": encodeCredentials("other", "secret"),
				},
				registryForDownstreamImages: {
					"auth": encodeCredentials(registryUsername, registryPassword),
				},
			},
		},
		{
			name: "skips if already present",
			secret: makeSecret(map[string]map[string]string{
				registryForDownstreamImages: {
					"auth": encodeCredentials("existing", "existing"),
				},
			}),
			expectModified: false,
			expectCredentials: map[string]map[string]string{
				registryForDownstreamImages: {
					"auth": encodeCredentials("existing", "existing"),
				},
			},
		},
		{
			name:           "handles nil secret data",
			secret:         &v1.Secret{},
			expectModified: true,
			expectCredentials: map[string]map[string]string{
				registryForDownstreamImages: {
					"auth": encodeCredentials(registryUsername, registryPassword),
				},
			},
		},
		{
			name:        "returns error on invalid JSON",
			secret:      &v1.Secret{Data: map[string][]byte{dockerConfigJsonKey: []byte("not json")}},
			expectError: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			creds := dockerauth.Credentials{Username: registryUsername, Password: registryPassword}
			modified, err := injectRegistryCredentialsIntoSecret(creds, tt.secret)
			if tt.expectError {
				assert.Error(t, err)
				return
			}
			require.NoError(t, err)
			assert.Equal(t, tt.expectModified, modified)

			var cfg dockerConfigJSON
			require.NoError(t, json.Unmarshal(tt.secret.Data[dockerConfigJsonKey], &cfg))

			assert.Equal(t, len(tt.expectCredentials), len(cfg.Auths), "credential length mismatch")

			for regName, regCredentials := range tt.expectCredentials {
				assert.Equal(t, regCredentials["auth"], cfg.Auths[regName].Auth, "credentials mismatch for registry %d", regName)
			}
		})
	}
}
