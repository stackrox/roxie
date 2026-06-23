package deployer

import (
	"fmt"
	"sort"
	"strings"
)

// ParseOperatorEnvVar parses a single KEY=VALUE environment variable string.
// Values may contain '=' characters (only the first '=' is used as the separator).
func ParseOperatorEnvVar(envExpr string) (string, string, error) {
	key, value, found := strings.Cut(envExpr, "=")
	if !found {
		return "", "", fmt.Errorf("invalid operator env var %q: expected KEY=VALUE format", envExpr)
	}
	if key == "" {
		return "", "", fmt.Errorf("invalid operator env var %q: empty key", envExpr)
	}
	return key, value, nil
}

// envVarsToSortedList converts a map of env vars to a sorted list of
// {name, value} maps suitable for use in Kubernetes container or OLM Subscription specs.
func envVarsToSortedList(envVars map[string]string) []interface{} {
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
