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
	logger               *logger.Logger
	skipCredVerification bool
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

// Credentials represents verified Docker credentials.
type Credentials struct {
	Username string
	Password string
}

// New creates a new DockerAuth instance.
func New(log *logger.Logger) *DockerAuth {
	return &DockerAuth{
		logger: log,
	}
}

// GetAndVerifyCredentials retrieves and verifies Docker credentials.
// This should be called early to fail fast if credentials are invalid.
func (d *DockerAuth) GetAndVerifyCredentials() (*Credentials, error) {
	var username, password string

	// Try environment variables first.
	username = os.Getenv("REGISTRY_USERNAME")
	password = os.Getenv("REGISTRY_PASSWORD")

	if username != "" && password == "" {
		return nil, errors.New("REGISTRY_USERNAME set but REGISTRY_PASSWORD is empty")
	}
	if username == "" && password != "" {
		return nil, errors.New("REGISTRY_PASSWORD set but REGISTRY_USERNAME is empty")
	}

	if username == "" {
		// Try to get from Docker config file.
		dockerConfigPath := filepath.Join(os.Getenv("HOME"), ".docker", "config.json")
		d.logger.Dimf("REGISTRY_USERNAME/REGISTRY_PASSWORD unset. Trying to obtain Docker credentials from config file: %s", dockerConfigPath)
		if _, err := os.Stat(dockerConfigPath); err == nil {
			var err error
			username, password, err = d.getCredentialsFromDockerConfig(dockerConfigPath)
			if err != nil {
				return nil, err
			}
		}
	}

	if username == "" || password == "" {
		return nil, errors.New("no Docker credentials found")
	}

	// Verify credentials.
	if !d.skipCredVerification {
		if err := d.VerifyCredentials(username, password); err != nil {
			return nil, fmt.Errorf("credentials are invalid: %w", err)
		}
	}

	return &Credentials{
		Username: username,
		Password: password,
	}, nil
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

// VerifyCredentials attempts to verify that the credentials work by making a request to the registry.
// This uses a read-only HTTP request.
// It mimics what the kubelet would do when pulling images.
func (d *DockerAuth) VerifyCredentials(username, password string) error {
	// Create auth header for Basic authentication
	authString := fmt.Sprintf("%s:%s", username, password)
	encodedAuth := base64.StdEncoding.EncodeToString([]byte(authString))

	// Try to get a token from quay.io's OAuth2 endpoint for a specific repository
	// This mimics what kubelet does when pulling images - it requests a token with pull scope
	// for the specific repository.
	repository := "rhacs-eng/main"
	authURL := fmt.Sprintf("https://%s/v2/auth?service=%s&scope=repository:%s:pull",
		acsImageRegistry, acsImageRegistry, repository)

	cmd := exec.Command("curl", "-s", "-f",
		"-H", fmt.Sprintf("Authorization: Basic %s", encodedAuth),
		authURL)

	output, err := cmd.CombinedOutput()
	if err != nil {
		d.logger.Warningf("Failed to verify credentials for %s: %v", acsImageRegistry, err)
		d.logger.Dimf("Verification output: %s", string(output))
		return fmt.Errorf("credential verification failed for %s: %w", acsImageRegistry, err)
	}

	// Check if we got a valid JSON response with a token
	var tokenResponse map[string]interface{}
	if err := json.Unmarshal(output, &tokenResponse); err != nil {
		return fmt.Errorf("credential verification failed: invalid response from %s: %w", acsImageRegistry, err)
	}

	if _, ok := tokenResponse["token"]; !ok {
		return fmt.Errorf("credential verification failed: no token received from %s", acsImageRegistry)
	}

	d.logger.Dimf("Successfully verified credentials for %s (repository: %s)", acsImageRegistry, repository)
	return nil
}

// CreatePullSecretYAMLFromCredentials creates Kubernetes pull secret YAML from verified credentials.
func (d *DockerAuth) CreatePullSecretYAMLFromCredentials(creds *Credentials, namespace string) string {
	// Create auth string
	authString := fmt.Sprintf("%s:%s", creds.Username, creds.Password)
	encodedAuth := base64.StdEncoding.EncodeToString([]byte(authString))

	dockerConfig := DockerConfig{
		Auths: map[string]AuthEntry{
			acsImageRegistry: {Auth: encodedAuth},
		},
	}

	jsonData, _ := json.Marshal(dockerConfig)
	encodedConfig := base64.StdEncoding.EncodeToString(jsonData)

	secretYAML := fmt.Sprintf(`apiVersion: v1
kind: Secret
metadata:
  name: stackrox
  namespace: %s
type: kubernetes.io/dockerconfigjson
data:
  .dockerconfigjson: %s
`, namespace, encodedConfig)

	return secretYAML
}
