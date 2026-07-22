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

func TestMergeKonfluxEnvVars_PerInstanceVersion(t *testing.T) {
	base := map[string]string{"CUSTOM": "1"}
	merged := MergeKonfluxEnvVars(base, "4.8.0")

	assert.Equal(t, "1", merged["CUSTOM"])
	assert.Equal(t, fmt.Sprintf("%s/release-main:4.8.0", constants.DefaultRegistry), merged["RELATED_IMAGE_MAIN"])
	assert.Equal(t, "1", base["CUSTOM"], "base map should not be mutated for existing keys")
	_, ok := base["RELATED_IMAGE_MAIN"]
	assert.False(t, ok, "base map should not receive new keys")
}
