package dockerauth

import (
	"encoding/base64"
	"encoding/json"
	"os"
	"strings"
	"testing"

	"github.com/stackrox/roxie-golang/pkg/logger"
)

func TestCreatePullSecretYAMLFromEnv(t *testing.T) {
	// Set environment variables for test
	os.Setenv("REGISTRY_USERNAME", "user")
	os.Setenv("REGISTRY_PASSWORD", "pass")
	defer func() {
		os.Unsetenv("REGISTRY_USERNAME")
		os.Unsetenv("REGISTRY_PASSWORD")
	}()

	log := logger.New()
	da := New(log, true)

	yamlText, err := da.CreatePullSecretYAML("ns")
	if err != nil {
		t.Fatalf("CreatePullSecretYAML failed: %v", err)
	}

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

func TestCreatePullSecretYAMLNoCredentials(t *testing.T) {
	// Ensure no credentials are set
	os.Unsetenv("REGISTRY_USERNAME")
	os.Unsetenv("REGISTRY_PASSWORD")

	// Remove docker config if it exists (temporarily)
	homeDir, _ := os.UserHomeDir()
	dockerConfigPath := homeDir + "/.docker/config.json"
	var backupConfig []byte
	var configExisted bool

	if data, err := os.ReadFile(dockerConfigPath); err == nil {
		backupConfig = data
		configExisted = true
		os.Remove(dockerConfigPath)
	}
	defer func() {
		if configExisted {
			os.WriteFile(dockerConfigPath, backupConfig, 0644)
		}
	}()

	log := logger.New()
	da := New(log, true)

	_, err := da.CreatePullSecretYAML("ns")
	if err == nil {
		t.Error("Expected error when no credentials are available")
	}
}
