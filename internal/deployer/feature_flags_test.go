package deployer

import (
	"reflect"
	"testing"
)

func TestParseFeatureFlags(t *testing.T) {
	tests := []struct {
		name        string
		input       []string
		expected    map[string]bool
		expectError bool
	}{
		{
			name:  "plus prefix",
			input: []string{"+ROX_FEATURE_1"},
			expected: map[string]bool{
				"ROX_FEATURE_1": true,
			},
		},
		{
			name:  "minus prefix",
			input: []string{"-ROX_FEATURE_1"},
			expected: map[string]bool{
				"ROX_FEATURE_1": false,
			},
		},
		{
			name:  "explicit true",
			input: []string{"ROX_FEATURE_1=true"},
			expected: map[string]bool{
				"ROX_FEATURE_1": true,
			},
		},
		{
			name:  "explicit false",
			input: []string{"ROX_FEATURE_1=false"},
			expected: map[string]bool{
				"ROX_FEATURE_1": false,
			},
		},
		{
			name:  "no prefix defaults to true",
			input: []string{"ROX_FEATURE_1"},
			expected: map[string]bool{
				"ROX_FEATURE_1": true,
			},
		},
		{
			name:  "comma separated",
			input: []string{"+ROX_F1,-ROX_F2,ROX_F3=true"},
			expected: map[string]bool{
				"ROX_F1": true,
				"ROX_F2": false,
				"ROX_F3": true,
			},
		},
		{
			name:  "multiple array elements",
			input: []string{"+ROX_F1", "-ROX_F2", "ROX_F3=true"},
			expected: map[string]bool{
				"ROX_F1": true,
				"ROX_F2": false,
				"ROX_F3": true,
			},
		},
		{
			name:  "mixed formats with spaces",
			input: []string{" +ROX_F1 , -ROX_F2 ", "ROX_F3=true"},
			expected: map[string]bool{
				"ROX_F1": true,
				"ROX_F2": false,
				"ROX_F3": true,
			},
		},
		{
			name:  "override previous value",
			input: []string{"ROX_F1=true", "ROX_F1=false"},
			expected: map[string]bool{
				"ROX_F1": false,
			},
		},
		{
			name:        "invalid prefix",
			input:       []string{"INVALID_FLAG=true"},
			expectError: true,
		},
		{
			name:        "invalid prefix with plus",
			input:       []string{"+INVALID_FLAG"},
			expectError: true,
		},
		{
			name:        "invalid boolean value",
			input:       []string{"ROX_F1=notabool"},
			expectError: true,
		},
		{
			name:        "empty flag name after ROX_",
			input:       []string{"+ROX_"},
			expectError: true,
		},
		{
			name:        "empty flag name with equals",
			input:       []string{"ROX_=true"},
			expectError: true,
		},
		{
			name:     "empty string",
			input:    []string{""},
			expected: map[string]bool{},
		},
		{
			name:  "empty elements in comma list",
			input: []string{"+ROX_F1,,,-ROX_F2"},
			expected: map[string]bool{
				"ROX_F1": true,
				"ROX_F2": false,
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result, err := parseFeatureFlags(tt.input)
			if tt.expectError {
				if err == nil {
					t.Errorf("expected error but got nil")
				}
			} else {
				if err != nil {
					t.Fatalf("unexpected error: %v", err)
				}
				if !reflect.DeepEqual(result, tt.expected) {
					t.Errorf("got %v, want %v", result, tt.expected)
				}
			}
		})
	}
}

func TestMergeEnvVars(t *testing.T) {
	tests := []struct {
		name     string
		base     []interface{}
		overlay  []interface{}
		expected map[string]string
	}{
		{
			name: "overlapping names - overlay wins",
			base: []interface{}{
				map[string]interface{}{"name": "ROX_FOO", "value": "old"},
				map[string]interface{}{"name": "ROX_BAR", "value": "keep"},
			},
			overlay: []interface{}{
				map[string]interface{}{"name": "ROX_FOO", "value": "new"},
			},
			expected: map[string]string{
				"ROX_FOO": "new",
				"ROX_BAR": "keep",
			},
		},
		{
			name: "no overlapping names",
			base: []interface{}{
				map[string]interface{}{"name": "ROX_FOO", "value": "foo"},
			},
			overlay: []interface{}{
				map[string]interface{}{"name": "ROX_BAR", "value": "bar"},
			},
			expected: map[string]string{
				"ROX_FOO": "foo",
				"ROX_BAR": "bar",
			},
		},
		{
			name: "empty base",
			base: []interface{}{},
			overlay: []interface{}{
				map[string]interface{}{"name": "ROX_FOO", "value": "foo"},
			},
			expected: map[string]string{
				"ROX_FOO": "foo",
			},
		},
		{
			name: "empty overlay",
			base: []interface{}{
				map[string]interface{}{"name": "ROX_FOO", "value": "foo"},
			},
			overlay: []interface{}{},
			expected: map[string]string{
				"ROX_FOO": "foo",
			},
		},
		{
			name: "malformed envVar without name field",
			base: []interface{}{
				map[string]interface{}{"name": "ROX_FOO", "value": "foo"},
			},
			overlay: []interface{}{
				map[string]interface{}{"value": "no-name"}, // missing "name" field
				map[string]interface{}{"name": "ROX_BAR", "value": "bar"},
			},
			expected: map[string]string{
				"ROX_FOO": "foo",
				"ROX_BAR": "bar",
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := mergeEnvVars(tt.base, tt.overlay)

			if len(result) != len(tt.expected) {
				t.Fatalf("expected %d env vars, got %d", len(tt.expected), len(result))
			}

			resultMap := make(map[string]string)
			for _, item := range result {
				envVar := item.(map[string]interface{})
				if name, ok := envVar["name"].(string); ok {
					resultMap[name] = envVar["value"].(string)
				}
			}

			if !reflect.DeepEqual(resultMap, tt.expected) {
				t.Errorf("got %v, want %v", resultMap, tt.expected)
			}
		})
	}
}

func TestMergeWithEnvVarSupport(t *testing.T) {
	tests := []struct {
		name             string
		base             map[string]interface{}
		overlay          map[string]interface{}
		expectedEnvVars  map[string]string
		checkOtherFields bool
	}{
		{
			name: "both have envVars with overlap",
			base: map[string]interface{}{
				"spec": map[string]interface{}{
					"customize": map[string]interface{}{
						"envVars": []interface{}{
							map[string]interface{}{"name": "ROX_FOO", "value": "old"},
							map[string]interface{}{"name": "ROX_KEEP", "value": "me"},
						},
					},
				},
			},
			overlay: map[string]interface{}{
				"spec": map[string]interface{}{
					"customize": map[string]interface{}{
						"envVars": []interface{}{
							map[string]interface{}{"name": "ROX_FOO", "value": "new"},
						},
					},
				},
			},
			expectedEnvVars: map[string]string{
				"ROX_FOO":  "new",
				"ROX_KEEP": "me",
			},
		},
		{
			name: "base has envVars, overlay has none",
			base: map[string]interface{}{
				"spec": map[string]interface{}{
					"customize": map[string]interface{}{
						"envVars": []interface{}{
							map[string]interface{}{"name": "ROX_FOO", "value": "foo"},
						},
					},
				},
			},
			overlay: map[string]interface{}{
				"spec": map[string]interface{}{
					"other": "field",
				},
			},
			expectedEnvVars: map[string]string{
				"ROX_FOO": "foo",
			},
		},
		{
			name: "overlay has envVars, base has none",
			base: map[string]interface{}{
				"spec": map[string]interface{}{
					"other": "field",
				},
			},
			overlay: map[string]interface{}{
				"spec": map[string]interface{}{
					"customize": map[string]interface{}{
						"envVars": []interface{}{
							map[string]interface{}{"name": "ROX_FOO", "value": "foo"},
						},
					},
				},
			},
			expectedEnvVars: map[string]string{
				"ROX_FOO": "foo",
			},
		},
		{
			name: "neither has envVars",
			base: map[string]interface{}{
				"spec": map[string]interface{}{
					"other": "field",
				},
			},
			overlay: map[string]interface{}{
				"spec": map[string]interface{}{
					"another": "field",
				},
			},
			expectedEnvVars:  nil,
			checkOtherFields: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := mergeWithEnvVarSupport(tt.base, tt.overlay)

			spec, ok := result["spec"].(map[string]interface{})
			if !ok {
				t.Fatal("spec should be a map")
			}

			if tt.expectedEnvVars != nil {
				customize, ok := spec["customize"].(map[string]interface{})
				if !ok {
					t.Fatal("customize should be a map")
				}

				envVars, ok := customize["envVars"].([]interface{})
				if !ok {
					t.Fatal("envVars should be a slice")
				}

				if len(envVars) != len(tt.expectedEnvVars) {
					t.Fatalf("expected %d env vars, got %d", len(tt.expectedEnvVars), len(envVars))
				}

				resultMap := make(map[string]string)
				for _, item := range envVars {
					envVar := item.(map[string]interface{})
					resultMap[envVar["name"].(string)] = envVar["value"].(string)
				}

				if !reflect.DeepEqual(resultMap, tt.expectedEnvVars) {
					t.Errorf("got %v, want %v", resultMap, tt.expectedEnvVars)
				}
			}

			if tt.checkOtherFields {
				if spec["other"] != "field" {
					t.Errorf("expected base field to be preserved")
				}
				if spec["another"] != "field" {
					t.Errorf("expected overlay field to be added")
				}
			}
		})
	}
}

func TestFeatureFlagsConversion(t *testing.T) {
	tests := []struct {
		name         string
		input        map[string]bool
		expectNil    bool
		expectedVars map[string]string
	}{
		{
			name: "to env vars",
			input: map[string]bool{
				"ROX_F1": true,
				"ROX_F2": false,
			},
			expectedVars: map[string]string{
				"ROX_F1": "true",
				"ROX_F2": "false",
			},
		},
		{
			name: "single flag to overrides",
			input: map[string]bool{
				"ROX_F1": true,
			},
			expectedVars: map[string]string{
				"ROX_F1": "true",
			},
		},
		{
			name:      "empty flags",
			input:     map[string]bool{},
			expectNil: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := featureFlagsToOverrides(tt.input)

			if tt.expectNil {
				if result != nil {
					t.Errorf("expected nil for empty flags, got %v", result)
				}
				return
			}

			if result == nil {
				t.Fatal("result should not be nil")
			}

			spec, ok := result["spec"].(map[string]interface{})
			if !ok {
				t.Fatal("spec should be a map")
			}

			customize, ok := spec["customize"].(map[string]interface{})
			if !ok {
				t.Fatal("customize should be a map")
			}

			envVars, ok := customize["envVars"].([]interface{})
			if !ok {
				t.Fatal("envVars should be a slice of interface{}")
			}

			if len(envVars) != len(tt.expectedVars) {
				t.Fatalf("expected %d env var(s), got %d", len(tt.expectedVars), len(envVars))
			}

			resultMap := make(map[string]string)
			for _, item := range envVars {
				envVar, ok := item.(map[string]interface{})
				if !ok {
					t.Fatal("envVar should be a map")
				}
				resultMap[envVar["name"].(string)] = envVar["value"].(string)
			}

			if !reflect.DeepEqual(resultMap, tt.expectedVars) {
				t.Errorf("got %v, want %v", resultMap, tt.expectedVars)
			}
		})
	}
}
