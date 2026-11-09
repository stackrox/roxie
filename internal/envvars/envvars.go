package envvars

import (
	"fmt"
	"regexp"
	"strings"

	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
)

var (
	// Note, we match the dollars in a seperate match group to be able to check for escaped dollar characters.
	varRefPattern = regexp.MustCompile(`(\$+)\(([^)]+)\)`)
)

// UnresolvedRef describes an env var that references a variable not
// yet defined at that point in the list.
type UnresolvedRef struct {
	Name    string
	RefName string
}

// FindUnresolvedRefs checks a Kubernetes-style envVars slice for $(VAR)
// references that Kubernetes cannot resolve because the referent is not
// defined at the point of reference (absent or appears later in the list).
// Also handles escaping such as "$$(NOT_GETTING_INTERPOLATED)".
func FindUnresolvedRefs(envVars []any) []UnresolvedRef {
	defined := make(map[string]bool)
	var refs []UnresolvedRef

	for _, item := range envVars {
		ev, ok := item.(map[string]any)
		if !ok {
			continue
		}
		name, ok := ev["name"].(string)
		if !ok {
			continue
		}
		if ev["value"] == nil && ev["valueFrom"] == nil {
			continue
		}
		defined[name] = true
		val, ok := ev["value"].(string)
		if !ok {
			continue
		}
		for _, match := range varRefPattern.FindAllStringSubmatch(val, -1) {
			dollars := match[1]
			refName := match[2]
			if len(dollars)%2 == 0 {
				continue // even number of $ — escaped, e.g. $$(VAR)
			}
			if !defined[refName] {
				refs = append(refs, UnresolvedRef{Name: name, RefName: refName})
			}
		}
	}
	return refs
}

// deduplicate deduplicates a Kubernetes-style envVars slice by name.
// First occurrence of a name determines its position in the output;
// last occurrence determines its value. This preserves the dependency
// ordering required by Kubernetes' $(VAR) interpolation: a variable
// that references another via $(VAR) must appear after the referent.
//
// Limitation: if a later source introduces a NEW variable that an
// earlier source already references via $(VAR), the new variable will
// appear after the reference and Kubernetes will not resolve it.
func deduplicate(envVars []any) []any {
	var result []any
	indices := make(map[string]int) // Stores name-keyed indices in result slice.

	// Takes an any-item and returns its name and its complete unstructured obj.
	// Returns non-empty name on success.
	extractItem := func(item any) (name string, envVarItem map[string]any) {
		var ok bool
		envVarItem, ok = item.(map[string]any)
		if !ok {
			return
		}
		name, _ = envVarItem["name"].(string)
		return
	}

	for _, item := range envVars {
		name, envVarItem := extractItem(item)
		if name == "" {
			continue
		}
		if i, exists := indices[name]; exists {
			result[i] = envVarItem
		} else {
			indices[name] = len(result)
			result = append(result, envVarItem)
		}
	}
	return result
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
