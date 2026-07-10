package deployer

import (
	"fmt"
	"testing"

	"github.com/stackrox/roxie/internal/constants"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestKonfluxOperatorImage(t *testing.T) {
	config := &Config{
		Operator: OperatorConfig{Version: "4.9.2"},
	}
	expected := fmt.Sprintf("%s/release-operator:4.9.2", constants.DefaultRegistry)
	assert.Equal(t, expected, KonfluxOperatorImage(config))
}

func TestPopulateKonfluxEnvVars_AllEntries(t *testing.T) {
	config := &Config{
		Operator: OperatorConfig{Version: "4.9.2"},
	}

	PopulateKonfluxEnvVars(config)

	require.Len(t, config.Operator.EnvVars, len(konfluxRelatedImages))

	for envName, imageSuffix := range konfluxRelatedImages {
		expected := fmt.Sprintf("%s/release-%s:%s", constants.DefaultRegistry, imageSuffix, "4.9.2")
		assert.Equal(t, expected, config.Operator.EnvVars[envName], "mismatch for %s", envName)
	}
}

func TestPopulateKonfluxEnvVars_UserOverridePreserved(t *testing.T) {
	userValue := "quay.io/custom/my-main:latest"
	config := &Config{
		Operator: OperatorConfig{
			Version: "4.9.2",
			EnvVars: map[string]string{
				"RELATED_IMAGE_MAIN": userValue,
			},
		},
	}

	PopulateKonfluxEnvVars(config)

	assert.Equal(t, userValue, config.Operator.EnvVars["RELATED_IMAGE_MAIN"],
		"user override should be preserved")

	assert.Len(t, config.Operator.EnvVars, len(konfluxRelatedImages),
		"all other entries should be populated")

	for envName, imageSuffix := range konfluxRelatedImages {
		if envName == "RELATED_IMAGE_MAIN" {
			continue
		}
		expected := fmt.Sprintf("%s/release-%s:%s", constants.DefaultRegistry, imageSuffix, "4.9.2")
		assert.Equal(t, expected, config.Operator.EnvVars[envName], "mismatch for %s", envName)
	}
}

func TestPopulateKonfluxEnvVars_NilEnvVarsMap(t *testing.T) {
	config := &Config{
		Operator: OperatorConfig{Version: "4.9.2"},
	}
	assert.Nil(t, config.Operator.EnvVars)

	PopulateKonfluxEnvVars(config)

	require.NotNil(t, config.Operator.EnvVars)
	assert.Len(t, config.Operator.EnvVars, len(konfluxRelatedImages))
}
