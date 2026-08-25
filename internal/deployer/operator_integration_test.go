//go:build integration

package deployer

import (
	"context"
	"testing"
	"time"

	"github.com/stackrox/roxie/internal/constants"
	"github.com/stackrox/roxie/internal/logger"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestResolveBundleImage_StackroxIOFallsBackToDefault_Integration(t *testing.T) {
	d := &Deployer{logger: logger.New()}
	ctx, cancel := context.WithTimeout(t.Context(), 2*time.Minute)
	defer cancel()

	instance := OperatorInstanceConfig{Version: "4.11.1", ImageRegistry: "quay.io/stackrox-io"}

	// We don't build operator bundles for upstream StackRox builds, so this should fall back to the rhacs-eng-hosted bundle.
	bundleImage, err := d.resolveBundleImage(ctx, instance)
	require.NoError(t, err)
	assert.Equal(t, constants.DefaultRegistry+"/stackrox-operator-bundle:v4.11.1", bundleImage,
		"should fall back to the rhacs-eng-hosted bundle")
}

func TestResolveBundleImage_NonNotFoundErrorPropagates_Integration(t *testing.T) {
	d := &Deployer{logger: logger.New()}
	ctx, cancel := context.WithTimeout(t.Context(), 30*time.Second)
	defer cancel()

	instance := OperatorInstanceConfig{Version: "4.11.1", ImageRegistry: "roxie-test-nonexistent-host.invalid/rhacs-eng"}

	_, err := d.resolveBundleImage(ctx, instance)
	require.Error(t, err)
}
