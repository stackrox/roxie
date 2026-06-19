package deployer

import (
	"context"
	"fmt"
	"strings"

	"github.com/stackrox/roxie/internal/containerrt"
	"github.com/stackrox/roxie/internal/logger"
)

type kindImagePreLoader struct {
	log *logger.Logger
	genericImageSender
	kindClusterName        string
	containerRuntimeSocket string
}

func (d *Deployer) newKindImagePreloader() (*kindImagePreLoader, error) {
	kindClusterName := kubeContextToKindClusterName(d.kubeContext)
	d.logger.Dimf("Kind cluster name is %s", kindClusterName)
	return &kindImagePreLoader{
		log:                    d.logger,
		kindClusterName:        kindClusterName,
		genericImageSender:     newGenericImageSender(d.logger, "kind", "load", "docker-image", "<image>", "--name", kindClusterName),
		containerRuntimeSocket: d.containerRuntimeSocket,
	}, nil
}

func (k *kindImagePreLoader) GetImages(ctx context.Context) ([]string, error) {
	if k.containerRuntimeSocket == "" {
		return nil, nil
	}

	nodeName := k.kindClusterName + "-control-plane"
	output, err := containerrt.ExecInContainer(ctx, k.log, k.containerRuntimeSocket, nodeName,
		[]string{"crictl", "images", "-o", "json"})
	if err != nil {
		return nil, fmt.Errorf("listing images in kind node %s: %w", nodeName, err)
	}

	return containerrt.ParseCrictlImages(output)
}

func (k *kindImagePreLoader) Name() string {
	return "Image pre-loader for local Kind clusters"
}

func kubeContextToKindClusterName(kubeContext string) string {
	// This seems to be a convention for kind clusters.
	return strings.TrimPrefix(kubeContext, "kind-")
}
