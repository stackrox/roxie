package deployer

import (
	"fmt"
	"os"
	"strings"

	"gopkg.in/yaml.v3"
)

func GetOverrides(overrideFile string, overrideSetExpressions []string) (map[string]interface{}, error) {
	overrides := make(map[string]interface{})
	if overrideFile != "" {
		content, err := os.ReadFile(overrideFile)
		if err != nil {
			return nil, fmt.Errorf("failed to read override file: %w", err)
		}
		if err := yaml.Unmarshal(content, &overrides); err != nil {
			return nil, fmt.Errorf("failed to parse override file: %w", err)
		}
	}

	for _, expr := range overrideSetExpressions {
		// Split expr at '=' to get path and value
		parts := splitAtFirstEquals(expr)
		if len(parts) != 2 {
			return nil, fmt.Errorf("invalid override expression '%s': expected format 'key.path=value'", expr)
		}

		path := parts[0]
		valueStr := parts[1]

		// Parse the value as YAML
		var value interface{}
		if err := yaml.Unmarshal([]byte(valueStr), &value); err != nil {
			return nil, fmt.Errorf("failed to parse value '%s' as YAML: %w", valueStr, err)
		}

		// Set the value at the specified path
		if err := setNestedValue(overrides, path, value); err != nil {
			return nil, fmt.Errorf("failed to set override for path '%s': %w", path, err)
		}
	}

	return overrides, nil
}

// splitAtFirstEquals splits a string at the first '=' character
// Returns [key, value] or just [expr] if no '=' found
func splitAtFirstEquals(expr string) []string {
	idx := strings.Index(expr, "=")
	if idx == -1 {
		return []string{expr}
	}
	return []string{expr[:idx], expr[idx+1:]}
}

// setNestedValue sets a value at a nested path in a map
// The path is a dot-separated string like "foo.bar.baz"
// Creates intermediate maps as needed
func setNestedValue(m map[string]interface{}, path string, value interface{}) error {
	if path == "" {
		return fmt.Errorf("path cannot be empty")
	}

	parts := strings.Split(path, ".")
	current := m

	// Navigate to the parent of the target key
	for i := 0; i < len(parts)-1; i++ {
		key := parts[i]

		if existing, ok := current[key]; ok {
			// Check if existing value is a map
			if existingMap, isMap := existing.(map[string]interface{}); isMap {
				current = existingMap
			} else {
				// Existing value is not a map, we need to replace it
				newMap := make(map[string]interface{})
				current[key] = newMap
				current = newMap
			}
		} else {
			// Key doesn't exist, create a new map
			newMap := make(map[string]interface{})
			current[key] = newMap
			current = newMap
		}
	}

	// Set the final value
	finalKey := parts[len(parts)-1]
	current[finalKey] = value

	return nil
}
