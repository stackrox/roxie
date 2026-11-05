package deployer

import (
	"reflect"
	"testing"

	"github.com/stackrox/roxie/internal/envvars"
	"github.com/stackrox/roxie/internal/helpers"
	"github.com/stretchr/testify/require"
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
			result, err := ParseFeatureFlags(tt.input)
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

func TestDeepMergeWithDeduplicateEnvVars(t *testing.T) {
	spec := map[string]any{
		"customize": map[string]any{
			"envVars": []any{
				map[string]any{"name": "MY_VAR", "value": "keep"},
				map[string]any{"name": "ROX_FOO", "value": "old"},
			},
		},
	}
	overlay := featureFlagsToOverrides(map[string]bool{"ROX_FOO": false, "ROX_NEW": true})

	err := helpers.DeepMerge(spec, overlay)
	require.NoError(t, err)

	err = envvars.DeduplicateInUnstructured(spec, "customize", "envVars")
	require.NoError(t, err)

	customize := spec["customize"].(map[string]any)
	envVars := customize["envVars"].([]any)

	resultMap := make(map[string]string)
	for _, item := range envVars {
		envVar := item.(map[string]any)
		resultMap[envVar["name"].(string)] = envVar["value"].(string)
	}

	require.Equal(t, map[string]string{
		"MY_VAR":  "keep",
		"ROX_FOO": "false",
		"ROX_NEW": "true",
	}, resultMap)
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

			customize, ok := result["customize"].(map[string]interface{})
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
