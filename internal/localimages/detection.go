package localimages

import (
	"fmt"
	"os/exec"
)

const (
	quayRegistry = "quay.io"
)

// mainImageNames lists the core product images that use mainTag
var mainImageNames = []string{
	"main",
	"scanner",
	"scanner-db",
	"scanner-v4",
	"scanner-v4-db",
	"central-db",
	"collector",
}

// imageSpec represents an image name and its tag
type imageSpec struct {
	name string
	tag  string
}

// getExpectedImages returns all expected images with their respective tags.
// Main images use mainTag, while operator images use operatorTag.
func getExpectedImages(mainTag, operatorTag string) []imageSpec {
	specs := make([]imageSpec, 0, len(mainImageNames)+2)

	// Main images use mainTag
	for _, name := range mainImageNames {
		specs = append(specs, imageSpec{name: name, tag: mainTag})
	}

	// Operator images use operatorTag (with and without v prefix)
	specs = append(specs, imageSpec{name: "stackrox-operator", tag: operatorTag})
	specs = append(specs, imageSpec{name: "stackrox-operator-bundle", tag: "v" + operatorTag})

	return specs
}

// buildImageReferences returns candidate image references to check in podman.
// Checks both branding organizations to handle cases where images don't support
// ROX_PRODUCT_BRANDING (e.g., collector currently only builds with stackrox-io).
func buildImageReferences(imageName, tag string) []string {
	currentOrg := GetBrandingOrganization()

	// Determine the fallback organization
	var fallbackOrg string
	if currentOrg == "rhacs-eng" {
		fallbackOrg = "stackrox-io"
	} else {
		fallbackOrg = "rhacs-eng"
	}

	return []string{
		fmt.Sprintf("%s/%s/%s:%s", quayRegistry, currentOrg, imageName, tag),
		fmt.Sprintf("%s/%s/%s:%s", quayRegistry, fallbackOrg, imageName, tag),
	}
}

// checkLocalImage checks if an image exists in podman.
// Returns the full image reference and true if found, empty string and false if not found.
func checkLocalImage(imageName, tag string) (string, bool, error) {
	refs := buildImageReferences(imageName, tag)

	for _, ref := range refs {
		exists, err := podmanImageExists(ref)
		if err != nil {
			return "", false, fmt.Errorf("checking podman for %s: %w", ref, err)
		}
		if exists {
			return ref, true, nil
		}
	}

	return "", false, nil
}

// checks if an image exists in podman
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

// CheckImages checks which images from the overall set exist locally.
// Returns a map of image names to their full references.
func CheckImages(mainTag, operatorTag string) (map[string]string, error) {
	localImages := make(map[string]string)

	for _, img := range getExpectedImages(mainTag, operatorTag) {
		ref, found, err := checkLocalImage(img.name, img.tag)
		if err != nil {
			return nil, fmt.Errorf("checking %s:%s: %w", img.name, img.tag, err)
		}
		if found {
			localImages[img.name+":"+img.tag] = ref
		}
	}

	return localImages, nil
}
