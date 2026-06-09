package ocihelper

import (
	"archive/tar"
	"context"
	"fmt"
	"io"
	"os"
	"path/filepath"

	"github.com/google/go-containerregistry/pkg/authn"
	"github.com/google/go-containerregistry/pkg/name"
	v1 "github.com/google/go-containerregistry/pkg/v1"
	"github.com/google/go-containerregistry/pkg/v1/daemon"
	"github.com/google/go-containerregistry/pkg/v1/remote"

	"github.com/stackrox/roxie/internal/logger"
)

// VerifyImageExistence verifies that an OCI image is accessible.
// Authentication is handled automatically from ~/.docker/config.json or $REGISTRY_AUTH_FILE.
func VerifyImageExistence(ctx context.Context, log *logger.Logger, imageRef string) error {
	log.Dimf("Inspecting image %s", imageRef)

	ref, err := name.ParseReference(imageRef)
	if err != nil {
		return fmt.Errorf("invalid image reference: %w", err)
	}

	// Use HEAD request to verify image exists without downloading
	_, err = remote.Head(ref,
		remote.WithContext(ctx),
		remote.WithAuthFromKeychain(authn.DefaultKeychain))
	if err != nil {
		return fmt.Errorf("image inspection failed: %w", err)
	}

	log.Dim("✓ Image is accessible")
	return nil
}

// ExtractManifestsFromImage extracts the /manifests/ directory from an operator bundle image.
// Authentication is handled automatically from ~/.docker/config.json or $REGISTRY_AUTH_FILE.
func ExtractManifestsFromImage(ctx context.Context, log *logger.Logger, imageRef, destDir string) error {
	tempDir, err := os.MkdirTemp("", "oci-image-")
	if err != nil {
		return fmt.Errorf("failed to create temp dir: %w", err)
	}
	defer os.RemoveAll(tempDir)

	log.Dimf("Using temporary directory: %s", tempDir)

	img, err := assureImageExistsLocally(ctx, log, imageRef)
	if err != nil {
		return err
	}

	log.Dim("Extracting /manifests/ directory from image layers...")
	if err := extractManifestsFromImage(log, img, tempDir, destDir); err != nil {
		return err
	}

	log.Dimf("✓ Manifests extracted to: %s", destDir)
	return nil
}

func assureImageExistsLocally(ctx context.Context, log *logger.Logger, imageRef string) (v1.Image, error) {
	log.Dimf("Fetching image %s", imageRef)

	ref, err := name.ParseReference(imageRef)
	if err != nil {
		return nil, fmt.Errorf("invalid image reference: %w", err)
	}

	img, err := daemon.Image(ref, daemon.WithContext(ctx))
	if err == nil {
		log.Dimf("✓ Image %s found in local daemon", imageRef)
		return img, nil
	}

	log.Dimf("Image not found in local daemon, pulling from registry: %v", err)

	// For operator bundles, we fetch linux/amd64 by default as they contain
	// platform-agnostic YAML files.
	platform := v1.Platform{
		OS:           "linux",
		Architecture: "amd64",
	}

	img, err = remote.Image(ref,
		remote.WithContext(ctx),
		remote.WithAuthFromKeychain(authn.DefaultKeychain),
		remote.WithPlatform(platform))
	if err != nil {
		return nil, fmt.Errorf("failed to fetch image: %w", err)
	}

	log.Dim("✓ Image fetched successfully")
	return img, nil
}

// extractManifestsFromImage extracts /manifests/ from an OCI image.
func extractManifestsFromImage(log *logger.Logger, img v1.Image, tempExtractDir, destDir string) error {
	layers, err := img.Layers()
	if err != nil {
		return fmt.Errorf("failed to get image layers: %w", err)
	}

	log.Dimf("Found %d layer(s) in image", len(layers))

	// Extract all layers into tempExtractDir
	for i, layer := range layers {
		log.Dimf("Extracting layer %d/%d...", i+1, len(layers))
		if err := extractLayerToDir(log, layer, tempExtractDir); err != nil {
			return fmt.Errorf("failed to extract layer %d: %w", i+1, err)
		}
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

// extractLayerToDir extracts a single image layer to a directory.
func extractLayerToDir(log *logger.Logger, layer v1.Layer, destDir string) error {
	rc, err := layer.Uncompressed()
	if err != nil {
		return fmt.Errorf("failed to get layer contents: %w", err)
	}
	defer rc.Close()

	return extractTarToDir(log, rc, destDir)
}

// extractTarToDir extracts an uncompressed tar stream to a directory.
func extractTarToDir(log *logger.Logger, r io.Reader, destDir string) error {
	// Open a Root directory to prevent path traversal attacks.
	root, err := os.OpenRoot(destDir)
	if err != nil {
		return fmt.Errorf("failed to open directory %q as root directory: %w", destDir, err)
	}
	defer root.Close()

	tr := tar.NewReader(r)

	for {
		header, err := tr.Next()
		if err == io.EOF {
			break
		}
		if err != nil {
			return fmt.Errorf("failed to read tar header: %w", err)
		}

		entryName := header.Name

		switch header.Typeflag {
		case tar.TypeDir:
			err := root.MkdirAll(entryName, 0755)
			if err != nil {
				return err
			}
		case tar.TypeReg:
			outFile, err := root.OpenFile(entryName, os.O_CREATE|os.O_WRONLY|os.O_TRUNC, os.FileMode(header.Mode))
			if err != nil {
				return fmt.Errorf("failed to create file %s: %w", entryName, err)
			}

			if _, err := io.Copy(outFile, tr); err != nil {
				outFile.Close()
				return fmt.Errorf("failed to write file %s: %w", entryName, err)
			}
			outFile.Close()
		default:
			log.Dimf("Skipping unsupported tar entry %s of type %c", entryName, header.Typeflag)
			continue
		}
	}

	return nil
}
