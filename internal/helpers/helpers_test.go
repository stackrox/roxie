package helpers

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/require"
)

func TestMergeMaps(t *testing.T) {
	base := map[string]interface{}{
		"a": "value_a",
		"b": map[string]interface{}{
			"c": "value_c",
			"d": "value_d",
		},
	}

	overlay := map[string]interface{}{
		"b": map[string]interface{}{
			"d": "value_d_override",
			"e": "value_e_new",
		},
		"f": "value_f",
	}

	result, err := MergeMaps(base, overlay)
	require.NoError(t, err, "MergeMaps failed")

	// Check that base values are preserved
	if result["a"] != "value_a" {
		t.Errorf("Expected a='value_a', got '%v'", result["a"])
	}

	// Check deep merge
	bMap, ok := result["b"].(map[string]interface{})
	if !ok {
		t.Fatal("Expected b to be a map")
	}

	if bMap["c"] != "value_c" {
		t.Errorf("Expected b.c='value_c', got '%v'", bMap["c"])
	}

	if bMap["d"] != "value_d_override" {
		t.Errorf("Expected b.d to be overridden to 'value_d_override', got '%v'", bMap["d"])
	}

	if bMap["e"] != "value_e_new" {
		t.Errorf("Expected b.e='value_e_new', got '%v'", bMap["e"])
	}

	// Check new top-level key
	if result["f"] != "value_f" {
		t.Errorf("Expected f='value_f', got '%v'", result["f"])
	}
}

func TestMergeMapsMultipleOverlays(t *testing.T) {
	base := map[string]interface{}{
		"key": "base",
	}

	overlay1 := map[string]interface{}{
		"key": "overlay1",
	}

	overlay2 := map[string]interface{}{
		"key": "overlay2",
	}

	result, err := MergeMaps(base, overlay1, overlay2)
	require.NoError(t, err, "MergeMaps failed")

	if result["key"] != "overlay2" {
		t.Errorf("Expected last overlay to win, got '%v'", result["key"])
	}
}

func TestLoadYAMLFileValid(t *testing.T) {
	tmpDir := t.TempDir()
	yamlPath := filepath.Join(tmpDir, "test.yaml")

	yamlContent := `
key1: value1
key2:
  nested: value2
`
	if err := os.WriteFile(yamlPath, []byte(yamlContent), 0644); err != nil {
		t.Fatalf("Failed to write test file: %v", err)
	}

	result, err := LoadYAMLFile(yamlPath)
	if err != nil {
		t.Fatalf("LoadYAMLFile failed: %v", err)
	}

	if result["key1"] != "value1" {
		t.Errorf("Expected key1='value1', got '%v'", result["key1"])
	}

	key2Map, ok := result["key2"].(map[string]interface{})
	if !ok {
		t.Fatal("Expected key2 to be a map")
	}

	if key2Map["nested"] != "value2" {
		t.Errorf("Expected key2.nested='value2', got '%v'", key2Map["nested"])
	}
}

func TestLoadYAMLFileEmpty(t *testing.T) {
	result, err := LoadYAMLFile("")
	if err != nil {
		t.Errorf("LoadYAMLFile with empty path should not error: %v", err)
	}

	if len(result) != 0 {
		t.Errorf("Expected empty map for empty path, got %d entries", len(result))
	}
}

func TestLoadYAMLFileNonExistent(t *testing.T) {
	result, err := LoadYAMLFile("/nonexistent/file.yaml")
	if err != nil {
		t.Errorf("LoadYAMLFile with nonexistent file should return empty map, got error: %v", err)
	}

	if len(result) != 0 {
		t.Errorf("Expected empty map for nonexistent file, got %d entries", len(result))
	}
}

func TestLoadYAMLFileInvalidYAML(t *testing.T) {
	tmpDir := t.TempDir()
	yamlPath := filepath.Join(tmpDir, "invalid.yaml")

	invalidYAML := "this: is: not: valid: yaml: syntax"
	if err := os.WriteFile(yamlPath, []byte(invalidYAML), 0644); err != nil {
		t.Fatalf("Failed to write test file: %v", err)
	}

	_, err := LoadYAMLFile(yamlPath)
	if err == nil {
		t.Error("Expected error for invalid YAML")
	}
}

func TestDeepMergeAppendsSlices(t *testing.T) {
	base := map[string]interface{}{
		"spec": map[string]interface{}{
			"imagePullSecrets": []interface{}{
				map[string]interface{}{"name": "secret-a"},
			},
			"keep": "this",
		},
	}

	overlay := map[string]interface{}{
		"spec": map[string]interface{}{
			"imagePullSecrets": []interface{}{
				map[string]interface{}{"name": "secret-b"},
			},
		},
	}

	err := DeepMerge(base, overlay)
	require.NoError(t, err)

	spec := base["spec"].(map[string]interface{})
	require.Equal(t, "this", spec["keep"])

	secrets := spec["imagePullSecrets"].([]interface{})
	require.Len(t, secrets, 2)
	require.Equal(t, map[string]interface{}{"name": "secret-a"}, secrets[0])
	require.Equal(t, map[string]interface{}{"name": "secret-b"}, secrets[1])
}

func TestDeepMergeSliceTypeMismatch(t *testing.T) {
	base := map[string]interface{}{
		"items": []interface{}{"a"},
	}
	overlay := map[string]interface{}{
		"items": "not-a-slice",
	}

	err := DeepMerge(base, overlay)
	require.ErrorContains(t, err, "incompatible types")
}

func TestDeepCopy(t *testing.T) {
	original := map[string]interface{}{
		"a": "value",
		"b": map[string]interface{}{
			"c": "nested",
		},
	}

	copied := deepCopy(original)

	// Modify the copy
	copied["a"] = "modified"
	copiedB := copied["b"].(map[string]interface{})
	copiedB["c"] = "modified_nested"

	// Verify original is unchanged
	if original["a"] != "value" {
		t.Error("Original should not be modified")
	}

	originalB := original["b"].(map[string]interface{})
	if originalB["c"] != "nested" {
		t.Error("Original nested value should not be modified")
	}
}
