//go:build integration

package helpers

import (
	"context"
	"errors"
	"fmt"
	"net/http"
	"testing"
	"time"

	"github.com/google/go-containerregistry/pkg/name"
	"github.com/google/go-containerregistry/pkg/v1/remote"
	"github.com/google/go-containerregistry/pkg/v1/remote/transport"
	"github.com/stackrox/roxie/internal/constants"
	"github.com/stackrox/roxie/internal/logger"
	"github.com/stackrox/roxie/internal/ocihelper"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestLookupLatestTag_Integration(t *testing.T) {
	log := logger.New()
	ctx, cancel := context.WithTimeout(t.Context(), 2*time.Minute)
	defer cancel()

	tag, err := LookupLatestTag(ctx, log)
	require.NoError(t, err)
	require.NotEmpty(t, tag)

	imageRef := fmt.Sprintf("%s/main:%s", constants.DefaultRegistry, tag)
	ref, err := name.ParseReference(imageRef)
	require.NoError(t, err)

	_, err = remote.Head(ref, remote.WithContext(ctx), remote.WithAuthFromKeychain(ocihelper.Keychain))
	require.NoError(t, err, "image %s is not pullable", imageRef)

	t.Logf("Latest pullable tag: %s (%s)", tag, imageRef)
}

func TestVerifyImageExistence_NotFound_Integration(t *testing.T) {
	log := logger.New()
	ctx, cancel := context.WithTimeout(t.Context(), 30*time.Second)
	defer cancel()

	madeUpImage := fmt.Sprintf("%s/main:99.99.99", constants.DefaultRegistry)
	err := ocihelper.VerifyImageExistence(ctx, log, madeUpImage)
	require.Error(t, err)

	var te *transport.Error
	require.True(t, errors.As(err, &te), "expected transport.Error, got %T", err)
	assert.Equal(t, http.StatusNotFound, te.StatusCode)
}
