package dockerauth

import (
	"bytes"
	"encoding/base64"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"

	"github.com/stackrox/roxie/pkg/logger"
)

const (
	acsImageRegistry = "quay.io"
)

// DockerAuth handles Docker authentication and pull secret management.
type DockerAuth struct {
	logger *logger.Logger
}

// DockerConfig represents Docker configuration structure.
type DockerConfig struct {
	Auths       map[string]AuthEntry `json:"auths,omitempty"`
	CredHelpers map[string]string    `json:"credHelpers,omitempty"`
	CredsStore  string               `json:"credsStore,omitempty"`
}

// AuthEntry represents a single auth entry in Docker config.
type AuthEntry struct {
	Auth string `json:"auth,omitempty"`
}

// CredentialData represents credential data from credential helper.
type CredentialData struct {
	Username string `json:"Username"`
	Secret   string `json:"Secret"`
}

// New creates a new DockerAuth instance.
func New(log *logger.Logger) *DockerAuth {
	return &DockerAuth{
		logger: log,
	}
}

// GetDockerAuthString generates Docker authentication string for image pull secrets
func (d *DockerAuth) GetDockerAuthString(_, _ string) (string, error) {
	var username, password string

	// Try environment variables first.
	username = os.Getenv("REGISTRY_USERNAME")
	password = os.Getenv("REGISTRY_PASSWORD")

	if username != "" && password == "" {
		return "", errors.New("REGISTRY_USERNAME set but REGISTRY_PASSWORD is empty")
	}
	if username == "" && password != "" {
		return "", errors.New("REGISTRY_PASSWORD set but REGISTRY_USERNAME is empty")
	}

	if username == "" {
		// Try to get from Docker config file.
		dockerConfigPath := filepath.Join(os.Getenv("HOME"), ".docker", "config.json")
		d.logger.Dimf("REGISTRY_USERNAME/REGISTRY_PASSWORD unset. Trying to obtain Docker credentials from config file: %s", dockerConfigPath)
		if _, err := os.Stat(dockerConfigPath); err == nil {
			var err error
			username, password, err = d.getCredentialsFromDockerConfig(dockerConfigPath)
			if err != nil {
				return "", err
			}
		}
	}

	if username == "" || password == "" {
		return "", errors.New("no Docker credentials found")
	}

	// Create auth string.
	authString := fmt.Sprintf("%s:%s", username, password)
	encodedAuth := base64.StdEncoding.EncodeToString([]byte(authString))

	dockerConfig := DockerConfig{
		Auths: map[string]AuthEntry{
			acsImageRegistry: {Auth: encodedAuth},
		},
	}

	jsonData, err := json.Marshal(dockerConfig)
	if err != nil {
		return "", fmt.Errorf("failed to marshal Docker config: %w", err)
	}

	return string(jsonData), nil
}

// getCredentialsFromDockerConfig extracts credentials from existing Docker config.
func (d *DockerAuth) getCredentialsFromDockerConfig(configPath string) (string, string, error) {
	data, err := os.ReadFile(configPath)
	if err != nil {
		return "", "", fmt.Errorf("failed to read Docker config: %w", err)
	}

	var config DockerConfig
	if err := json.Unmarshal(data, &config); err != nil {
		return "", "", fmt.Errorf("failed to parse Docker config: %w", err)
	}

	// Check for existing auths for the ACS image registry.
	if authEntry, ok := config.Auths[acsImageRegistry]; ok && authEntry.Auth != "" {
		// Decode the base64 auth string to get username:password
		decoded, err := base64.StdEncoding.DecodeString(authEntry.Auth)
		if err != nil {
			return "", "", fmt.Errorf("failed to decode auth string: %w", err)
		}
		parts := bytes.SplitN(decoded, []byte(":"), 2)
		if len(parts) != 2 {
			return "", "", errors.New("invalid auth format")
		}
		return string(parts[0]), string(parts[1]), nil
	}

	// Try credential helper specifically configured for the ACS image registry
	helper := d.lookupCredentialHelperForRegistry(&config, acsImageRegistry)
	if helper == "" {
		return "", "", fmt.Errorf("no Docker credentials found in config for ACS image registry (%s)", acsImageRegistry)
	}

	credData, err := d.getCredentialFromHelper(helper, acsImageRegistry)
	if err != nil {
		return "", "", fmt.Errorf("failed to get credentials from helper '%s' for '%s': %w", helper, acsImageRegistry, err)
	}

	return credData.Username, credData.Secret, nil
}

// lookupCredentialHelperForRegistry returns the credential helper name for a given registry
// by checking registry-specific credHelpers first, then falling back to the global credsStore.
// Returns empty string if no helper is configured.
func (d *DockerAuth) lookupCredentialHelperForRegistry(config *DockerConfig, registry string) string {
	// First check for registry-specific credential helper
	if helper, ok := config.CredHelpers[registry]; ok {
		return helper
	}

	// Fall back to global credential store
	return config.CredsStore
}

// getCredentialFromHelper retrieves credentials from a credential helper.
func (d *DockerAuth) getCredentialFromHelper(helperName, registry string) (*CredentialData, error) {
	cmd := exec.Command(fmt.Sprintf("docker-credential-%s", helperName), "get")
	cmd.Stdin = bytes.NewBufferString(registry)

	output, err := cmd.Output()
	if err != nil {
		return nil, fmt.Errorf("credential helper '%s' for '%s' failed: %w", helperName, registry, err)
	}

	var credData CredentialData
	if err := json.Unmarshal(output, &credData); err != nil {
		return nil, fmt.Errorf("failed to parse credential helper output: %w", err)
	}

	if credData.Username == "" || credData.Secret == "" {
		return nil, errors.New("credential helper returned empty credentials")
	}

	return &credData, nil
}

// CreatePullSecretYAML creates Kubernetes pull secret YAML.
func (d *DockerAuth) CreatePullSecretYAML(namespace string) (string, error) {
	dockerConfigJSON, err := d.GetDockerAuthString("", "")
	if err != nil {
		return "", err
	}

	encodedConfig := base64.StdEncoding.EncodeToString([]byte(dockerConfigJSON))

	secretYAML := fmt.Sprintf(`apiVersion: v1
kind: Secret
metadata:
  name: stackrox
  namespace: %s
type: kubernetes.io/dockerconfigjson
data:
  .dockerconfigjson: %s
`, namespace, encodedConfig)

	return secretYAML, nil
}
