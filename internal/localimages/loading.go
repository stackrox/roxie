package localimages

import (
	"context"
	"fmt"
	"os/exec"
	"sync"

	"github.com/stackrox/roxie/internal/logger"
)

// LoadImageToKind loads a single image into a kind cluster.
func LoadImageToKind(ctx context.Context, imageRef, clusterName string, log *logger.Logger) error {
	log.Dimf("Loading %s into kind cluster %s", imageRef, clusterName)

	cmd := exec.CommandContext(ctx, "kind", "load", "docker-image", imageRef, "-n", clusterName)
	output, err := cmd.CombinedOutput()
	if err != nil {
		return fmt.Errorf("kind load failed for %s: %w\nOutput: %s", imageRef, err, string(output))
	}

	return nil
}

// buildKindLoadCommand builds the kind load command arguments.
func buildKindLoadCommand(imageRef, clusterName string) []string {
	return []string{"kind", "load", "docker-image", imageRef, "-n", clusterName}
}

// LoadImagesToKind loads multiple images into a kind cluster in parallel.
// Uses up to 4 concurrent workers to speed up loading.
func LoadImagesToKind(ctx context.Context, images map[string]string, clusterName string, log *logger.Logger) error {
	if len(images) == 0 {
		return nil
	}

	log.Infof("Loading %d images into kind cluster %s", len(images), clusterName)

	// Channel for images to process
	imageChan := make(chan string, len(images))
	for _, imageRef := range images {
		imageChan <- imageRef
	}
	close(imageChan)

	// Error channel
	errChan := make(chan error, len(images))

	// Use 4 workers for parallel loading (matching existing image verification parallelism)
	const numWorkers = 4
	var wg sync.WaitGroup

	for i := 0; i < numWorkers && i < len(images); i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			for imageRef := range imageChan {
				if err := LoadImageToKind(ctx, imageRef, clusterName, log); err != nil {
					errChan <- err
					return
				}
			}
		}()
	}

	wg.Wait()
	close(errChan)

	// Check for errors
	if err := <-errChan; err != nil {
		return err
	}

	log.Infof("Successfully loaded %d images into kind cluster", len(images))
	return nil
}
