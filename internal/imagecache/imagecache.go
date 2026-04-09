package imagecache

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"sync"

	"github.com/stackrox/roxie/internal/logger"
	"github.com/stackrox/roxie/internal/skopeohelper"
)

// ImageCache manages cache of verified pullable Docker images
type ImageCache struct {
	cacheFile  string
	maxEntries int
	cache      []string
	logger     *logger.Logger
	mu         sync.Mutex
}

// CacheData represents the structure of the cache file
type CacheData struct {
	Images []string `json:"images"`
}

// New creates a new ImageCache instance
func New(log *logger.Logger, cacheFile string, maxEntries int) *ImageCache {
	if cacheFile == "" {
		home, err := os.UserHomeDir()
		if err != nil {
			home = "."
		}
		cacheFile = filepath.Join(home, ".roxie.image_cache")
	}

	if maxEntries <= 0 {
		maxEntries = 20
	}

	ic := &ImageCache{
		cacheFile:  cacheFile,
		maxEntries: maxEntries,
		logger:     log,
	}

	ic.cache = ic.loadCache()
	return ic
}

// loadCache loads image cache from file
func (ic *ImageCache) loadCache() []string {
	data, err := os.ReadFile(ic.cacheFile)
	if err != nil {
		return []string{}
	}

	// Try new format first (with "images" key)
	var cacheData CacheData
	if err := json.Unmarshal(data, &cacheData); err == nil && cacheData.Images != nil {
		return cacheData.Images
	}

	// Try old format (plain array)
	var images []string
	if err := json.Unmarshal(data, &images); err == nil {
		return images
	}

	return []string{}
}

// saveCache saves image cache to file
// Must be called with ic.mu held
func (ic *ImageCache) saveCache() {
	// Ensure directory exists
	dir := filepath.Dir(ic.cacheFile)
	if err := os.MkdirAll(dir, 0755); err != nil {
		return // Silently fail on cache save errors
	}

	cacheData := CacheData{Images: ic.cache}
	data, err := json.MarshalIndent(cacheData, "", "  ")
	if err != nil {
		return
	}

	_ = os.WriteFile(ic.cacheFile, data, 0644)
}

// IsCached checks if image is in cache
func (ic *ImageCache) IsCached(imageRef string) bool {
	ic.mu.Lock()
	defer ic.mu.Unlock()

	for _, img := range ic.cache {
		if img == imageRef {
			return true
		}
	}
	return false
}

// AddToCache adds image to cache, maintaining max_entries limit
func (ic *ImageCache) AddToCache(imageRef string) {
	ic.mu.Lock()
	defer ic.mu.Unlock()

	// Remove if already exists (to move to end)
	for i, img := range ic.cache {
		if img == imageRef {
			ic.cache = append(ic.cache[:i], ic.cache[i+1:]...)
			break
		}
	}

	// Add to end
	ic.cache = append(ic.cache, imageRef)

	// Maintain max entries
	if len(ic.cache) > ic.maxEntries {
		ic.cache = ic.cache[len(ic.cache)-ic.maxEntries:]
	}

	ic.saveCache()
}

// VerifyImagePullable verifies if image is pullable using cache when possible
func (ic *ImageCache) VerifyImagePullable(ctx context.Context, imageRef string) bool {
	if ic.IsCached(imageRef) {
		return true
	}

	// Use OCI registry client to verify image accessibility.
	err := skopeohelper.InspectImage(ctx, ic.logger, imageRef)
	if err == nil {
		ic.AddToCache(imageRef)
		return true
	}

	fmt.Fprintf(os.Stderr, "Failed to verify image %s: %v\n", imageRef, err)
	return false
}

// VerifyImagesPullable verifies that Docker images are pullable using cached results
func (ic *ImageCache) VerifyImagesPullable(ctx context.Context, images ...string) bool {
	if len(images) == 0 {
		return true
	}

	// Skip verification if environment variable is set
	if skip := os.Getenv("SKIP_IMAGE_VERIFICATION"); skip == "true" || skip == "1" || skip == "yes" {
		ic.logger.Infof("Skipping image verification for %d images (SKIP_IMAGE_VERIFICATION=true)", len(images))
		return true
	}

	// Separate cached and uncached images
	var cachedImages, uncachedImages []string
	for _, img := range images {
		if ic.IsCached(img) {
			cachedImages = append(cachedImages, img)
		} else {
			uncachedImages = append(uncachedImages, img)
		}
	}

	// Report cached results immediately
	if len(cachedImages) > 0 {
		ic.logger.Successf("✓ %d images verified from cache", len(cachedImages))
		for _, img := range cachedImages {
			ic.logger.Dim(fmt.Sprintf("✓ Image %s (cached)", img))
		}
	}

	// Verify uncached images if any
	failedImages := []string{}
	if len(uncachedImages) > 0 {
		// In Go, we can use goroutines for parallel verification
		type result struct {
			img     string
			success bool
			errMsg  string
		}

		results := make(chan result, len(uncachedImages))
		maxWorkers := 4
		if len(uncachedImages) < maxWorkers {
			maxWorkers = len(uncachedImages)
		}

		// Worker pool
		sem := make(chan struct{}, maxWorkers)
		var wg sync.WaitGroup

		for _, img := range uncachedImages {
			wg.Add(1)
			go func(image string) {
				defer wg.Done()
				sem <- struct{}{}        // Acquire semaphore
				defer func() { <-sem }() // Release semaphore

				success := ic.VerifyImagePullable(ctx, image)
				if success {
					results <- result{img: image, success: true}
				} else {
					results <- result{img: image, success: false, errMsg: "not pullable"}
				}
			}(img)
		}

		// Close results channel when all workers are done
		go func() {
			wg.Wait()
			close(results)
		}()

		// Collect results
		for res := range results {
			if res.success {
				ic.logger.Dim(fmt.Sprintf("✓ Image %s verified", res.img))
			} else {
				ic.logger.Errorf("✗ Image %s failed: %s", res.img, res.errMsg)
				failedImages = append(failedImages, res.img)
			}
		}
	}

	if len(failedImages) > 0 {
		ic.logger.Errorf("Failed to verify %d images:", len(failedImages))
		for _, img := range failedImages {
			ic.logger.Errorf("  - %s", img)
		}
		return false
	}

	cachedCount := len(cachedImages)
	verifiedCount := len(uncachedImages) - len(failedImages)
	ic.logger.Successf("✓ All %d images verified successfully (%d cached, %d verified)",
		len(images), cachedCount, verifiedCount)

	return true
}
