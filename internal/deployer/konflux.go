package deployer

import (
	"fmt"

	"github.com/stackrox/roxie/internal/constants"
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

// KonfluxOperatorImage returns the Konflux-built operator image reference.
func KonfluxOperatorImage(config *Config) string {
	return fmt.Sprintf("%s/release-operator:%s", constants.DefaultRegistry, config.Operator.Version)
}

// PopulateKonfluxEnvVars populates config.Operator.EnvVars with RELATED_IMAGE_*
// entries for Konflux image rewriting. Explicitly-provided env vars (e.g. from
// --operator-env) take precedence and are not overwritten.
func PopulateKonfluxEnvVars(config *Config) {
	if config.Operator.EnvVars == nil {
		config.Operator.EnvVars = make(map[string]string)
	}
	for envName, imageSuffix := range konfluxRelatedImages {
		if _, exists := config.Operator.EnvVars[envName]; exists {
			continue
		}
		config.Operator.EnvVars[envName] = fmt.Sprintf(
			"%s/release-%s:%s",
			constants.DefaultRegistry,
			imageSuffix,
			config.Operator.Version, // Konflux built images use the "operator tag".
		)
	}
}
