package deployer

import (
	"fmt"

	"github.com/stackrox/roxie/internal/constants"
	"github.com/stackrox/roxie/internal/helpers"
)

var konfluxRelatedImages = map[string]string{
	"RELATED_IMAGE_MAIN":            "main",
	"RELATED_IMAGE_CENTRAL_DB":      "central-db",
	"RELATED_IMAGE_SCANNER":         "scanner",
	"RELATED_IMAGE_SCANNER_SLIM":    "scanner-slim",
	"RELATED_IMAGE_SCANNER_DB":      "scanner-db",
	"RELATED_IMAGE_SCANNER_DB_SLIM": "scanner-db-slim",
	"RELATED_IMAGE_COLLECTOR":       "collector",
	"RELATED_IMAGE_SCANNER_V4_DB":   "scanner-v4-db",
	"RELATED_IMAGE_SCANNER_V4":      "scanner-v4",
	"RELATED_IMAGE_FACT":            "fact",
}

// KonfluxOperatorImage returns the Konflux-built operator image reference for an operator version.
func KonfluxOperatorImage(operatorVersion string) string {
	return fmt.Sprintf("%s/release-operator:%s", constants.DefaultRegistry, operatorVersion)
}

// PopulateKonfluxEnvVars adds RELATED_IMAGE_* entries to the instance's EnvVars
// for its version. It is a no-op when Konflux images are not enabled for the instance.
// Explicitly-provided env vars (e.g. from --operator-env) take precedence and are not overwritten.
func PopulateKonfluxEnvVars(instance *OperatorInstanceConfig) {
	if !instance.KonfluxImagesEnabled() {
		return
	}
	if instance.EnvVars == nil {
		instance.EnvVars = make(map[string]string, len(konfluxRelatedImages))
	}
	operatorTag := helpers.ConvertToOperatorTag(instance.Version)
	for envName, imageSuffix := range konfluxRelatedImages {
		if _, exists := instance.EnvVars[envName]; exists {
			continue
		}
		instance.EnvVars[envName] = fmt.Sprintf(
			"%s/release-%s:%s",
			constants.DefaultRegistry,
			imageSuffix,
			operatorTag,
		)
	}
}
