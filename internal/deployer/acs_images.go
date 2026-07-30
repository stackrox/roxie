package deployer

import (
	"fmt"

	"github.com/stackrox/roxie/internal/constants"
)

func imagesForConfig(config Config) []string {
	var images []string
	imageRegistry := constants.DefaultRegistry
	instances := config.OperatorInstances()
	mainTags := []string{config.CentralVersion(), config.SecuredClusterVersion()}

	for i, instance := range instances {
		prefix := ""
		operatorPrefix := "stackrox-"
		if instance.KonfluxImagesEnabled() {
			prefix = "release-"
			operatorPrefix = "release-"
		}
		images = append(images,
			fmt.Sprintf("%s/%s%s:%s", imageRegistry, prefix, "main", mainTags[i]),
			fmt.Sprintf("%s/%s%s:%s", imageRegistry, prefix, "central-db", mainTags[i]),
			fmt.Sprintf("%s/%s%s:%s", imageRegistry, prefix, "scanner-v4-db", mainTags[i]),
			fmt.Sprintf("%s/%s%s:%s", imageRegistry, prefix, "scanner-v4", mainTags[i]),
			fmt.Sprintf("%s/%s%s:%s", imageRegistry, operatorPrefix, "operator", instance.Version),
			instance.BundleImage(),
		)
	}

	return images
}
