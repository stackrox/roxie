package deployer

import (
	"fmt"
	"sort"
	"strings"
)

// ParseOperatorEnvVars parses a slice of operator environment variable strings.
// Each string can contain one or more comma-separated KEY=VALUE pairs.
// Values may contain '=' characters (only the first '=' is used as the separator).
func ParseOperatorEnvVars(envExprs []string) (map[string]string, error) {
	result := make(map[string]string)

	for _, expr := range envExprs {
		for _, part := range strings.Split(expr, ",") {
			part = strings.TrimSpace(part)
			if part == "" {
				continue
			}

			key, value, found := strings.Cut(part, "=")
			if !found {
				return nil, fmt.Errorf("invalid operator env var %q: expected KEY=VALUE format", part)
			}
			key = strings.TrimSpace(key)
			if key == "" {
				return nil, fmt.Errorf("invalid operator env var %q: empty key", part)
			}
			result[key] = value
		}
	}

	return result, nil
}

// operatorEnvVarsToSortedList converts a map of env vars to a sorted list of
// {name, value} maps suitable for use in Kubernetes container or OLM Subscription specs.
func operatorEnvVarsToSortedList(envVars map[string]string) []interface{} {
	result := make([]interface{}, 0, len(envVars))
	for name, value := range envVars {
		result = append(result, map[string]interface{}{
			"name":  name,
			"value": value,
		})
	}
	sort.Slice(result, func(i, j int) bool {
		return result[i].(map[string]interface{})["name"].(string) <
			result[j].(map[string]interface{})["name"].(string)
	})
	return result
}
