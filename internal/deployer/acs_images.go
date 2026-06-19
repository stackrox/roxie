package deployer

import "fmt"

const (
	imageRegistry = "quay.io/rhacs-eng"
)

func imagesForConfig(config Config) []string {
	images := make([]string, 0)
	prefix := ""
	if config.Roxie.KonfluxImagesEnabled() {
		prefix = "release-"
	}

	images = append(images, fmt.Sprintf("%s/%s%s:%s", imageRegistry, prefix, "main", config.Roxie.Version))
	images = append(images, fmt.Sprintf("%s/%s%s:%s", imageRegistry, prefix, "central-db", config.Roxie.Version))
	images = append(images, fmt.Sprintf("%s/%s%s:%s", imageRegistry, prefix, "scanner-v4-db", config.Roxie.Version))
	images = append(images, fmt.Sprintf("%s/%s%s:%s", imageRegistry, prefix, "scanner-v4", config.Roxie.Version))
	if !config.Roxie.KonfluxImagesEnabled() {
		prefix = "stackrox-"
	}
	images = append(images, fmt.Sprintf("%s/%s%s:%s", imageRegistry, prefix, "operator", config.Operator.Version))
	images = append(images, fmt.Sprintf("%s/%s%s:v%s", imageRegistry, prefix, "operator-bundle", config.Operator.Version))

	return images
}

func OperatorBundleImage(config Config) string {
	if config.Roxie.KonfluxImagesEnabled() {
		return fmt.Sprintf("%s/release-operator-bundle:v%s", imageRegistry, config.Operator.Version)
	}
	return fmt.Sprintf("%s/stackrox-operator-bundle:v%s", imageRegistry, config.Operator.Version)
}
