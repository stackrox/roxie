package helpers

import (
	"bytes"
	"fmt"
	"os"
	"os/exec"
	"strings"

	"dario.cat/mergo"
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
func MergeMaps(base map[string]interface{}, overlays ...map[string]interface{}) (map[string]interface{}, error) {
	result := deepCopy(base)

	for _, overlay := range overlays {
		if err := DeepMerge(result, overlay); err != nil {
			return nil, err
		}
	}

	return result, nil
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

// DeepMerge recursively merges overlay into base.
// Slices are appended, with values from overlay appearing after values from base.
func DeepMerge(base, overlay map[string]any) error {
	return mergo.Merge(&base, &overlay, mergo.WithOverride, mergo.WithAppendSlice)
}

func MapToStruct(m map[string]interface{}, out interface{}) error {
	bytes, err := yaml.Marshal(m)
	if err != nil {
		return err
	}
	return yaml.Unmarshal(bytes, out)
}

// ShellQuote wraps s in POSIX single quotes, safe for use in shell eval contexts.
func ShellQuote(s string) string {
	return "'" + strings.ReplaceAll(s, "'", "'\\''") + "'"
}
