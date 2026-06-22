package deployer

import (
	"reflect"
	"testing"
)

func TestParseOperatorEnvVars(t *testing.T) {
	tests := []struct {
		name        string
		input       []string
		expected    map[string]string
		expectError bool
	}{
		{
			name:     "single KEY=VALUE",
			input:    []string{"RELATED_IMAGE_MAIN=quay.io/rhacs-eng/main:4.7.0"},
			expected: map[string]string{"RELATED_IMAGE_MAIN": "quay.io/rhacs-eng/main:4.7.0"},
		},
		{
			name:  "comma-separated pairs",
			input: []string{"RELATED_IMAGE_MAIN=quay.io/main:4.7.0,RELATED_IMAGE_SCANNER=quay.io/scanner:4.7.0"},
			expected: map[string]string{
				"RELATED_IMAGE_MAIN":    "quay.io/main:4.7.0",
				"RELATED_IMAGE_SCANNER": "quay.io/scanner:4.7.0",
			},
		},
		{
			name:  "multiple array elements",
			input: []string{"FOO=bar", "BAZ=qux"},
			expected: map[string]string{
				"FOO": "bar",
				"BAZ": "qux",
			},
		},
		{
			name:     "value containing equals sign",
			input:    []string{"MY_VAR=a=b=c"},
			expected: map[string]string{"MY_VAR": "a=b=c"},
		},
		{
			name:     "empty value",
			input:    []string{"MY_VAR="},
			expected: map[string]string{"MY_VAR": ""},
		},
		{
			name:     "duplicate key last wins",
			input:    []string{"FOO=old", "FOO=new"},
			expected: map[string]string{"FOO": "new"},
		},
		{
			name:     "empty string input",
			input:    []string{""},
			expected: map[string]string{},
		},
		{
			name:     "empty elements in comma list",
			input:    []string{"FOO=bar,,,BAZ=qux"},
			expected: map[string]string{"FOO": "bar", "BAZ": "qux"},
		},
		{
			name:  "whitespace around pairs trimmed",
			input: []string{" FOO=bar , BAZ=qux "},
			expected: map[string]string{
				"FOO": "bar",
				"BAZ": "qux",
			},
		},
		{
			name:        "missing equals sign",
			input:       []string{"NO_VALUE"},
			expectError: true,
		},
		{
			name:        "empty key",
			input:       []string{"=value"},
			expectError: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result, err := ParseOperatorEnvVars(tt.input)
			if tt.expectError {
				if err == nil {
					t.Errorf("expected error but got nil")
				}
				return
			}
			if err != nil {
				t.Fatalf("unexpected error: %v", err)
			}
			if !reflect.DeepEqual(result, tt.expected) {
				t.Errorf("got %v, want %v", result, tt.expected)
			}
		})
	}
}

func TestOperatorEnvVarsToSortedList(t *testing.T) {
	input := map[string]string{
		"ZZZ": "z",
		"AAA": "a",
		"MMM": "m",
	}
	result := operatorEnvVarsToSortedList(input)

	if len(result) != 3 {
		t.Fatalf("expected 3 items, got %d", len(result))
	}

	expectedOrder := []string{"AAA", "MMM", "ZZZ"}
	for i, item := range result {
		name := item.(map[string]interface{})["name"].(string)
		if name != expectedOrder[i] {
			t.Errorf("index %d: got %q, want %q", i, name, expectedOrder[i])
		}
	}
}
