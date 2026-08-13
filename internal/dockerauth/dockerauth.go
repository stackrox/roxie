package dockerauth

import (
	"bytes"
	"context"
	"encoding/base64"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"os"
	"os/exec"
	"path/filepath"
	"strings"

	"github.com/google/go-containerregistry/pkg/authn"
	"github.com/google/go-containerregistry/pkg/name"
	"github.com/google/go-containerregistry/pkg/v1/remote/transport"

	"github.com/stackrox/roxie/internal/constants"
	"github.com/stackrox/roxie/internal/logger"
)

// splitRegistryHost splits a resolved image registry (e.g. "quay.io/stackrox-io")
// into its host ("quay.io") and org/repo path ("stackrox-io").
func splitRegistryHost(registry string) (host, path string) {
	host, path, _ = strings.Cut(registry, "/")
	return host, path
}

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
func (d *DockerAuth) GetAndVerifyCredentials(ctx context.Context, registry string) (*Credentials, error) {
	host, orgPath := splitRegistryHost(registry)
	mainImageRepository := orgPath + "/main"

	var username, password string

	// Try environment variables first.
	username = os.Getenv(constants.EnvRegistryUsername)
	password = os.Getenv(constants.EnvRegistryPassword)

	if username != "" && password == "" {
		return nil, fmt.Errorf("%s set but %s is empty", constants.EnvRegistryUsername, constants.EnvRegistryPassword)
	}
	if username == "" && password != "" {
		return nil, fmt.Errorf("%s set but %s is empty", constants.EnvRegistryPassword, constants.EnvRegistryUsername)
	}

	if username == "" {
		// Try to get from Docker config file.
		dockerConfigPath := filepath.Join(os.Getenv("HOME"), ".docker", "config.json")
		d.logger.Dimf("REGISTRY_USERNAME/REGISTRY_PASSWORD unset. Trying to obtain Docker credentials from config file: %s", dockerConfigPath)
		if _, err := os.Stat(dockerConfigPath); err == nil {
			var err error
			username, password, err = d.getCredentialsFromDockerConfig(dockerConfigPath, host)
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
		if err := d.VerifyCredentials(ctx, username, password, host, mainImageRepository); err != nil {
			return nil, fmt.Errorf("credentials are invalid: %w", err)
		}
	}

	return &Credentials{
		Username: username,
		Password: password,
	}, nil
}

// getCredentialsFromDockerConfig extracts credentials from existing Docker config
// for the given registry host.
func (d *DockerAuth) getCredentialsFromDockerConfig(configPath, host string) (string, string, error) {
	data, err := os.ReadFile(configPath)
	if err != nil {
		return "", "", fmt.Errorf("failed to read Docker config: %w", err)
	}

	var config DockerConfig
	if err := json.Unmarshal(data, &config); err != nil {
		return "", "", fmt.Errorf("failed to parse Docker config: %w", err)
	}

	// Check for existing auths for the target registry host.
	if authEntry, ok := config.Auths[host]; ok && authEntry.Auth != "" {
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

	// Try credential helper specifically configured for the target registry host.
	helper := d.lookupCredentialHelperForRegistry(&config, host)
	if helper == "" {
		return "", "", fmt.Errorf("no Docker credentials found in config for image registry (%s)", host)
	}

	credData, err := d.getCredentialFromHelper(helper, host)
	if err != nil {
		return "", "", fmt.Errorf("failed to get credentials from helper '%s' for '%s': %w", helper, host, err)
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

// VerifyCredentials verifies that the given credentials grant pull access to
// the given repository on the given registry host. It works for registries
// that follow the standard OCI Distribution v2 challenge/token protocol.
func (d *DockerAuth) VerifyCredentials(ctx context.Context, username, password, host, repository string) error {
	reg, err := name.NewRegistry(host)
	if err != nil {
		return fmt.Errorf("invalid registry host %q: %w", host, err)
	}

	auth := &authn.Basic{Username: username, Password: password}
	scope := fmt.Sprintf("repository:%s:pull", repository)

	if _, err := transport.NewWithContext(ctx, reg, auth, http.DefaultTransport, []string{scope}); err != nil {
		return fmt.Errorf("credential verification failed for %s: %w", host, err)
	}

	d.logger.Dimf("Successfully verified credentials for %s (repository: %s)", host, repository)
	return nil
}

// CreatePullSecretYAMLFromCredentials creates Kubernetes pull secret YAML from
// verified credentials, scoped to the host of the given image registry
func (d *DockerAuth) CreatePullSecretYAMLFromCredentials(creds Credentials, namespace, registry string) string {
	host, _ := splitRegistryHost(registry)

	// Create auth string
	authString := fmt.Sprintf("%s:%s", creds.Username, creds.Password)
	encodedAuth := base64.StdEncoding.EncodeToString([]byte(authString))

	dockerConfig := DockerConfig{
		Auths: map[string]AuthEntry{
			host: {Auth: encodedAuth},
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
