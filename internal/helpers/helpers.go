package helpers

import (
	"bytes"
	"fmt"
	"os"
	"os/exec"

	"gopkg.in/yaml.v3"
)

// RunCommandWithOutput executes a command and captures stdout/stderr
func RunCommandWithOutput(title string, name string, args []string, opts ...CommandOption) (string, string, error) {
	var stdout, stderr bytes.Buffer

	cmd := exec.Command(name, args...)
	cmd.Stdout = &stdout
	cmd.Stderr = &stderr

	// Apply additional options
	for _, opt := range opts {
		opt(cmd)
	}

	err := cmd.Run()
	if err != nil {
		return stdout.String(), stderr.String(), fmt.Errorf("step '%s' failed: %w\nStderr: %s", title, err, stderr.String())
	}

	return stdout.String(), stderr.String(), nil
}

// CommandOption is a function that modifies an exec.Cmd
type CommandOption func(*exec.Cmd)

// GetContainerTool returns the container tool to use (podman)
func GetContainerTool() string {
	return "podman"
}

// LoadYAMLFile loads a YAML file and unmarshals it into a map
// Returns an empty map if path is empty or file doesn't exist
func LoadYAMLFile(path string) (map[string]interface{}, error) {
	if path == "" {
		return make(map[string]interface{}), nil
	}

	data, err := os.ReadFile(path)
	if err != nil {
		if os.IsNotExist(err) {
			return make(map[string]interface{}), nil
		}
		return nil, fmt.Errorf("failed to read YAML file %s: %w", path, err)
	}

	var result map[string]interface{}
	if err := yaml.Unmarshal(data, &result); err != nil {
		return nil, fmt.Errorf("failed to parse YAML file %s: %w", path, err)
	}

	if result == nil {
		return make(map[string]interface{}), nil
	}

	return result, nil
}

// MergeMaps deeply merges multiple maps, with later maps taking precedence
func MergeMaps(base map[string]interface{}, overlays ...map[string]interface{}) map[string]interface{} {
	result := deepCopy(base)

	for _, overlay := range overlays {
		deepMerge(result, overlay)
	}

	return result
}

// deepCopy creates a deep copy of a map
func deepCopy(m map[string]interface{}) map[string]interface{} {
	result := make(map[string]interface{})
	for k, v := range m {
		switch val := v.(type) {
		case map[string]interface{}:
			result[k] = deepCopy(val)
		case []interface{}:
			result[k] = deepCopySlice(val)
		default:
			result[k] = v
		}
	}
	return result
}

// deepCopySlice creates a deep copy of a slice
func deepCopySlice(s []interface{}) []interface{} {
	result := make([]interface{}, len(s))
	for i, v := range s {
		switch val := v.(type) {
		case map[string]interface{}:
			result[i] = deepCopy(val)
		case []interface{}:
			result[i] = deepCopySlice(val)
		default:
			result[i] = v
		}
	}
	return result
}

// deepMerge recursively merges overlay into base
func deepMerge(base, overlay map[string]interface{}) {
	for k, v := range overlay {
		if baseVal, ok := base[k]; ok {
			// Both are maps - merge recursively
			if baseMap, baseIsMap := baseVal.(map[string]interface{}); baseIsMap {
				if overlayMap, overlayIsMap := v.(map[string]interface{}); overlayIsMap {
					deepMerge(baseMap, overlayMap)
					continue
				}
			}
		}
		// Override with overlay value
		base[k] = v
	}
}
