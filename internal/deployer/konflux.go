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

// KonfluxOperatorImage returns the Konflux-built operator image reference for a version.
func KonfluxOperatorImage(operatorVersion string) string {
	return fmt.Sprintf("%s/release-operator:%s", constants.DefaultRegistry, operatorVersion)
}

// MergeKonfluxEnvVars returns a copy of envVars with RELATED_IMAGE_* entries filled
// for the given operator version. Explicitly-provided env vars take precedence.
func MergeKonfluxEnvVars(envVars map[string]string, operatorVersion string) map[string]string {
	result := copyStringMap(envVars)
	for envName, imageSuffix := range konfluxRelatedImages {
		if _, exists := result[envName]; exists {
			continue
		}
		result[envName] = fmt.Sprintf(
			"%s/release-%s:%s",
			constants.DefaultRegistry,
			imageSuffix,
			operatorVersion, // Konflux built images use the "operator tag".
		)
	}
	return result
}

// PopulateKonfluxEnvVars populates config.Operator.EnvVars with RELATED_IMAGE_*
// entries for Konflux image rewriting. Used for the single-operator / OLM path
// where env vars live on the top-level OperatorConfig.
func PopulateKonfluxEnvVars(config *Config) {
	config.Operator.EnvVars = MergeKonfluxEnvVars(config.Operator.EnvVars, config.Operator.Version)
}
