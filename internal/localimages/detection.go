package localimages

import (
	"fmt"
	"os/exec"
)

const (
	localhostPrefix = "localhost/stackrox"
	quayRegistry    = "quay.io"
)

// buildImageReferences returns candidate image references to check in podman.
// Returns in priority order: localhost/stackrox first, then quay.io registry.
func buildImageReferences(imageName, tag string) []string {
	org := GetBrandingOrganization()
	return []string{
		fmt.Sprintf("%s/%s:%s", localhostPrefix, imageName, tag),
		fmt.Sprintf("%s/%s/%s:%s", quayRegistry, org, imageName, tag),
	}
}

// CheckLocalImage checks if an image exists in podman.
// Returns the full image reference if found, empty string if not found.
func CheckLocalImage(imageName, tag string) (string, error) {
	refs := buildImageReferences(imageName, tag)

	for _, ref := range refs {
		exists, err := podmanImageExists(ref)
		if err != nil {
			return "", fmt.Errorf("checking podman for %s: %w", ref, err)
		}
		if exists {
			return ref, nil
		}
	}

	return "", nil
}

// podmanImageExists checks if an image exists in podman using 'podman image exists'.
// Returns true if the image exists (exit code 0), false otherwise.
func podmanImageExists(imageRef string) (bool, error) {
	cmd := exec.Command("podman", "image", "exists", imageRef)
	err := cmd.Run()
	if err != nil {
		// Exit code 1 means image doesn't exist (expected)
		// Other errors are actual failures
		if exitErr, ok := err.(*exec.ExitError); ok && exitErr.ExitCode() == 1 {
			return false, nil
		}
		return false, err
	}
	return true, nil
}

// CheckImages checks which images from the set exist locally.
// Returns a map of image names to their full references.
func CheckImages(mainTag, operatorTag string) (map[string]string, error) {
	images := []string{
		"main",
		"scanner",
		"scanner-db",
		"scanner-v4-db",
		"scanner-v4-indexer",
		"scanner-v4-matcher",
		"central-db",
	}

	localImages := make(map[string]string)

	// Check main images
	for _, imageName := range images {
		ref, err := CheckLocalImage(imageName, mainTag)
		if err != nil {
			return nil, fmt.Errorf("checking %s:%s: %w", imageName, mainTag, err)
		}
		if ref != "" {
			localImages[imageName+":"+mainTag] = ref
		}
	}

	// Check stackrox-operator with main tag (no v prefix)
	ref, err := CheckLocalImage("stackrox-operator", mainTag)
	if err != nil {
		return nil, fmt.Errorf("checking stackrox-operator:%s: %w", mainTag, err)
	}
	if ref != "" {
		localImages["stackrox-operator:"+mainTag] = ref
	}

	// Check operator bundle image with v prefix
	// Note: We don't check operator-index as roxie doesn't use it in default (non-OLM) mode
	ref, err = CheckLocalImage("stackrox-operator-bundle", "v"+operatorTag)
	if err != nil {
		return nil, fmt.Errorf("checking stackrox-operator-bundle:v%s: %w", operatorTag, err)
	}
	if ref != "" {
		localImages["stackrox-operator-bundle:v"+operatorTag] = ref
	}

	return localImages, nil
}
