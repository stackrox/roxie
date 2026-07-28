package deployer

import (
	"fmt"

	"github.com/stackrox/roxie/internal/constants"
)

func imagesForConfig(config Config) []string {
	var images []string
	prefix := ""
	if config.Roxie.KonfluxImagesEnabled() {
		prefix = "release-"
	}

	imageRegistry := constants.DefaultRegistry

	for _, mainTag := range uniqueMainVersions(config) {
		images = append(images,
			fmt.Sprintf("%s/%s%s:%s", imageRegistry, prefix, "main", mainTag),
			fmt.Sprintf("%s/%s%s:%s", imageRegistry, prefix, "central-db", mainTag),
			fmt.Sprintf("%s/%s%s:%s", imageRegistry, prefix, "scanner-v4-db", mainTag),
			fmt.Sprintf("%s/%s%s:%s", imageRegistry, prefix, "scanner-v4", mainTag),
		)
	}

	operatorPrefix := prefix
	if !config.Roxie.KonfluxImagesEnabled() {
		operatorPrefix = "stackrox-"
	}
	for _, instance := range config.OperatorInstances() {
		images = append(images,
			fmt.Sprintf("%s/%s%s:%s", imageRegistry, operatorPrefix, "operator", instance.Version),
			OperatorBundleImage(instance.Version, config.Roxie.KonfluxImagesEnabled()),
		)
	}

	return images
}

func uniqueMainVersions(config Config) []string {
	central := config.CentralVersion()
	sc := config.SecuredClusterVersion()
	if central == sc {
		return []string{central}
	}
	return []string{central, sc}
}

// OperatorBundleImage returns the operator bundle image for a specific operator version.
func OperatorBundleImage(operatorVersion string, konflux bool) string {
	imageRegistry := constants.DefaultRegistry
	if konflux {
		return fmt.Sprintf("%s/release-operator-bundle:v%s", imageRegistry, operatorVersion)
	}
	return fmt.Sprintf("%s/stackrox-operator-bundle:v%s", imageRegistry, operatorVersion)
}
