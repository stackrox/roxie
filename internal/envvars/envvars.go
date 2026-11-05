package envvars

import (
	"fmt"
	"maps"
	"slices"
	"strings"

	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
)

// deduplicateEnvVars deduplicates a Kubernetes-style envVars slice by name,
// keeping the last occurrence of each name. Returns a new slice sorted by name.
func deduplicate(envVars []any) []any {
	envVarMap := make(map[string]map[string]any)

	for _, item := range envVars {
		if envVarItem, ok := item.(map[string]any); ok {
			if name, ok := envVarItem["name"].(string); ok {
				envVarMap[name] = envVarItem
			}
		}
	}
	deduplicatedEnvVars := make([]any, 0, len(envVarMap))
	for _, name := range slices.Sorted(maps.Keys(envVarMap)) {
		deduplicatedEnvVars = append(deduplicatedEnvVars, envVarMap[name])
	}

	return deduplicatedEnvVars
}

// DeduplicateInUnstructured deduplicates the envVars slice at the given
// path inside an unstructured map. No-op if the path does not exist.
func DeduplicateInUnstructured(obj map[string]any, path ...string) error {
	envVars, found, err := unstructured.NestedSlice(obj, path...)
	if err != nil {
		return fmt.Errorf("retrieving nested slice at path %s from unstructured object: %w", strings.Join(path, "."), err)
	}
	if !found {
		return nil
	}
	return unstructured.SetNestedSlice(obj, deduplicate(envVars), path...)
}

// MergeByNameInUnstructured prepends baseEnvVars to the envVars already at
// path in obj, then deduplicates by name (last-one-wins, so current entries
// from an overlay take precedence over base). No-op if both sources are empty.
func MergeByNameInUnstructured(obj map[string]any, baseEnvVars []any, path ...string) error {
	current, _, err := unstructured.NestedSlice(obj, path...)
	if err != nil {
		return fmt.Errorf("retrieving nested slice at path %s from unstructured object: %w", strings.Join(path, "."), err)
	}
	merged := append(baseEnvVars, current...)
	if len(merged) == 0 {
		return nil
	}
	return unstructured.SetNestedSlice(obj, deduplicate(merged), path...)
}
