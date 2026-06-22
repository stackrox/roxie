package deployer

import (
	"context"
	"errors"
	"fmt"
	"slices"

	"github.com/stackrox/roxie/internal/containerrt"
	"github.com/stackrox/roxie/internal/types"
)

var (
	ErrLocalImagesUnsupported          = errors.New("cluster does not support deploying with local images")
	ErrLocalImageRetrievalNotSupported = errors.New("image pre-loader does not support retrieval of images available on the cluster")
)

// ImagePreLoader transfers container images into a local cluster's image store.
type ImagePreLoader interface {
	SendImage(ctx context.Context, imageTag string) error
	GetImages(ctx context.Context) ([]string, error)
	Name() string
}

// GetPreLoaderForCluster returns an ImagePreLoader suited for the detected
// cluster type, or ErrLocalImagesUnsupported if the cluster does not support it.
func (d *Deployer) GetPreLoaderForCluster() (ImagePreLoader, error) {
	switch d.config.Roxie.ClusterType {
	case types.ClusterTypeKind:
		return d.newKindImagePreloader()
	case types.ClusterTypeMinikube:
		return d.newMinikubeImagePreloader()
	default:
		return nil, ErrLocalImagesUnsupported
	}
}

// TryTransferLocalImages sends locally available ACS images to the cluster
// via the given pre-loader, skipping images already present in the cluster.
func (d *Deployer) TryTransferLocalImages(ctx context.Context, preLoader ImagePreLoader) error {
	localImages, err := d.collectLocalImages(ctx)
	if err != nil {
		d.logger.Dimf("Collecting local images failed: %v", err)
		return err
	}
	if len(localImages) == 0 {
		d.logger.Dim("No local images found")
		return nil
	}

	availableImagesInCluster, err := preLoader.GetImages(ctx)
	if err != nil && !errors.Is(err, ErrLocalImageRetrievalNotSupported) {
		return fmt.Errorf("retrieving list of images available in local cluster: %w", err)
	}

	// Simple for now.
	for _, image := range localImages {
		if slices.Contains(availableImagesInCluster, image) {
			// Exists already in local cluster registry.
			d.logger.Dimf("Image %s already available in local cluster, skipping.", image)
			continue
		}
		d.logger.Dimf("Transferring local image %s to local cluster...", image)
		if err := preLoader.SendImage(ctx, image); err != nil {
			d.logger.Warningf("Transferring local image %s to %s cluster failed: %s",
				image, d.config.Roxie.ClusterType, err)
		}
	}

	return nil
}

// collectLocalImages returns the subset of ACS images that exist in the local
// container runtime (Docker/Podman). Returns nil if no runtime is reachable.
func (d *Deployer) collectLocalImages(ctx context.Context) ([]string, error) {
	socket := d.containerRuntimeSocket
	if socket == "" {
		return nil, nil
	}

	if d.verbose {
		d.logger.Dimf("Using container runtime socket %s", socket)
	}
	available, err := containerrt.ListLocalImages(ctx, socket)
	if err != nil {
		d.logger.Dimf("Could not query container runtime at %s: %v", socket, err)
		return nil, err
	}

	availableSet := make(map[string]struct{}, len(available))
	for _, img := range available {
		availableSet[img] = struct{}{}
	}

	wanted := imagesForConfig(d.config)
	localImages := make([]string, 0, len(wanted))
	for _, img := range wanted {
		if _, ok := availableSet[img]; ok {
			d.logger.Dimf("Image %s exists locally", img)
			localImages = append(localImages, img)
		} else {
			d.logger.Dimf("Image %s needs to be pulled from registry", img)
		}
	}
	return localImages, nil
}
