package dockerauth

import (
	"encoding/base64"
	"encoding/json"
	"strings"
	"testing"

	"github.com/stackrox/roxie/internal/logger"
)

func TestGetAndVerifyCredentialsFromEnv(t *testing.T) {
	// Set environment variables for test
	t.Setenv("REGISTRY_USERNAME", "user")
	t.Setenv("REGISTRY_PASSWORD", "pass")

	log := logger.New()
	da := New(log)
	da.skipCredVerification = true // Skip verification in tests

	creds, err := da.GetAndVerifyCredentials()
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
	yamlText := da.CreatePullSecretYAMLFromCredentials(*creds, "ns")

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

	if _, ok := data["auths"]; !ok {
		t.Error("Decoded JSON should contain 'auths' key")
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

	_, err := da.GetAndVerifyCredentials()
	if err == nil {
		t.Error("Expected error when no credentials are available")
	}
}
