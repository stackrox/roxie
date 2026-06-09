package deployer

import (
	"context"
	"fmt"
	"os/exec"
	"strings"

	"github.com/stackrox/roxie/internal/containerrt"
	"github.com/stackrox/roxie/internal/logger"
)

type minikubeImagePreLoader struct {
	log *logger.Logger
	genericImageSender
}

var (
	minikubeGetImagesCommand = []string{"minikube", "ssh", "--", "crictl", "images", "-o", "json"}
)

func (d *Deployer) newMinikubeImagePreloader() (*minikubeImagePreLoader, error) {
	return &minikubeImagePreLoader{
		log:                d.logger,
		genericImageSender: newGenericImageSender(d.logger, "minikube", "image", "load", "<image>"),
	}, nil
}

func (k *minikubeImagePreLoader) Name() string {
	return "Image pre-loader for local Minikube clusters"
}

func (k *minikubeImagePreLoader) GetImages(ctx context.Context) ([]string, error) {
	output, err := exec.CommandContext(ctx, minikubeGetImagesCommand[0], minikubeGetImagesCommand[1:]...).Output()
	if err != nil {
		k.log.Warningf("Command %q failed: %v", strings.Join(minikubeGetImagesCommand, " "), err)
		for line := range strings.SplitSeq(strings.TrimSpace(string(output)), "\n") {
			k.log.Dimf("| %s", line)
		}
		return nil, fmt.Errorf("listing images in minikube node: %w", err)
	}
	return containerrt.ParseCrictlImages(output)
}
