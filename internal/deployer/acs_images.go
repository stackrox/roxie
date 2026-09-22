package deployer

import (
	"fmt"
)

func imagesForConfig(config Config) []string {
	var images []string

	for _, instance := range config.OperatorInstances() {
		prefix := ""
		if instance.KonfluxImagesEnabled() {
			prefix = "release-"
		}
		images = append(images,
			fmt.Sprintf("%s/%s%s:%s", instance.ImageRegistry, prefix, "main", instance.Version),
			fmt.Sprintf("%s/%s%s:%s", instance.ImageRegistry, prefix, "central-db", instance.Version),
			fmt.Sprintf("%s/%s%s:%s", instance.ImageRegistry, prefix, "scanner-v4-db", instance.Version),
			fmt.Sprintf("%s/%s%s:%s", instance.ImageRegistry, prefix, "scanner-v4", instance.Version),
			instance.OperatorImage(),
			instance.BundleImage(),
		)
	}

	return images
}
