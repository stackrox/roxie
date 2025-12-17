package deployer

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	corev1 "k8s.io/api/core/v1"
)

func TestFeatureFlags_parseAndSetOne(t *testing.T) {
	tests := []struct {
		name          string
		setting       string
		expectedKey   string
		expectedVal   bool
		expectedError bool
	}{
		{
			name:        "explicit true with equals",
			setting:     "ROX_FEATURE_1=true",
			expectedKey: "ROX_FEATURE_1",
			expectedVal: true,
		},
		{
			name:        "explicit false with equals",
			setting:     "ROX_FEATURE_2=false",
			expectedKey: "ROX_FEATURE_2",
			expectedVal: false,
		},
		{
			name:        "explicit 1 (true)",
			setting:     "ROX_FEATURE_3=1",
			expectedKey: "ROX_FEATURE_3",
			expectedVal: true,
		},
		{
			name:        "explicit 0 (false)",
			setting:     "ROX_FEATURE_4=0",
			expectedKey: "ROX_FEATURE_4",
			expectedVal: false,
		},
		{
			name:        "plus prefix (enabled)",
			setting:     "+ROX_FEATURE_5",
			expectedKey: "ROX_FEATURE_5",
			expectedVal: true,
		},
		{
			name:        "minus prefix (disabled)",
			setting:     "-ROX_FEATURE_6",
			expectedKey: "ROX_FEATURE_6",
			expectedVal: false,
		},
		{
			name:        "no prefix (defaults to true)",
			setting:     "ROX_FEATURE_7",
			expectedKey: "ROX_FEATURE_7",
			expectedVal: true,
		},
		{
			name:          "invalid value",
			setting:       "ROX_FEATURE_8=invalid",
			expectedError: true,
		},
		{
			name:          "invalid prefix",
			setting:       "FEATURE_9=true",
			expectedError: true,
		},
		{
			name:          "invalid prefix with plus",
			setting:       "+FEATURE_10",
			expectedError: true,
		},
		{
			name:          "invalid prefix with minus",
			setting:       "-FEATURE_11",
			expectedError: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			ff := NewFeatureFlags()
			err := ff.parseAndSetOne(tt.setting)
			if tt.expectedError {
				assert.Error(t, err)
			} else {
				require.NoError(t, err)
				assert.Equal(t, tt.expectedVal, ff.m[tt.expectedKey])
			}
		})
	}
}

func TestFeatureFlags_parseAndSetFromString(t *testing.T) {
	tests := []struct {
		name        string
		input       string
		expected    map[string]bool
		expectError bool
	}{
		{
			name:  "single flag",
			input: "ROX_FEATURE_1=true",
			expected: map[string]bool{
				"ROX_FEATURE_1": true,
			},
			expectError: false,
		},
		{
			name:  "multiple flags comma-separated",
			input: "ROX_FEATURE_1=true,ROX_FEATURE_2=false,+ROX_FEATURE_3,-ROX_FEATURE_4",
			expected: map[string]bool{
				"ROX_FEATURE_1": true,
				"ROX_FEATURE_2": false,
				"ROX_FEATURE_3": true,
				"ROX_FEATURE_4": false,
			},
			expectError: false,
		},
		{
			name:  "flags with spaces",
			input: "ROX_FEATURE_1=true, ROX_FEATURE_2=false , +ROX_FEATURE_3 , -ROX_FEATURE_4",
			expected: map[string]bool{
				"ROX_FEATURE_1": true,
				"ROX_FEATURE_2": false,
				"ROX_FEATURE_3": true,
				"ROX_FEATURE_4": false,
			},
			expectError: false,
		},
		{
			name:  "mixed formats",
			input: "ROX_FEATURE_1,+ROX_FEATURE_2,-ROX_FEATURE_3,ROX_FEATURE_4=1,ROX_FEATURE_5=0",
			expected: map[string]bool{
				"ROX_FEATURE_1": true,
				"ROX_FEATURE_2": true,
				"ROX_FEATURE_3": false,
				"ROX_FEATURE_4": true,
				"ROX_FEATURE_5": false,
			},
			expectError: false,
		},
		{
			name:        "invalid flag in list",
			input:       "ROX_FEATURE_1=true,INVALID_FLAG=true",
			expectError: true,
		},
		{
			name:        "invalid value in list",
			input:       "ROX_FEATURE_1=true,ROX_FEATURE_2=invalid",
			expectError: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			ff := NewFeatureFlags()
			err := ff.parseAndSetFromString(tt.input)
			if tt.expectError {
				assert.Error(t, err)
			} else {
				require.NoError(t, err)
				assert.Equal(t, tt.expected, ff.m)
			}
		})
	}
}

func TestFeatureFlags_ParseAndSetFromSlice(t *testing.T) {
	tests := []struct {
		name        string
		input       []string
		expected    map[string]bool
		expectError bool
	}{
		{
			name:  "single string with single flag",
			input: []string{"ROX_FEATURE_1=true"},
			expected: map[string]bool{
				"ROX_FEATURE_1": true,
			},
		},
		{
			name:  "single string with multiple flags",
			input: []string{"ROX_FEATURE_1=true,ROX_FEATURE_2=false"},
			expected: map[string]bool{
				"ROX_FEATURE_1": true,
				"ROX_FEATURE_2": false,
			},
		},
		{
			name: "multiple strings",
			input: []string{
				"ROX_FEATURE_1=true,ROX_FEATURE_2=false",
				"+ROX_FEATURE_3,-ROX_FEATURE_4",
			},
			expected: map[string]bool{
				"ROX_FEATURE_1": true,
				"ROX_FEATURE_2": false,
				"ROX_FEATURE_3": true,
				"ROX_FEATURE_4": false,
			},
		},
		{
			name: "override previous value",
			input: []string{
				"ROX_FEATURE_1=true",
				"ROX_FEATURE_1=false",
			},
			expected: map[string]bool{
				"ROX_FEATURE_1": false,
			},
		},
		{
			name:     "empty slice",
			input:    []string{},
			expected: map[string]bool{},
		},
		{
			name: "error in first string",
			input: []string{
				"INVALID_FLAG=true",
				"ROX_FEATURE_1=true",
			},
			expectError: true,
		},
		{
			name: "error in second string",
			input: []string{
				"ROX_FEATURE_1=true",
				"INVALID_FLAG=true",
			},
			expectError: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			ff := NewFeatureFlags()
			err := ff.ParseAndSetFromSlice(tt.input)
			if tt.expectError {
				assert.Error(t, err)
			} else {
				require.NoError(t, err)
				assert.Equal(t, tt.expected, ff.m)
			}
		})
	}
}

func TestFeatureFlags_Integration(t *testing.T) {
	t.Run("parse from slice and convert to env vars", func(t *testing.T) {
		ff := NewFeatureFlags()
		err := ff.ParseAndSetFromSlice([]string{
			"ROX_FEATURE_1=true,ROX_FEATURE_2=false",
			"+ROX_FEATURE_3,-ROX_FEATURE_4",
			"ROX_FEATURE_5=1",
		})
		require.NoError(t, err)

		envVars := ff.ToEnvVars()
		expected := []corev1.EnvVar{
			{Name: "ROX_FEATURE_1", Value: "true"},
			{Name: "ROX_FEATURE_2", Value: "false"},
			{Name: "ROX_FEATURE_3", Value: "true"},
			{Name: "ROX_FEATURE_4", Value: "false"},
			{Name: "ROX_FEATURE_5", Value: "true"},
		}
		assert.ElementsMatch(t, expected, envVars)
	})
}
