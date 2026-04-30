package deployer

import (
	"fmt"
	"strings"
)

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
				// TODO(ROX-34499): shouldn't this be an error instead? I think it's more likely someone
				// made a typo...
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
