package deployer

import (
	"fmt"

	"github.com/stackrox/roxie/internal/constants"
	"github.com/stackrox/roxie/internal/imagetag"
)

var relatedImages = map[string]string{
	"RELATED_IMAGE_MAIN":          "main",
	"RELATED_IMAGE_CENTRAL_DB":    "central-db",
	"RELATED_IMAGE_COLLECTOR":     "collector",
	"RELATED_IMAGE_SCANNER_V4_DB": "scanner-v4-db",
	"RELATED_IMAGE_SCANNER_V4":    "scanner-v4",
	"RELATED_IMAGE_FACT":          "fact",

	// Old scanner images need to stay for now to be able to deploy older ACS versions.
	"RELATED_IMAGE_SCANNER":         "scanner",
	"RELATED_IMAGE_SCANNER_SLIM":    "scanner-slim",
	"RELATED_IMAGE_SCANNER_DB":      "scanner-db",
	"RELATED_IMAGE_SCANNER_DB_SLIM": "scanner-db-slim",
}

// TODO: Update comment.
// populateKonfluxEnvVars adds RELATED_IMAGE_* entries to the instance's EnvVars
// for its version. Explicitly-provided env vars (e.g. from --operator-env) take
// precedence and are not overwritten.
func populateOperatorEnvVars(instance *OperatorInstanceConfig) {
	if instance == nil {
		// Nothing to do.
		return
	}

	envVars := make(map[string]string)

	for envName, imageName := range relatedImages {
		envVars[envName] = constructOperandImageFromInstance(*instance, imageName)
	}

	instanceWithoutOverwrites := *instance
	instanceWithoutOverwrites.OperandTag = imagetag.MainTag("")
	instanceWithoutOverwrites.OperandTagOverwrites = nil

	if instance.EnvVars == nil {
		instance.EnvVars = make(map[string]string)
	}
	for envName, imageRef := range envVars {
		if _, exists := instance.EnvVars[envName]; exists {
			// Explicitly set env vars for related images take precedence.
			continue
		}
		if !instance.KonfluxImagesEnabled() {
			// Only add env var if it differs from what the operator would use anyway.
			// For Konflux mode we need to add the overwrites unconditionally, because the operator
			// happily references non-existing downstream operand images.
			defaultImageRef := constructOperandImageFromInstance(instanceWithoutOverwrites, relatedImages[envName])
			if imageRef == defaultImageRef {
				continue
			}
		}
		instance.EnvVars[envName] = imageRef
	}
}

func constructOperandImageFromInstance(instance OperatorInstanceConfig, imageName string) string {
	imagePrefix := ""
	useKonflux := instance.KonfluxImagesEnabled()
	if useKonflux {
		imagePrefix = "release-"
	}
	imageRegistry := constants.DefaultRegistry
	if !useKonflux && instance.ImageRegistry != "" { // Konflux images are only supported for default registry.
		imageRegistry = instance.ImageRegistry
	}

	// The top-level version in an OperatorInstanceConfig can be
	// selectively overwritten for specific images.
	effectiveOperandImageVersion := instance.Version
	if instance.OperandTag.IsSet() {
		effectiveOperandImageVersion = instance.OperandTag
	}
	if perImageVersion := instance.OperandTagOverwrites[imageName]; perImageVersion.IsSet() {
		// A custom version for this particular image is requested.
		effectiveOperandImageVersion = perImageVersion
	}

	var effectiveVersionString string
	if useKonflux {
		// Konflux built images use the "operator tag".
		effectiveVersionString = effectiveOperandImageVersion.ToOperatorTag().String()
	} else {
		effectiveVersionString = effectiveOperandImageVersion.String()
	}

	return fmt.Sprintf("%s/%s%s:%s", imageRegistry, imagePrefix, imageName, effectiveVersionString)
}
