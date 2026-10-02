package dockerauth

import (
	"context"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/stackrox/roxie/internal/constants"
	"github.com/stackrox/roxie/internal/logger"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestGetAndVerifyCredentialsFromEnv(t *testing.T) {
	// Set environment variables for test
	t.Setenv("REGISTRY_USERNAME", "user")
	t.Setenv("REGISTRY_PASSWORD", "pass")

	log := logger.New()
	da := New(log)
	da.skipCredVerification = true // Skip verification in tests
	da.authFiles = []string{}

	testGetAndVerifyCredentials(t, da)
}

func TestGetAndVerifyCredentialsFromAuthFile(t *testing.T) {
	tests := []struct {
		name     string
		authFile string
	}{
		{
			name:     "docker style auth path",
			authFile: filepath.Join(t.TempDir(), ".docker", "config.json"),
		}, {
			name:     "podman style auth path",
			authFile: filepath.Join(t.TempDir(), ".config", "containers", "auth.json"),
		}, {
			name:     "podman XDG style auth path",
			authFile: filepath.Join(t.TempDir(), "containers", "auth.json"),
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			setupMockAuthEnvironment(t)
			authFile := createMockAuthFile(t, tt.authFile)

			log := logger.New()
			da := New(log)
			da.skipCredVerification = true // Skip verification in tests
			da.authFiles = []string{authFile}

			testGetAndVerifyCredentials(t, da)
		})
	}
}

func testGetAndVerifyCredentials(t *testing.T, da *DockerAuth) {
	creds, err := da.GetAndVerifyCredentials(t.Context(), constants.DefaultRegistry)
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
	require.True(t, ok, "Decoded JSON should contain 'auths' key")
	require.Containsf(t, auths, "registry.example.com", "Expected auths to be keyed by the registry host 'registry.example.com', got %v", auths)
}

func TestGetAndVerifyCredentialsNoCredentials(t *testing.T) {
	// Ensure no credentials are set
	t.Setenv("REGISTRY_USERNAME", "")
	t.Setenv("REGISTRY_PASSWORD", "")

	// Use temporary directories to simulate missing credentials.
	setupMockAuthEnvironment(t)

	log := logger.New()
	da := New(log)
	da.skipCredVerification = true // Skip verification in tests
	da.authFiles = []string{}

	_, err := da.GetAndVerifyCredentials(t.Context(), constants.DefaultRegistry)
	assert.Errorf(t, err, "Expected error when no credentials are available")
}

func TestRepositoryRequiresAuth(t *testing.T) {
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
		},
		{
			name: "tags-list request returns an unexpected server error",
			// A transient 5xx doesn't tell us whether the registry is public or private.
			challengeAuth:    true,
			tokenStatus:      http.StatusOK,
			tagsListStatus:   http.StatusInternalServerError,
			expectedRequires: true,
			expectErr:        true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			registryAddr, cleanup := newFakeRegistry(t, tt.challengeAuth, tt.tokenStatus, tt.tagsListStatus)
			defer cleanup()

			da := &DockerAuth{logger: logger.New()}
			requiresAuth, err := da.RepositoryRequiresAuth(context.Background(), registryAddr+"/some-org/some-repo")
			assert.Equal(t, tt.expectedRequires, requiresAuth)
			if tt.expectErr {
				assert.Error(t, err)
			} else {
				assert.NoError(t, err)
			}
		})
	}
}

func TestFindAuthConfigPath(t *testing.T) {
	tests := []struct {
		name                string
		mockAuthPaths       []string
		expectAuthFileIndex uint32
		expectErr           bool
	}{
		{
			"empty auth types",
			[]string{},
			0,
			true,
		},
		{
			"docker style auth path",
			[]string{filepath.Join(t.TempDir(), ".docker", "config.json")},
			0,
			false,
		},
		{
			"podman style auth path",
			[]string{filepath.Join(t.TempDir(), ".config", "containers", "auth.json")},
			0,
			false,
		},
		{
			"podman XDG style auth path",
			[]string{filepath.Join(t.TempDir(), "containers", "auth.json")},
			0,
			false,
		},
		{
			"Use first path",
			[]string{
				filepath.Join(t.TempDir(), ".docker", "config.json"),
				filepath.Join(t.TempDir(), ".config", "containers", "auth.json"),
				filepath.Join(t.TempDir(), "containers", "auth.json"),
			},
			0,
			false,
		},
		{
			"Use middle path",
			[]string{
				filepath.Join(t.TempDir(), ".docker", "config.json"),
				filepath.Join(t.TempDir(), ".config", "containers", "auth.json"),
				filepath.Join(t.TempDir(), "containers", "auth.json"),
			},
			1,
			false,
		}, {
			"Use last path",
			[]string{
				filepath.Join(t.TempDir(), ".docker", "config.json"),
				filepath.Join(t.TempDir(), ".config", "containers", "auth.json"),
				filepath.Join(t.TempDir(), "containers", "auth.json"),
			},
			2,
			false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if !tt.expectErr {
				createMockAuthFile(t, tt.mockAuthPaths[tt.expectAuthFileIndex])
			}

			log := logger.New()
			da := New(log)
			da.authFiles = tt.mockAuthPaths

			authFile, err := da.findAuthConfigPath()
			if tt.expectErr {
				assert.Error(t, err)
				return
			} else {
				assert.NoError(t, err)
				assert.Equal(t, authFile, tt.mockAuthPaths[tt.expectAuthFileIndex])
			}
		})
	}
}

// newFakeRegistry starts an httptest server that simulates an OCI registry's
// authentication and tags-list endpoints.
func newFakeRegistry(t *testing.T, challengeAuth bool, tokenStatus, tagsListStatus int) (string, func()) {
	t.Helper()
	var registryAddr string

	mux := http.NewServeMux()
	mux.HandleFunc("/v2/", func(w http.ResponseWriter, r *http.Request) {
		if !challengeAuth {
			w.WriteHeader(http.StatusOK)
			return
		}
		w.Header().Set("WWW-Authenticate", fmt.Sprintf(`Bearer realm="http://%s/token",service="test-registry"`, registryAddr))
		w.WriteHeader(http.StatusUnauthorized)
	})
	mux.HandleFunc("/token", func(w http.ResponseWriter, r *http.Request) {
		if tokenStatus != http.StatusOK {
			w.WriteHeader(tokenStatus)
			return
		}
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"token":"fake-anonymous-token"}`))
	})
	mux.HandleFunc("/v2/some-org/some-repo/tags/list", func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(tagsListStatus)
	})

	server := httptest.NewServer(mux)
	registryAddr = strings.TrimPrefix(server.URL, "http://")
	return registryAddr, server.Close
}

func setupMockAuthEnvironment(t *testing.T) {
	t.Setenv("HOME", t.TempDir())
	t.Setenv("XDG_RUNTIME_DIR", t.TempDir())
}

func createMockAuthFile(t *testing.T, authFile string) string {
	err := os.MkdirAll(filepath.Dir(authFile), 0755)
	if err != nil {
		t.Fatalf("Auth directory creation failed: %s", err)
	}

	f, err := os.OpenFile(authFile, os.O_CREATE|os.O_WRONLY, 0644)
	if err != nil {
		t.Fatalf("Auth file creation failed: %s", err)
	}
	defer f.Close()

	mockCredentials := base64.StdEncoding.EncodeToString([]byte("user:pass"))
	mockAuth := fmt.Sprintf(`{
		"auths": {
			"quay.io": {
				"auth": %q
			}
		}
	}`, mockCredentials)

	_, err = f.WriteString(mockAuth)
	if err != nil {
		t.Fatalf("Writing credentials failed: %s", err)
	}

	return authFile
}
