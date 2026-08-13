package deployer

import (
	"fmt"
)

func imagesForConfig(config Config) []string {
	var images []string
	imageRegistry := config.Roxie.Registry()

	for _, instance := range config.OperatorInstances() {
		prefix := ""
		if instance.KonfluxImagesEnabled() {
			prefix = "release-"
		}
		images = append(images,
			fmt.Sprintf("%s/%s%s:%s", imageRegistry, prefix, "main", instance.Version),
			fmt.Sprintf("%s/%s%s:%s", imageRegistry, prefix, "central-db", instance.Version),
			fmt.Sprintf("%s/%s%s:%s", imageRegistry, prefix, "scanner-v4-db", instance.Version),
			fmt.Sprintf("%s/%s%s:%s", imageRegistry, prefix, "scanner-v4", instance.Version),
			instance.OperatorImage(imageRegistry),
			instance.BundleImage(imageRegistry),
		)
	}

	return images
}
