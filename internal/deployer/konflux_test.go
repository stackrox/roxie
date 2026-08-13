package deployer

import (
	"fmt"
	"testing"

	"github.com/stackrox/roxie/internal/constants"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestOperatorImage_Konflux(t *testing.T) {
	instance := OperatorInstanceConfig{Version: "4.9.2", KonfluxImages: new(true)}
	expected := fmt.Sprintf("%s/release-operator:4.9.2", constants.DefaultRegistry)
	assert.Equal(t, expected, instance.OperatorImage(constants.DefaultRegistry))
}

func TestOperatorImage_NonKonflux(t *testing.T) {
	instance := OperatorInstanceConfig{Version: "4.9.2", KonfluxImages: new(false)}
	expected := fmt.Sprintf("%s/stackrox-operator:4.9.2", constants.DefaultRegistry)
	assert.Equal(t, expected, instance.OperatorImage(constants.DefaultRegistry))
}

func TestOperatorImage_RegistryOverride(t *testing.T) {
	instance := OperatorInstanceConfig{Version: "4.9.2"}
	assert.Equal(t, "quay.io/stackrox-io/stackrox-operator:4.9.2", instance.OperatorImage("quay.io/stackrox-io"))
}

func TestPopulateKonfluxEnvVars_AllEntries(t *testing.T) {
	instance := &OperatorInstanceConfig{Version: "4.9.2", KonfluxImages: new(true)}
	populateKonfluxEnvVars(instance)

	require.Len(t, instance.EnvVars, len(konfluxRelatedImages))

	for envName, imageSuffix := range konfluxRelatedImages {
		expected := fmt.Sprintf("%s/release-%s:%s", constants.DefaultRegistry, imageSuffix, "4.9.2")
		assert.Equal(t, expected, instance.EnvVars[envName], "mismatch for %s", envName)
	}
}

func TestPopulateKonfluxEnvVars_UserOverridePreserved(t *testing.T) {
	userValue := "quay.io/custom/my-main:latest"
	instance := &OperatorInstanceConfig{
		Version:       "4.9.2",
		KonfluxImages: new(true),
		EnvVars: map[string]string{
			"RELATED_IMAGE_MAIN": userValue,
		},
	}

	populateKonfluxEnvVars(instance)

	assert.Equal(t, userValue, instance.EnvVars["RELATED_IMAGE_MAIN"],
		"user override should be preserved")

	assert.Len(t, instance.EnvVars, len(konfluxRelatedImages),
		"all other entries should be populated")

	for envName, imageSuffix := range konfluxRelatedImages {
		if envName == "RELATED_IMAGE_MAIN" {
			continue
		}
		expected := fmt.Sprintf("%s/release-%s:%s", constants.DefaultRegistry, imageSuffix, "4.9.2")
		assert.Equal(t, expected, instance.EnvVars[envName], "mismatch for %s", envName)
	}
}
