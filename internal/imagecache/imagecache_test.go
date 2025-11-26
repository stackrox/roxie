package imagecache

import (
	"encoding/json"
	"os"
	"path/filepath"
	"testing"

	"github.com/stackrox/roxie/internal/logger"
)

func TestImageCacheLoadSaveRoundtrip(t *testing.T) {
	tmpDir := t.TempDir()
	cachePath := filepath.Join(tmpDir, ".roxie.image_cache")

	log := logger.New()
	c := New(log, cachePath, 20)

	if len(c.cache) != 0 {
		t.Errorf("Expected empty cache, got %d entries", len(c.cache))
	}

	// Add an image to cache
	c.AddToCache("quay.io/example/app:1")

	if !c.IsCached("quay.io/example/app:1") {
		t.Error("Image should be cached after adding")
	}

	// Reopen cache and verify persistence
	c2 := New(log, cachePath, 20)
	if !c2.IsCached("quay.io/example/app:1") {
		t.Error("Image should be cached after reopening")
	}
}

func TestImageCacheHandlesOldFormat(t *testing.T) {
	tmpDir := t.TempDir()
	cachePath := filepath.Join(tmpDir, ".roxie.image_cache")

	// Write old format (plain array)
	oldFormat := []string{"a", "b"}
	data, err := json.Marshal(oldFormat)
	if err != nil {
		t.Fatalf("Failed to marshal old format: %v", err)
	}

	if err := os.WriteFile(cachePath, data, 0644); err != nil {
		t.Fatalf("Failed to write cache file: %v", err)
	}

	log := logger.New()
	c := New(log, cachePath, 20)

	if !c.IsCached("a") {
		t.Error("Should load 'a' from old format")
	}
	if !c.IsCached("b") {
		t.Error("Should load 'b' from old format")
	}
}

func TestImageCacheMaxEntries(t *testing.T) {
	tmpDir := t.TempDir()
	cachePath := filepath.Join(tmpDir, ".roxie.image_cache")

	log := logger.New()
	maxEntries := 5
	c := New(log, cachePath, maxEntries)

	// Add more than maxEntries
	for i := 0; i < 10; i++ {
		c.AddToCache("image" + string(rune('0'+i)))
	}

	if len(c.cache) > maxEntries {
		t.Errorf("Cache should not exceed maxEntries=%d, got %d", maxEntries, len(c.cache))
	}

	// Verify most recent entries are kept
	if !c.IsCached("image9") {
		t.Error("Most recent entry should be in cache")
	}
}

func TestImageCacheMoveToEnd(t *testing.T) {
	tmpDir := t.TempDir()
	cachePath := filepath.Join(tmpDir, ".roxie.image_cache")

	log := logger.New()
	c := New(log, cachePath, 5)

	c.AddToCache("image1")
	c.AddToCache("image2")
	c.AddToCache("image3")

	// Re-add image1 (should move to end)
	c.AddToCache("image1")

	// Verify cache order
	if len(c.cache) != 3 {
		t.Errorf("Expected 3 entries, got %d", len(c.cache))
	}

	// Last entry should be image1
	if c.cache[len(c.cache)-1] != "image1" {
		t.Errorf("Expected image1 to be last, got %s", c.cache[len(c.cache)-1])
	}
}
