package dockerauth

import (
	"context"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/stackrox/roxie/internal/constants"
	"github.com/stackrox/roxie/internal/logger"
	"github.com/stretchr/testify/assert"
)

func TestGetAndVerifyCredentialsFromEnv(t *testing.T) {
	// Set environment variables for test
	t.Setenv("REGISTRY_USERNAME", "user")
	t.Setenv("REGISTRY_PASSWORD", "pass")

	log := logger.New()
	da := New(log)
	da.skipCredVerification = true // Skip verification in tests

	creds, err := da.GetAndVerifyCredentials(context.Background(), constants.DefaultRegistry)
	if err != nil {
		t.Fatalf("GetAndVerifyCredentials failed: %v", err)
	}

	if creds.Username != "user" {
		t.Errorf("Expected username 'user', got '%s'", creds.Username)
	}
	if creds.Password != "pass" {
		t.Errorf("Expected password 'pass', got '%s'", creds.Password)
	}

	// Test creating YAML from credentials
	yamlText := da.CreatePullSecretYAMLFromCredentials(*creds, "ns", "registry.example.com/some-org")

	// Verify YAML structure
	if !strings.Contains(yamlText, "apiVersion: v1") {
		t.Error("YAML should contain 'apiVersion: v1'")
	}
	if !strings.Contains(yamlText, "kind: Secret") {
		t.Error("YAML should contain 'kind: Secret'")
	}
	if !strings.Contains(yamlText, "ns") {
		t.Error("YAML should contain namespace 'ns'")
	}

	// Extract and verify the base64 encoded dockerconfigjson
	lines := strings.Split(yamlText, "\n")
	var encodedConfig string
	for _, line := range lines {
		if strings.Contains(line, ".dockerconfigjson:") {
			parts := strings.SplitN(line, ":", 2)
			if len(parts) == 2 {
				encodedConfig = strings.TrimSpace(parts[1])
				break
			}
		}
	}

	if encodedConfig == "" {
		t.Fatal("Could not find .dockerconfigjson in YAML")
	}

	// Decode and verify it's valid JSON
	decoded, err := base64.StdEncoding.DecodeString(encodedConfig)
	if err != nil {
		t.Fatalf("Failed to decode base64: %v", err)
	}

	var data map[string]interface{}
	if err := json.Unmarshal(decoded, &data); err != nil {
		t.Fatalf("Decoded data is not valid JSON: %v", err)
	}

	auths, ok := data["auths"].(map[string]interface{})
	if !ok {
		t.Fatal("Decoded JSON should contain 'auths' key")
	}
	if _, ok := auths["registry.example.com"]; !ok {
		t.Errorf("Expected auths to be keyed by the registry host 'registry.example.com', got %v", auths)
	}
}

func TestGetAndVerifyCredentialsNoCredentials(t *testing.T) {
	// Ensure no credentials are set
	t.Setenv("REGISTRY_USERNAME", "")
	t.Setenv("REGISTRY_PASSWORD", "")

	// Use a temporary home directory to simulate missing credentials.
	t.Setenv("HOME", t.TempDir())

	log := logger.New()
	da := New(log)
	da.skipCredVerification = true // Skip verification in tests

	_, err := da.GetAndVerifyCredentials(context.Background(), constants.DefaultRegistry)
	if err == nil {
		t.Error("Expected error when no credentials are available")
	}
}

func TestRegistryRequiresAuth(t *testing.T) {
	tests := []struct {
		name             string
		challengeAuth    bool // whether /v2/ demands a Bearer challenge at all
		tokenStatus      int  // status the token endpoint returns, if challenged
		tagsListStatus   int  // status the tags-list request returns
		expectedRequires bool
		expectErr        bool
	}{
		{
			name:             "no auth mechanism: public",
			challengeAuth:    false,
			tagsListStatus:   http.StatusOK,
			expectedRequires: false,
		},
		{
			name:             "anonymous token granted, public repository",
			challengeAuth:    true,
			tokenStatus:      http.StatusOK,
			tagsListStatus:   http.StatusOK,
			expectedRequires: false,
		},
		{
			name:             "anonymous token granted, private repository",
			challengeAuth:    true,
			tokenStatus:      http.StatusOK,
			tagsListStatus:   http.StatusUnauthorized,
			expectedRequires: true,
		},
		{
			name: "anonymous token granted, private repository hidden behind 404",
			// Some registries return 404 instead of 401/403 for private
			// repositories, to avoid leaking their existence to unauthenticated
			// callers.
			challengeAuth:    true,
			tokenStatus:      http.StatusOK,
			tagsListStatus:   http.StatusNotFound,
			expectedRequires: true,
		},
		{
			name:             "anonymous token request itself rejected",
			challengeAuth:    true,
			tokenStatus:      http.StatusUnauthorized,
			expectedRequires: true,
			expectErr:        true,
		},
		{
			name: "tags-list request fails with a server error",
			// A transient 5xx doesn't tell us whether the registry is public or
			// private, so this fails safe by reporting that auth is required,
			// rather than treating it as confirmed "public".
			challengeAuth:    true,
			tokenStatus:      http.StatusOK,
			tagsListStatus:   http.StatusInternalServerError,
			expectedRequires: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			var registryAddr string

			mux := http.NewServeMux()
			mux.HandleFunc("/v2/", func(w http.ResponseWriter, r *http.Request) {
				if !tt.challengeAuth {
					w.WriteHeader(http.StatusOK)
					return
				}
				w.Header().Set("WWW-Authenticate", fmt.Sprintf(`Bearer realm="http://%s/token",service="test-registry"`, registryAddr))
				w.WriteHeader(http.StatusUnauthorized)
			})
			mux.HandleFunc("/token", func(w http.ResponseWriter, r *http.Request) {
				if tt.tokenStatus != http.StatusOK {
					w.WriteHeader(tt.tokenStatus)
					return
				}
				w.Header().Set("Content-Type", "application/json")
				_, _ = w.Write([]byte(`{"token":"fake-anonymous-token"}`))
			})
			mux.HandleFunc("/v2/some-org/main/tags/list", func(w http.ResponseWriter, r *http.Request) {
				w.WriteHeader(tt.tagsListStatus)
			})

			server := httptest.NewServer(mux)
			defer server.Close()
			registryAddr = strings.TrimPrefix(server.URL, "http://")

			da := &DockerAuth{logger: logger.New()}
			requiresAuth, err := da.RegistryRequiresAuth(context.Background(), registryAddr+"/some-org")
			assert.Equal(t, tt.expectedRequires, requiresAuth)
			if tt.expectErr {
				assert.Error(t, err)
			} else {
				assert.NoError(t, err)
			}
		})
	}
}

func TestSplitRegistryHost(t *testing.T) {
	tests := []struct {
		name         string
		registry     string
		expectedHost string
		expectedPath string
	}{
		{"default registry", constants.DefaultRegistry, "quay.io", "rhacs-eng"},
		{"quay.io with org", "quay.io/stackrox-io", "quay.io", "stackrox-io"},
		{"registry with port and nested path", "registry.io:5000/org/suborg", "registry.io:5000", "org/suborg"},
		{"just hostname", "justahost", "justahost", ""},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			host, path := splitRegistryHost(tt.registry)
			if host != tt.expectedHost {
				t.Errorf("splitRegistryHost(%q): expected host %q, got %q", tt.registry, tt.expectedHost, host)
			}
			if path != tt.expectedPath {
				t.Errorf("splitRegistryHost(%q): expected path %q, got %q", tt.registry, tt.expectedPath, path)
			}
		})
	}
}
