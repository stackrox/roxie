package deployer

import (
	"fmt"
	"testing"

	"github.com/stackrox/roxie/internal/constants"
	"github.com/stackrox/roxie/internal/imagetag"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestOperatorImage_Konflux(t *testing.T) {
	instance := OperatorInstanceConfig{Version: "4.9.2", KonfluxImages: new(true), ImageRegistry: constants.DefaultRegistry}
	expected := fmt.Sprintf("%s/release-operator:4.9.2", constants.DefaultRegistry)
	assert.Equal(t, expected, instance.OperatorImage())
}

func TestOperatorImage_NonKonflux(t *testing.T) {
	instance := OperatorInstanceConfig{Version: "4.9.2", KonfluxImages: new(false), ImageRegistry: constants.DefaultRegistry}
	expected := fmt.Sprintf("%s/stackrox-operator:4.9.2", constants.DefaultRegistry)
	assert.Equal(t, expected, instance.OperatorImage())
}

func TestOperatorImage_RegistryOverride(t *testing.T) {
	instance := OperatorInstanceConfig{Version: "4.9.2", ImageRegistry: "quay.io/stackrox-io"}
	assert.Equal(t, "quay.io/stackrox-io/stackrox-operator:4.9.2", instance.OperatorImage())
}

func TestPopulateKonfluxEnvVars_AllEntries(t *testing.T) {
	instance := &OperatorInstanceConfig{Version: "4.9.2", KonfluxImages: new(true)}
	populateOperatorEnvVars(instance)

	require.Len(t, instance.EnvVars, len(relatedImages))

	for envName, imageSuffix := range relatedImages {
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

	populateOperatorEnvVars(instance)

	assert.Equal(t, userValue, instance.EnvVars["RELATED_IMAGE_MAIN"],
		"user override should be preserved")

	assert.Len(t, instance.EnvVars, len(relatedImages),
		"all other entries should be populated")

	for envName, imageSuffix := range relatedImages {
		if envName == "RELATED_IMAGE_MAIN" {
			continue
		}
		expected := fmt.Sprintf("%s/release-%s:%s", constants.DefaultRegistry, imageSuffix, "4.9.2")
		assert.Equal(t, expected, instance.EnvVars[envName], "mismatch for %s", envName)
	}
}

func TestConstructOperandImageFromInstance(t *testing.T) {
	const img = "some-image"

	tests := []struct {
		name     string
		instance OperatorInstanceConfig
		expected string
	}{
		{
			name: "non-konflux plain version",
			instance: OperatorInstanceConfig{
				Version:       "4.9.2",
				ImageRegistry: constants.DefaultRegistry,
			},
			expected: constants.DefaultRegistry + "/" + img + ":4.9.2",
		},
		{
			name: "non-konflux dirty tag preserved",
			instance: OperatorInstanceConfig{
				Version:       "4.11.x-dirty",
				ImageRegistry: constants.DefaultRegistry,
			},
			expected: constants.DefaultRegistry + "/" + img + ":4.11.x-dirty",
		},
		{
			name: "non-konflux custom registry",
			instance: OperatorInstanceConfig{
				Version:       "4.9.2",
				ImageRegistry: "quay.io/stackrox-io",
			},
			expected: "quay.io/stackrox-io/" + img + ":4.9.2",
		},
		{
			name: "konflux adds release- prefix",
			instance: OperatorInstanceConfig{
				Version:       "4.9.2",
				KonfluxImages: new(true),
				ImageRegistry: constants.DefaultRegistry,
			},
			expected: constants.DefaultRegistry + "/release-" + img + ":4.9.2",
		},
		{
			name: "konflux normalizes dirty tag via ToOperatorTag",
			instance: OperatorInstanceConfig{
				Version:       "4.11.x-dirty",
				KonfluxImages: new(true),
				ImageRegistry: constants.DefaultRegistry,
			},
			expected: constants.DefaultRegistry + "/release-" + img + ":4.11.0",
		},
		{
			name: "konflux ignores custom registry",
			instance: OperatorInstanceConfig{
				Version:       "4.9.2",
				KonfluxImages: new(true),
				ImageRegistry: "quay.io/stackrox-io",
			},
			expected: constants.DefaultRegistry + "/release-" + img + ":4.9.2",
		},
		{
			name: "operand tag overrides version",
			instance: OperatorInstanceConfig{
				Version:       "4.9.2",
				OperandTag:    "4.10.0",
				ImageRegistry: constants.DefaultRegistry,
			},
			expected: constants.DefaultRegistry + "/" + img + ":4.10.0",
		},
		{
			name: "operand tag overrides version in konflux mode",
			instance: OperatorInstanceConfig{
				Version:       "4.9.2",
				OperandTag:    "4.10.x-dirty",
				KonfluxImages: new(true),
				ImageRegistry: constants.DefaultRegistry,
			},
			expected: constants.DefaultRegistry + "/release-" + img + ":4.10.0",
		},
		{
			name: "per-image overwrite",
			instance: OperatorInstanceConfig{
				Version:       "4.9.2",
				ImageRegistry: constants.DefaultRegistry,
				OperandTagOverwrites: map[string]imagetag.MainTag{
					img: "4.10.0",
				},
			},
			expected: constants.DefaultRegistry + "/" + img + ":4.10.0",
		},
		{
			name: "per-image overwrite does not affect other images",
			instance: OperatorInstanceConfig{
				Version:       "4.9.2",
				ImageRegistry: constants.DefaultRegistry,
				OperandTagOverwrites: map[string]imagetag.MainTag{
					"other-image": "4.10.0",
				},
			},
			expected: constants.DefaultRegistry + "/" + img + ":4.9.2",
		},
		{
			name: "per-image overwrite in konflux mode uses operator tag",
			instance: OperatorInstanceConfig{
				Version:       "4.9.2",
				KonfluxImages: new(true),
				ImageRegistry: constants.DefaultRegistry,
				OperandTagOverwrites: map[string]imagetag.MainTag{
					img: "4.10.x-dirty",
				},
			},
			expected: constants.DefaultRegistry + "/release-" + img + ":4.10.0",
		},
		{
			name: "per-image overwrite in konflux does not affect other images",
			instance: OperatorInstanceConfig{
				Version:       "4.9.2",
				KonfluxImages: new(true),
				ImageRegistry: constants.DefaultRegistry,
				OperandTagOverwrites: map[string]imagetag.MainTag{
					"other-image": "4.10.x-dirty",
				},
			},
			expected: constants.DefaultRegistry + "/release-" + img + ":4.9.2",
		},
		{
			name: "all three set: overwrite wins for matched image",
			instance: OperatorInstanceConfig{
				Version:       "4.9.2",
				OperandTag:    "4.10.0",
				ImageRegistry: constants.DefaultRegistry,
				OperandTagOverwrites: map[string]imagetag.MainTag{
					img: "4.11.0",
				},
			},
			expected: constants.DefaultRegistry + "/" + img + ":4.11.0",
		},
		{
			name: "all three set: operand tag wins for non-matched image",
			instance: OperatorInstanceConfig{
				Version:       "4.9.2",
				OperandTag:    "4.10.0",
				ImageRegistry: constants.DefaultRegistry,
				OperandTagOverwrites: map[string]imagetag.MainTag{
					"other-image": "4.11.0",
				},
			},
			expected: constants.DefaultRegistry + "/" + img + ":4.10.0",
		},
		{
			name: "all three set in konflux: overwrite wins for matched image",
			instance: OperatorInstanceConfig{
				Version:       "4.9.x-dirty",
				OperandTag:    "4.10.x-dirty",
				KonfluxImages: new(true),
				ImageRegistry: constants.DefaultRegistry,
				OperandTagOverwrites: map[string]imagetag.MainTag{
					img: "4.11.x-dirty",
				},
			},
			expected: constants.DefaultRegistry + "/release-" + img + ":4.11.0",
		},
		{
			name: "all three set in konflux: operand tag wins for non-matched image",
			instance: OperatorInstanceConfig{
				Version:       "4.9.x-dirty",
				OperandTag:    "4.10.x-dirty",
				KonfluxImages: new(true),
				ImageRegistry: constants.DefaultRegistry,
				OperandTagOverwrites: map[string]imagetag.MainTag{
					"other-image": "4.11.x-dirty",
				},
			},
			expected: constants.DefaultRegistry + "/release-" + img + ":4.10.0",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := constructOperandImageFromInstance(tt.instance, img)
			assert.Equal(t, tt.expected, got)
		})
	}
}

func TestPopulateOperatorEnvVars_OverwriteResetsToVersion(t *testing.T) {
	const pinnedImage = "scanner-v4"

	instance := &OperatorInstanceConfig{
		Version:       "4.9.2",
		OperandTag:    "4.10.0",
		ImageRegistry: constants.DefaultRegistry,
		OperandTagOverwrites: map[string]imagetag.MainTag{
			pinnedImage: "4.9.2", // Pin this image back to the operator version.
		},
	}
	populateOperatorEnvVars(instance)

	envNameForPinnedImage := ""
	for envName, imageName := range relatedImages {
		if imageName == pinnedImage {
			envNameForPinnedImage = envName
			continue
		}
		expected := fmt.Sprintf("%s/%s:%s", constants.DefaultRegistry, imageName, "4.10.0")
		assert.Equal(t, expected, instance.EnvVars[envName],
			"non-pinned image %s should use operand tag", imageName)
	}

	assert.NotContains(t, instance.EnvVars, envNameForPinnedImage,
		"pinned image reset to version should not need an env var override")
}
