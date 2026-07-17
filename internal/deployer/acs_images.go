package deployer

import (
	"fmt"

	"github.com/stackrox/roxie/internal/constants"
	"github.com/stackrox/roxie/internal/helpers"
)

func imagesForConfig(config Config) []string {
	images := make([]string, 0)
	prefix := ""
	if config.Roxie.KonfluxImagesEnabled() {
		prefix = "release-"
	}

	imageRegistry := constants.DefaultRegistry
	seen := make(map[string]bool)
	add := func(image string) {
		if seen[image] {
			return
		}
		seen[image] = true
		images = append(images, image)
	}

	for _, mainTag := range uniqueMainVersions(config) {
		add(fmt.Sprintf("%s/%s%s:%s", imageRegistry, prefix, "main", mainTag))
		add(fmt.Sprintf("%s/%s%s:%s", imageRegistry, prefix, "central-db", mainTag))
		add(fmt.Sprintf("%s/%s%s:%s", imageRegistry, prefix, "scanner-v4-db", mainTag))
		add(fmt.Sprintf("%s/%s%s:%s", imageRegistry, prefix, "scanner-v4", mainTag))
	}

	operatorPrefix := prefix
	if !config.Roxie.KonfluxImagesEnabled() {
		operatorPrefix = "stackrox-"
	}
	for _, instance := range config.OperatorInstances() {
		add(fmt.Sprintf("%s/%s%s:%s", imageRegistry, operatorPrefix, "operator", instance.Version))
		add(OperatorBundleImageForVersion(instance.Version, config.Roxie.KonfluxImagesEnabled()))
	}

	return images
}

func uniqueMainVersions(config Config) []string {
	versions := []string{config.EffectiveCentralVersion(), config.EffectiveSecuredClusterVersion()}
	seen := make(map[string]bool)
	var unique []string
	for _, v := range versions {
		if v == "" || seen[v] {
			continue
		}
		seen[v] = true
		unique = append(unique, v)
	}
	if len(unique) == 0 && config.Roxie.Version != "" {
		unique = append(unique, config.Roxie.Version)
	}
	return unique
}

// OperatorBundleImage returns the operator bundle image for the top-level operator version.
func OperatorBundleImage(config Config) string {
	version := config.Operator.Version
	if version == "" {
		version = helpers.ConvertMainTagToOperatorTag(config.Roxie.Version)
	}
	return OperatorBundleImageForVersion(version, config.Roxie.KonfluxImagesEnabled())
}

// OperatorBundleImageForVersion returns the operator bundle image for a specific operator version.
func OperatorBundleImageForVersion(operatorVersion string, konflux bool) string {
	imageRegistry := constants.DefaultRegistry
	if konflux {
		return fmt.Sprintf("%s/release-operator-bundle:v%s", imageRegistry, operatorVersion)
	}
	return fmt.Sprintf("%s/stackrox-operator-bundle:v%s", imageRegistry, operatorVersion)
}
