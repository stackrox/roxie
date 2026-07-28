package deployer

import (
	"fmt"
	"testing"

	"github.com/stackrox/roxie/internal/constants"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestKonfluxOperatorImage(t *testing.T) {
	expected := fmt.Sprintf("%s/release-operator:4.9.2", constants.DefaultRegistry)
	assert.Equal(t, expected, KonfluxOperatorImage("4.9.2"))
}

func TestPopulateKonfluxEnvVars_AllEntries(t *testing.T) {
	envVars := make(map[string]string)
	PopulateKonfluxEnvVars(envVars, "4.9.2")

	require.Len(t, envVars, len(konfluxRelatedImages))

	for envName, imageSuffix := range konfluxRelatedImages {
		expected := fmt.Sprintf("%s/release-%s:%s", constants.DefaultRegistry, imageSuffix, "4.9.2")
		assert.Equal(t, expected, envVars[envName], "mismatch for %s", envName)
	}
}

func TestPopulateKonfluxEnvVars_UserOverridePreserved(t *testing.T) {
	userValue := "quay.io/custom/my-main:latest"
	envVars := map[string]string{
		"RELATED_IMAGE_MAIN": userValue,
	}

	PopulateKonfluxEnvVars(envVars, "4.9.2")

	assert.Equal(t, userValue, envVars["RELATED_IMAGE_MAIN"],
		"user override should be preserved")

	assert.Len(t, envVars, len(konfluxRelatedImages),
		"all other entries should be populated")

	for envName, imageSuffix := range konfluxRelatedImages {
		if envName == "RELATED_IMAGE_MAIN" {
			continue
		}
		expected := fmt.Sprintf("%s/release-%s:%s", constants.DefaultRegistry, imageSuffix, "4.9.2")
		assert.Equal(t, expected, envVars[envName], "mismatch for %s", envName)
	}
}
