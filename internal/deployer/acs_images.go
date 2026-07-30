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
		if instance.KonfluxImages {
			prefix = "release-"
			operatorPrefix = "release-"
		}
		images = append(images,
			fmt.Sprintf("%s/%s%s:%s", imageRegistry, prefix, "main", mainTags[i]),
			fmt.Sprintf("%s/%s%s:%s", imageRegistry, prefix, "central-db", mainTags[i]),
			fmt.Sprintf("%s/%s%s:%s", imageRegistry, prefix, "scanner-v4-db", mainTags[i]),
			fmt.Sprintf("%s/%s%s:%s", imageRegistry, prefix, "scanner-v4", mainTags[i]),
			fmt.Sprintf("%s/%s%s:%s", imageRegistry, operatorPrefix, "operator", instance.Version),
			OperatorBundleImage(instance.Version, instance.KonfluxImages),
		)
	}

	return images
}

// OperatorBundleImage returns the operator bundle image for a specific operator version.
func OperatorBundleImage(operatorVersion string, konflux bool) string {
	imageRegistry := constants.DefaultRegistry
	if konflux {
		return fmt.Sprintf("%s/release-operator-bundle:v%s", imageRegistry, operatorVersion)
	}
	return fmt.Sprintf("%s/stackrox-operator-bundle:v%s", imageRegistry, operatorVersion)
}
