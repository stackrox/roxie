package stackroxversions

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"sort"

	"github.com/Masterminds/semver/v3"
	"github.com/stackrox/roxie/internal/constants"
)

const (
	gitHubAPIBase = "https://api.github.com/repos/" + constants.GitHubStackroxRepo
)

type ghRelease struct {
	TagName string `json:"tag_name"`
}

// LookupLatestReleaseTagsViaGitHub queries the GitHub releases API for stackrox/stackrox
// and attempts to return the newest atMost stable release tags (e.g. ["4.11.0", "4.10.4", "4.9.8"]),
// sorted by descending semver. Pre-release and RC tags are excluded.
// It is not guaranteed that the function returns exactly atMost tags, though this should be
// the case for reasonable values of atMost (0 < atMost < 10).
// The code is deliberately kept simple, i.e. it is not using pagination, which would
// smell like over-engineering for the use-case we need this for.
func LookupLatestReleaseTagsViaGitHub(ctx context.Context, atMost int) ([]string, error) {
	if atMost <= 0 {
		return nil, fmt.Errorf("atMost must be positive, got %d", atMost)
	}

	tags, err := fetchLatestGitHubReleases(ctx, atMost)
	if err != nil {
		return nil, err
	}

	var versions semver.Collection
	for _, tag := range tags {
		version, err := semver.NewVersion(tag)
		if err != nil || version.Prerelease() != "" {
			continue
		}
		versions = append(versions, version)
	}
	sort.Sort(sort.Reverse(versions))

	n := min(atMost, len(versions))
	if n == 0 {
		return nil, errors.New("failed to obtain any release tags parsing as semantic versions")
	}

	sortedReleaseTags := make([]string, n)
	for i := range n {
		sortedReleaseTags[i] = versions[i].Original()
	}

	return sortedReleaseTags, nil
}

func fetchLatestGitHubReleases(ctx context.Context, atMost int) ([]string, error) {
	releasesToFetch := min(100, atMost*10) // Reasonable estimate for how many tags we intend to fetch.
	url := fmt.Sprintf("%s?per_page=%d", gitHubAPIBase+"/releases", releasesToFetch)
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, url, nil)
	if err != nil {
		return nil, fmt.Errorf("creating request: %w", err)
	}
	req.Header.Set("Accept", "application/vnd.github+json")
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		return nil, fmt.Errorf("fetching releases: %w", err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("GitHub API returned status %d", resp.StatusCode)
	}

	var releasesResponse []ghRelease
	if err := json.NewDecoder(resp.Body).Decode(&releasesResponse); err != nil {
		return nil, fmt.Errorf("decoding releases response: %w", err)
	}

	// Convert to string slice.
	tags := make([]string, len(releasesResponse))
	for i, release := range releasesResponse {
		tags[i] = release.TagName
	}
	return tags, nil
}
