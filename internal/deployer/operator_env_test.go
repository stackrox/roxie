package deployer

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestParseOperatorEnvVar(t *testing.T) {
	tests := []struct {
		name        string
		input       string
		expectedKey string
		expectedVal string
		expectError bool
	}{
		{
			name:        "simple KEY=VALUE",
			input:       "RELATED_IMAGE_MAIN=quay.io/rhacs-eng/main:4.7.0",
			expectedKey: "RELATED_IMAGE_MAIN",
			expectedVal: "quay.io/rhacs-eng/main:4.7.0",
		},
		{
			name:        "value containing equals sign",
			input:       "MY_VAR=a=b=c",
			expectedKey: "MY_VAR",
			expectedVal: "a=b=c",
		},
		{
			name:        "empty value",
			input:       "MY_VAR=",
			expectedKey: "MY_VAR",
			expectedVal: "",
		},
		{
			name:        "value with spaces",
			input:       "MY_VAR= hello world ",
			expectedKey: "MY_VAR",
			expectedVal: " hello world ",
		},
		{
			name:        "value with commas",
			input:       "MY_VAR=a,b,c",
			expectedKey: "MY_VAR",
			expectedVal: "a,b,c",
		},
		{
			name:        "missing equals sign",
			input:       "NO_VALUE",
			expectError: true,
		},
		{
			name:        "empty key",
			input:       "=value",
			expectError: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			key, value, err := ParseOperatorEnvVar(tt.input)
			if tt.expectError {
				require.Error(t, err)
				return
			}
			require.NoError(t, err)
			assert.Equal(t, tt.expectedKey, key)
			assert.Equal(t, tt.expectedVal, value)
		})
	}
}


func TestEnvVarsToSortedList(t *testing.T) {
	input := map[string]string{
		"ZZZ": "z",
		"AAA": "a",
		"MMM": "m",
	}
	result := envVarsToSortedList(input)

	require.Len(t, result, 3)

	expectedOrder := []string{"AAA", "MMM", "ZZZ"}
	for i, item := range result {
		name := item.(map[string]interface{})["name"].(string)
		assert.Equal(t, expectedOrder[i], name, "index %d", i)
	}
}
