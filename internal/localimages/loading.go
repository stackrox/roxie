package localimages

import (
	"context"
	"fmt"
	"os/exec"
	"sync"

	"github.com/stackrox/roxie/internal/logger"
)

// loadImageToKind loads a single image into a kind cluster.
func loadImageToKind(ctx context.Context, imageRef, clusterName string, log *logger.Logger) error {
	log.Dimf("Loading %s into kind cluster %s", imageRef, clusterName)

	cmd := exec.CommandContext(ctx, "kind", "load", "docker-image", imageRef, "-n", clusterName)
	output, err := cmd.CombinedOutput()
	if err != nil {
		return fmt.Errorf("kind load failed for %s: %w\nOutput: %s", imageRef, err, string(output))
	}

	return nil
}

// LoadImagesToKind loads multiple images into a kind cluster in parallel.
// Uses up to 4 concurrent workers to speed up loading.
// Returns on first error encountered (fail-fast behavior).
//
// Images are loaded using quay.io paths (e.g., quay.io/rhacs-eng/main:tag) instead of
// localhost paths to ensure the tags in kind match what the operator CSV will reference.
func LoadImagesToKind(ctx context.Context, images map[string]string, mainImageTag, operatorTag string, clusterName string, log *logger.Logger) error {
	if len(images) == 0 {
		return nil
	}

	log.Infof("Loading %d images into kind cluster %s", len(images), clusterName)

	// Build list of image references to load
	// Use the actual references found during detection (which may be from fallback branding)
	imageRefs := make([]string, 0, len(images))

	for _, img := range getExpectedImages(mainImageTag, operatorTag) {
		imageKey := img.name + ":" + img.tag
		if imageRef, exists := images[imageKey]; exists {
			imageRefs = append(imageRefs, imageRef)
		}
	}

	// Channel for images to process
	imageChan := make(chan string, len(imageRefs))
	for _, imageRef := range imageRefs {
		imageChan <- imageRef
	}
	close(imageChan)

	// Error channel
	errChan := make(chan error, len(imageRefs))

	// Use 4 workers for parallel loading (matching existing image verification parallelism)
	const numWorkers = 4
	var wg sync.WaitGroup

	for i := 0; i < numWorkers && i < len(imageRefs); i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			for imageRef := range imageChan {
				if err := loadImageToKind(ctx, imageRef, clusterName, log); err != nil {
					errChan <- err
					return
				}
			}
		}()
	}

	wg.Wait()
	close(errChan)

	// Check for errors - collect all errors that occurred
	var firstErr error
	for err := range errChan {
		if firstErr == nil {
			firstErr = err
		}
	}

	if firstErr != nil {
		return firstErr
	}

	log.Infof("Successfully loaded %d images into kind cluster", len(imageRefs))
	return nil
}
