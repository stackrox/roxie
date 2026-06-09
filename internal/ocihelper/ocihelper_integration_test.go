//go:build integration

package ocihelper

import (
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/stackrox/roxie/internal/logger"
)

func TestExtractManifestsFromImage_Integration(t *testing.T) {
	// Use a known stable operator bundle image.
	bundleImage := "quay.io/rhacs-eng/stackrox-operator-bundle:v4.10.0"

	destDir, err := os.MkdirTemp("", "test-bundle-extract-")
	if err != nil {
		t.Fatalf("Failed to create temp dir: %v", err)
	}
	defer os.RemoveAll(destDir)

	log := logger.New()
	ctx := context.Background()

	t.Logf("Extracting manifests from %s", bundleImage)
	err = ExtractManifestsFromImage(ctx, log, bundleImage, destDir, "")
	if err != nil {
		t.Fatalf("ExtractManifestsFromImage failed: %v", err)
	}

	// Verify expected files exist.
	expectedFiles := []string{
		"rhacs-operator.clusterserviceversion.yaml",
		"rhacs-operator-controller-manager-metrics-service_v1_service.yaml",
		"rhacs-operator-metrics-reader_rbac.authorization.k8s.io_v1_clusterrole.yaml",
	}

	for _, expectedFile := range expectedFiles {
		filePath := filepath.Join(destDir, expectedFile)
		if _, err := os.Stat(filePath); os.IsNotExist(err) {
			t.Errorf("Expected file %s not found in extracted manifests", expectedFile)
		} else {
			t.Logf("✓ Found expected file: %s", expectedFile)
		}
	}

	// Verify that extracted files are non-empty.
	entries, err := os.ReadDir(destDir)
	if err != nil {
		t.Fatalf("Failed to read destination directory: %v", err)
	}

	if len(entries) == 0 {
		t.Fatal("No files were extracted")
	}

	t.Logf("Successfully extracted %d files", len(entries))

	// Verify CSV file contents and size.
	csvPath := filepath.Join(destDir, "rhacs-operator.clusterserviceversion.yaml")
	csvContent, err := os.ReadFile(csvPath)
	if err != nil {
		t.Fatalf("Failed to read CSV file: %v", err)
	}

	// Verify exact size for pinned image version.
	expectedCSVSize := 105398
	if len(csvContent) != expectedCSVSize {
		t.Errorf("CSV file size mismatch: expected %d bytes, got %d bytes", expectedCSVSize, len(csvContent))
	}

	// Basic YAML structure check.
	csvStr := string(csvContent)
	if !strings.Contains(csvStr, "apiVersion") {
		t.Error("CSV file does not appear to be valid Kubernetes YAML (missing apiVersion)")
	}

	t.Logf("✓ CSV file verified (%d bytes)", len(csvContent))
}

func TestVerifyImageExistence_Integration(t *testing.T) {
	bundleImage := "quay.io/rhacs-eng/stackrox-operator-bundle:v4.10.0"

	log := logger.New()
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()

	t.Logf("Verifying image %s exists", bundleImage)
	err := VerifyImageExistence(ctx, log, bundleImage)
	if err != nil {
		t.Fatalf("VerifyImageExistence failed: %v", err)
	}

	t.Log("✓ VerifyImageExistence succeeded for existing image")
}

func TestVerifyImageExistence_NonExistent_Integration(t *testing.T) {
	nonExistentImage := "quay.io/rhacs-eng/this-image-does-not-exist:v999.999.999"

	log := logger.New()
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()

	t.Logf("Verifying image %s does not exist", nonExistentImage)
	err := VerifyImageExistence(ctx, log, nonExistentImage)
	if err == nil {
		t.Fatal("Expected VerifyImageExistence to fail for non-existent image, but it succeeded")
	}

	t.Log("✓ VerifyImageExistence correctly failed for non-existent image")
}
