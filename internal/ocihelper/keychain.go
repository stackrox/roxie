package ocihelper

import (
	"os"

	"github.com/google/go-containerregistry/pkg/authn"
	"github.com/stackrox/roxie/internal/constants"
)

// Keychain resolves registry credentials by checking REGISTRY_USERNAME/REGISTRY_PASSWORD
// environment variables first, then falling back to the default Docker keychain
// (~/.docker/config.json, credential helpers, etc.).
var Keychain = authn.NewMultiKeychain(&envKeychain{}, authn.DefaultKeychain)

type envKeychain struct{}

func (e *envKeychain) Resolve(target authn.Resource) (authn.Authenticator, error) {
	username := os.Getenv(constants.EnvRegistryUsername)
	password := os.Getenv(constants.EnvRegistryPassword)
	if username == "" || password == "" {
		return authn.Anonymous, nil
	}
	return authn.FromConfig(authn.AuthConfig{
		Username: username,
		Password: password,
	}), nil
}
