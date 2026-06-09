//go:build integration

package manifest

import (
	"context"
	"testing"
	"time"

	"github.com/stackrox/roxie/internal/deployer"
	"github.com/stackrox/roxie/internal/k8s"
	"github.com/stackrox/roxie/internal/logger"
	"github.com/stackrox/roxie/internal/types"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func cleanupRoxieNamespace(t *testing.T) {
	t.Helper()
	log := logger.New()
	ctx, cancel := context.WithTimeout(context.Background(), 60*time.Second)
	defer cancel()
	err := DeleteRoxieNamespace(ctx, log)
	assert.NoError(t, err, "deleting roxie namespace failed")
}

func TestCreateAndLoadManifest_Integration(t *testing.T) {
	t.Cleanup(func() { cleanupRoxieNamespace(t) })
	cleanupRoxieNamespace(t)

	log := logger.New()
	ctx, cancel := context.WithTimeout(context.Background(), 60*time.Second)
	defer cancel()

	original := RoxieManifest{
		RoxieEnvironment: types.RoxieEnvironment{
			APIEndpoint:      "localhost:8443",
			RoxAdminPassword: "testpassword",
			RoxBaseURL:       "https://localhost:8443",
			RoxEndpoint:      "localhost:8443",
			RoxUsername:      "admin",
		},
		Config: deployer.Config{
			Central: deployer.CentralConfig{
				Namespace: "acs-central",
			},
		},
	}

	err := CreateManifestSecretOnCluster(ctx, log, original)
	require.NoError(t, err)

	loaded, err := LoadManifestSecret(ctx, log)
	require.NoError(t, err)

	assert.Empty(t, loaded.RoxieEnvironment.RoxCaCertFile)
	assert.Equal(t, original, *loaded)
}

func TestDeleteManifest_Integration(t *testing.T) {
	t.Cleanup(func() { cleanupRoxieNamespace(t) })
	cleanupRoxieNamespace(t)

	log := logger.New()
	ctx, cancel := context.WithTimeout(context.Background(), 60*time.Second)
	defer cancel()

	m := RoxieManifest{
		RoxieEnvironment: types.RoxieEnvironment{
			RoxUsername: "admin",
		},
	}

	err := CreateManifestSecretOnCluster(ctx, log, m)
	require.NoError(t, err)

	err = DeleteManifestSecret(ctx, log)
	assert.NoError(t, err)
}

func TestDeleteRoxieNamespace_Integration(t *testing.T) {
	t.Cleanup(func() { cleanupRoxieNamespace(t) })
	cleanupRoxieNamespace(t)

	log := logger.New()
	ctx, cancel := context.WithTimeout(context.Background(), 60*time.Second)
	defer cancel()

	err := ensureRoxieNamespace(ctx, log)
	require.NoError(t, err)

	err = DeleteRoxieNamespace(ctx, log)
	require.NoError(t, err)

	_, err = k8s.RunKubectl(ctx, log, k8s.KubectlOptions{
		Args: []string{"get", "namespace", roxieNamespace},
	})
	assert.Error(t, err, "namespace should no longer exist")
}

func TestLoadManifest_NotFound_Integration(t *testing.T) {
	t.Cleanup(func() { cleanupRoxieNamespace(t) })
	cleanupRoxieNamespace(t)

	log := logger.New()
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()

	_, err := LoadManifestSecret(ctx, log)
	assert.Error(t, err)
}
