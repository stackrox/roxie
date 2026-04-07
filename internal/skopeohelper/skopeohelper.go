package skopeohelper

import (
	"archive/tar"
	"compress/gzip"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"strings"

	"github.com/stackrox/roxie/internal/logger"
)

// ExtractManifestsFromImage extracts the /manifests/ directory from an operator bundle image.
// Authentication is handled automatically by skopeo from ~/.docker/config.json or $REGISTRY_AUTH_FILE.
func ExtractManifestsFromImage(ctx context.Context, log *logger.Logger, imageRef, destDir string) error {
	tempDir, err := os.MkdirTemp("", "skopeo-image-")
	if err != nil {
		return fmt.Errorf("failed to create temp dir: %w", err)
	}
	defer os.RemoveAll(tempDir)

	log.Dimf("Using temporary directory: %s", tempDir)

	if err := copyImageToDir(ctx, log, imageRef, tempDir); err != nil {
		return err
	}

	log.Dim("Extracting /manifests/ directory from image layers...")
	if err := extractManifestsFromDir(log, tempDir, destDir); err != nil {
		return err
	}

	log.Dimf("✓ Manifests extracted to: %s", destDir)
	return nil
}

// InspectImage verifies that an OCI image is accessible.
// Authentication is handled automatically by skopeo from ~/.docker/config.json or $REGISTRY_AUTH_FILE.
func InspectImage(ctx context.Context, log *logger.Logger, imageRef string) error {
	log.Dimf("Inspecting image %s", imageRef)

	args := []string{
		"inspect",
		"--override-os", "linux",
		"--override-arch", "amd64",
		withDockerTransport(imageRef),
	}

	cmd := exec.CommandContext(ctx, "skopeo", args...)
	output, err := cmd.CombinedOutput()
	if err != nil {
		log.Dimf("skopeo command output: %s", string(output))
		return fmt.Errorf("skopeo inspect failed: %w", err)
	}

	log.Dim("✓ Image is accessible")
	return nil
}

// copyImageToDir downloads an OCI image to a local directory.
func copyImageToDir(ctx context.Context, log *logger.Logger, imageRef, destDir string) error {
	log.Dimf("Copying image %s to %s", imageRef, destDir)

	args := []string{
		"copy",
		"--override-os", "linux",
		"--override-arch", "amd64",
		withDockerTransport(imageRef),
		fmt.Sprintf("dir:%s", destDir),
	}

	cmd := exec.CommandContext(ctx, "skopeo", args...)
	output, err := cmd.CombinedOutput()
	if err != nil {
		log.Dimf("skopeo command output: %s", string(output))
		return fmt.Errorf("skopeo copy failed: %w", err)
	}

	log.Dim("✓ Image copied successfully")
	return nil
}

type imageManifest struct {
	Layers []imageLayer `json:"layers"`
}

type imageLayer struct {
	Digest string `json:"digest"`
}

// extractManifestsFromDir extracts /manifests/ from a skopeo dir-format image.
func extractManifestsFromDir(log *logger.Logger, skopeoDir, destDir string) error {
	manifestPath := filepath.Join(skopeoDir, "manifest.json")
	manifestData, err := os.ReadFile(manifestPath)
	if err != nil {
		return fmt.Errorf("failed to read manifest: %w", err)
	}

	var manifest imageManifest
	if err := json.Unmarshal(manifestData, &manifest); err != nil {
		return fmt.Errorf("failed to parse manifest: %w", err)
	}

	// Extract all image layers into tempExtractDir.
	log.Dimf("Found %d layer(s) in image", len(manifest.Layers))
	tempExtractDir, err := os.MkdirTemp("", "layer-extract-")
	if err != nil {
		return fmt.Errorf("failed to create temp extract dir: %w", err)
	}
	defer os.RemoveAll(tempExtractDir)

	if err := extractLayers(log, manifest.Layers, skopeoDir, tempExtractDir); err != nil {
		return err
	}

	// From the directory into which all layers have been extracted, copy the
	// /manifests/ directory to the destination.
	log.Dim("Copying manifests to destination...")
	manifestsDir := filepath.Join(tempExtractDir, "manifests")
	if _, err := os.Stat(manifestsDir); err != nil {
		return fmt.Errorf("no /manifests directory found in image: %w", err)
	}

	return os.CopyFS(destDir, os.DirFS(manifestsDir))
}

// extractLayers extracts all image layers to a destination directory.
func extractLayers(log *logger.Logger, layers []imageLayer, skopeoDir, destDir string) error {
	for i, layer := range layers {
		log.Dimf("Extracting layer %d/%d...", i+1, len(layers))
		layerFilename := strings.TrimPrefix(layer.Digest, "sha256:")
		layerFile := filepath.Join(skopeoDir, layerFilename)
		if err := extractTarToDir(layerFile, destDir); err != nil {
			return fmt.Errorf("failed to extract layer %s: %w", layer.Digest, err)
		}
	}
	return nil
}

// extractTarToDir extracts a gzip-compressed tar file to a directory.
func extractTarToDir(tarPath, destDir string) error {
	f, err := os.Open(tarPath)
	if err != nil {
		return fmt.Errorf("failed to open tar file: %w", err)
	}
	defer f.Close()

	gzr, err := gzip.NewReader(f)
	if err != nil {
		return fmt.Errorf("failed to create gzip reader: %w", err)
	}
	defer gzr.Close()

	tr := tar.NewReader(gzr)
	dirPermissions := make(map[string]os.FileMode) // Map to track directory permissions.

	for {
		header, err := tr.Next()
		if err == io.EOF {
			break
		}
		if err != nil {
			return fmt.Errorf("failed to read tar header: %w", err)
		}

		// Prevent path traversal attacks.
		target := filepath.Join(destDir, header.Name)
		relPath, err := filepath.Rel(destDir, target)
		if err != nil || strings.HasPrefix(relPath, ".."+string(filepath.Separator)) || relPath == ".." {
			continue
		}

		switch header.Typeflag {
		case tar.TypeSymlink, tar.TypeLink:
			// Skip symlinks and hardlinks for security.
		case tar.TypeDir:
			if err := os.MkdirAll(target, 0755); err != nil {
				return fmt.Errorf("failed to create directory: %w", err)
			}
			dirPermissions[target] = os.FileMode(header.Mode)
		case tar.TypeReg:
			if err := os.MkdirAll(filepath.Dir(target), 0755); err != nil {
				return fmt.Errorf("failed to create parent directory: %w", err)
			}

			outFile, err := os.OpenFile(target, os.O_CREATE|os.O_WRONLY|os.O_TRUNC, os.FileMode(header.Mode))
			if err != nil {
				return fmt.Errorf("failed to create file %s: %w", target, err)
			}

			if _, err := io.Copy(outFile, tr); err != nil {
				outFile.Close()
				return fmt.Errorf("failed to write file %s: %w", target, err)
			}
			outFile.Close()
		}
	}

	for dir, perm := range dirPermissions {
		if err := os.Chmod(dir, perm); err != nil {
			return fmt.Errorf("failed to set directory permissions for %s: %w", dir, err)
		}
	}

	return nil
}

func withDockerTransport(imageRef string) string {
	return "docker://" + imageRef
}
