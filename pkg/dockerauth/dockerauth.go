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

	"github.com/stackrox/roxie-golang/pkg/logger"
)

// DockerAuth handles Docker authentication and pull secret management
type DockerAuth struct {
	logger       *logger.Logger
	cacheEnabled bool
	authCache    map[string]string
}

// DockerConfig represents Docker configuration structure
type DockerConfig struct {
	Auths       map[string]AuthEntry `json:"auths,omitempty"`
	CredHelpers map[string]string    `json:"credHelpers,omitempty"`
}

// AuthEntry represents a single auth entry in Docker config
type AuthEntry struct {
	Auth string `json:"auth,omitempty"`
}

// CredentialData represents credential data from credential helper
type CredentialData struct {
	Username string `json:"Username"`
	Secret   string `json:"Secret"`
}

// New creates a new DockerAuth instance
func New(log *logger.Logger, cacheEnabled bool) *DockerAuth {
	return &DockerAuth{
		logger:       log,
		cacheEnabled: cacheEnabled,
		authCache:    make(map[string]string),
	}
}

// GetDockerAuthString generates Docker authentication string for image pull secrets
func (d *DockerAuth) GetDockerAuthString(_, _ string) (string, error) {
	// Try environment variables first
	username := os.Getenv("REGISTRY_USERNAME")
	password := os.Getenv("REGISTRY_PASSWORD")

	if username != "" && password != "" {
		// Use credentials from environment
	} else {
		// Try to get from Docker config file
		dockerConfigPath := filepath.Join(os.Getenv("HOME"), ".docker", "config.json")
		if _, err := os.Stat(dockerConfigPath); err == nil {
			return d.getDockerConfigAuth(dockerConfigPath)
		}

		return "", errors.New("no Docker credentials found")
	}

	// Create auth string
	authString := fmt.Sprintf("%s:%s", username, password)
	encodedAuth := base64.StdEncoding.EncodeToString([]byte(authString))

	dockerConfig := DockerConfig{
		Auths: map[string]AuthEntry{
			"quay.io": {Auth: encodedAuth},
		},
	}

	jsonData, err := json.Marshal(dockerConfig)
	if err != nil {
		return "", fmt.Errorf("failed to marshal Docker config: %w", err)
	}

	return string(jsonData), nil
}

// getDockerConfigAuth extracts auth from existing Docker config
func (d *DockerAuth) getDockerConfigAuth(configPath string) (string, error) {
	data, err := os.ReadFile(configPath)
	if err != nil {
		return "", fmt.Errorf("failed to read Docker config: %w", err)
	}

	var config DockerConfig
	if err := json.Unmarshal(data, &config); err != nil {
		return "", fmt.Errorf("failed to parse Docker config: %w", err)
	}

	// Check for existing auths
	if len(config.Auths) > 0 {
		result := DockerConfig{Auths: config.Auths}
		jsonData, err := json.Marshal(result)
		if err != nil {
			return "", fmt.Errorf("failed to marshal auths: %w", err)
		}
		return string(jsonData), nil
	}

	// Check for credential helpers
	if len(config.CredHelpers) > 0 {
		for registry, helper := range config.CredHelpers {
			cmd := exec.Command(fmt.Sprintf("docker-credential-%s", helper), "get")
			cmd.Stdin = bytes.NewBufferString(registry)

			output, err := cmd.Output()
			if err != nil {
				d.logger.Warningf("Credential helper '%s' for '%s' failed: %v", helper, registry, err)
				continue
			}

			var credData CredentialData
			if err := json.Unmarshal(output, &credData); err != nil {
				continue
			}

			if credData.Username != "" && credData.Secret != "" {
				authString := fmt.Sprintf("%s:%s", credData.Username, credData.Secret)
				encodedAuth := base64.StdEncoding.EncodeToString([]byte(authString))

				result := DockerConfig{
					Auths: map[string]AuthEntry{
						registry: {Auth: encodedAuth},
					},
				}

				jsonData, err := json.Marshal(result)
				if err != nil {
					return "", fmt.Errorf("failed to marshal credential helper result: %w", err)
				}
				return string(jsonData), nil
			}
		}
	}

	return "", errors.New("no Docker credentials found in config")
}

// CreatePullSecretYAML creates Kubernetes pull secret YAML
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
