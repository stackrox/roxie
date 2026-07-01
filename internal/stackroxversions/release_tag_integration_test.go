//go:build integration

package stackroxversions

import (
	"context"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestLookupLatestReleaseTags_Integration(t *testing.T) {
	ctx, cancel := context.WithTimeout(t.Context(), 30*time.Second)
	defer cancel()

	tags, err := LookupLatestReleaseTagsViaGitHub(ctx, 3)
	require.NoError(t, err)
	assert.NotEmpty(t, tags, "no tags")

	t.Log("Latest release tags")
	for i, tag := range tags {
		t.Logf("%v. %s", i+1, tag)
	}
}
