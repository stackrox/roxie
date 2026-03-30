package deployer

import (
	"fmt"
	"slices"
	"sort"
	"strconv"
	"strings"

	"github.com/stackrox/roxie/internal/helpers"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
)

// parseFlagWithEquals parses a feature flag in the format "ROX_FOO=true" or "ROX_FOO=false"
func parseFlagWithEquals(part string) (name string, value bool, err error) {
	idx := strings.Index(part, "=")
	if idx == -1 {
		return "", false, fmt.Errorf("internal error: no equals sign found")
	}

	name = strings.TrimSpace(part[:idx])
	valueStr := strings.TrimSpace(part[idx+1:])

	if !strings.HasPrefix(name, "ROX_") || len(name) <= 4 {
		return "", false, fmt.Errorf("invalid feature flag name %q: must be in format ROX_<NAME>", name)
	}

	value, err = strconv.ParseBool(valueStr)
	if err != nil {
		return "", false, fmt.Errorf("invalid boolean value %q for feature flag %s", valueStr, name)
	}

	return name, value, nil
}

// parseFlagWithPrefix parses a feature flag in the format "+ROX_FOO", "-ROX_FOO", or "ROX_FOO"
func parseFlagWithPrefix(part string) (name string, value bool, err error) {
	value = true // default

	if strings.HasPrefix(part, "+") {
		name = strings.TrimSpace(part[1:])
	} else if strings.HasPrefix(part, "-") {
		name = strings.TrimSpace(part[1:])
		value = false
	} else {
		name = strings.TrimSpace(part)
	}

	if !strings.HasPrefix(name, "ROX_") || len(name) <= 4 {
		return "", false, fmt.Errorf("invalid feature flag name %q: must be in format ROX_<NAME>", name)
	}

	return name, value, nil
}

// parseFeatureFlags parses a slice of feature flag strings and returns a map of flag names to boolean values.
// Supports formats: +ROX_FOO (enable), -ROX_FOO (disable), ROX_FOO=true, ROX_FOO=false, ROX_FOO (enable)
func parseFeatureFlags(flags []string) (map[string]bool, error) {
	result := make(map[string]bool)

	for _, flagStr := range flags {
		parts := strings.Split(flagStr, ",")
		for _, part := range parts {
			part = strings.TrimSpace(part)
			if part == "" {
				continue
			}

			var name string
			var value bool
			var err error

			if strings.Contains(part, "=") {
				name, value, err = parseFlagWithEquals(part)
			} else {
				name, value, err = parseFlagWithPrefix(part)
			}

			if err != nil {
				return nil, err
			}

			result[name] = value
		}
	}

	return result, nil
}

// featureFlagsToEnvVars converts feature flags to envVar maps
func featureFlagsToEnvVars(flags map[string]bool) []interface{} {
	envVars := make([]interface{}, 0, len(flags))
	for name, value := range flags {
		envVars = append(envVars, map[string]interface{}{
			"name":  name,
			"value": strconv.FormatBool(value),
		})
	}
	return envVars
}

// featureFlagsToOverrides converts feature flags to a CR override structure
func featureFlagsToOverrides(flags map[string]bool) map[string]interface{} {
	if len(flags) == 0 {
		return nil
	}

	return map[string]interface{}{
		"spec": map[string]interface{}{
			"customize": map[string]interface{}{
				"envVars": featureFlagsToEnvVars(flags),
			},
		},
	}
}

// mergeEnvVars smartly merges envVars arrays, with newer values overriding older ones by name.
// This allows feature flags to override specific env vars without wiping the entire array.
func mergeEnvVars(base, overlay []interface{}) []interface{} {
	envVarMap := make(map[string]interface{})

	for _, item := range slices.Concat(base, overlay) {
		if envVar, ok := item.(map[string]interface{}); ok {
			if name, ok := envVar["name"].(string); ok {
				envVarMap[name] = envVar
			}
		}
	}

	result := make([]interface{}, 0, len(envVarMap))
	for _, envVar := range envVarMap {
		result = append(result, envVar)
	}

	sort.Slice(result, func(i, j int) bool {
		return result[i].(map[string]interface{})["name"].(string) <
			result[j].(map[string]interface{})["name"].(string)
	})

	return result
}

// mergeWithEnvVarSupport merges two maps, with special handling for spec.customize.envVars arrays.
// Instead of replacing the entire envVars array, it merges individual env vars by name,
// allowing overlay to override specific env vars while preserving others from base.
func mergeWithEnvVarSupport(base, overlay map[string]interface{}) map[string]interface{} {
	result := helpers.MergeMaps(base, overlay)

	baseEnvVars, baseFound, _ := unstructured.NestedSlice(base, "spec", "customize", "envVars")
	overlayEnvVars, overlayFound, _ := unstructured.NestedSlice(overlay, "spec", "customize", "envVars")

	if baseFound && overlayFound {
		_ = unstructured.SetNestedSlice(result, mergeEnvVars(baseEnvVars, overlayEnvVars), "spec", "customize", "envVars")
	}

	return result
}
