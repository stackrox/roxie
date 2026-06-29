//go:build integration

package helpers

import (
	"context"
	"fmt"
	"testing"
	"time"

	"github.com/google/go-containerregistry/pkg/authn"
	"github.com/google/go-containerregistry/pkg/name"
	"github.com/google/go-containerregistry/pkg/v1/remote"
	"github.com/stackrox/roxie/internal/constants"
	"github.com/stackrox/roxie/internal/logger"
	"github.com/stretchr/testify/require"
)

func TestLookupLatestTag_Integration(t *testing.T) {
	log := logger.New()
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()

	tag, err := LookupLatestTag(ctx, log)
	require.NoError(t, err)
	require.NotEmpty(t, tag)

	imageRef := fmt.Sprintf("%s/main:%s", constants.DefaultRegistry, tag)
	ref, err := name.ParseReference(imageRef)
	require.NoError(t, err)

	_, err = remote.Head(ref, remote.WithContext(ctx), remote.WithAuthFromKeychain(authn.DefaultKeychain))
	require.NoError(t, err, "image %s is not pullable", imageRef)

	t.Logf("Latest pullable tag: %s (%s)", tag, imageRef)
}
