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
	assert.Equal(t, expected, instance.OperatorImage())
}

func TestOperatorImage_NonKonflux(t *testing.T) {
	instance := OperatorInstanceConfig{Version: "4.9.2", KonfluxImages: new(false)}
	expected := fmt.Sprintf("%s/stackrox-operator:4.9.2", constants.DefaultRegistry)
	assert.Equal(t, expected, instance.OperatorImage())
}

func TestPopulateKonfluxEnvVars_AllEntries(t *testing.T) {
	instance := &OperatorInstanceConfig{Version: "4.9.2", KonfluxImages: new(true)}
	PopulateKonfluxEnvVars(instance)

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

	PopulateKonfluxEnvVars(instance)

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

func TestPopulateKonfluxEnvVars_NoOpWhenDisabled(t *testing.T) {
	instance := &OperatorInstanceConfig{Version: "4.9.2", KonfluxImages: new(false)}
	PopulateKonfluxEnvVars(instance)
	assert.Nil(t, instance.EnvVars)
}
