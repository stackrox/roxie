package deployer

import (
	"fmt"

	"github.com/stackrox/roxie/internal/constants"
)

func imagesForConfig(config Config) []string {
	var images []string
	imageRegistry := constants.DefaultRegistry

	for _, instance := range config.OperatorInstances() {
		prefix := ""
		operatorPrefix := "stackrox-"
		if instance.KonfluxImagesEnabled() {
			prefix = "release-"
			operatorPrefix = "release-"
		}
		images = append(images,
			fmt.Sprintf("%s/%s%s:%s", imageRegistry, prefix, "main", instance.Version),
			fmt.Sprintf("%s/%s%s:%s", imageRegistry, prefix, "central-db", instance.Version),
			fmt.Sprintf("%s/%s%s:%s", imageRegistry, prefix, "scanner-v4-db", instance.Version),
			fmt.Sprintf("%s/%s%s:%s", imageRegistry, prefix, "scanner-v4", instance.Version),
			fmt.Sprintf("%s/%s%s:%s", imageRegistry, operatorPrefix, "operator", instance.Version.ToOperatorTag().String()),
			instance.BundleImage(),
		)
	}

	return images
}
