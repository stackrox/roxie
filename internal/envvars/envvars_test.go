package envvars

import (
	"testing"

	"github.com/stretchr/testify/require"
)

func TestDeduplicateEnvVars(t *testing.T) {
	tests := []struct {
		name            string
		spec            map[string]interface{}
		expectedEnvVars map[string]string
	}{
		{
			name: "duplicates are resolved with last-one-wins",
			spec: map[string]interface{}{
				"customize": map[string]interface{}{
					"envVars": []interface{}{
						map[string]interface{}{"name": "ROX_FOO", "value": "old"},
						map[string]interface{}{"name": "ROX_KEEP", "value": "me"},
						map[string]interface{}{"name": "ROX_FOO", "value": "new"},
					},
				},
			},
			expectedEnvVars: map[string]string{
				"ROX_FOO":  "new",
				"ROX_KEEP": "me",
			},
		},
		{
			name: "no duplicates is a no-op",
			spec: map[string]interface{}{
				"customize": map[string]interface{}{
					"envVars": []interface{}{
						map[string]interface{}{"name": "ROX_A", "value": "a"},
						map[string]interface{}{"name": "ROX_B", "value": "b"},
					},
				},
			},
			expectedEnvVars: map[string]string{
				"ROX_A": "a",
				"ROX_B": "b",
			},
		},
		{
			name:            "no envVars is a no-op",
			spec:            map[string]interface{}{"other": "field"},
			expectedEnvVars: nil,
		},
		{
			name:            "empty spec is a no-op",
			spec:            map[string]interface{}{},
			expectedEnvVars: nil,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := DeduplicateInUnstructured(tt.spec, "customize", "envVars")
			require.NoError(t, err)

			if tt.expectedEnvVars == nil {
				return
			}

			customize := tt.spec["customize"].(map[string]interface{})
			envVars := customize["envVars"].([]interface{})

			require.Len(t, envVars, len(tt.expectedEnvVars))

			resultMap := make(map[string]string)
			for _, item := range envVars {
				envVar := item.(map[string]interface{})
				resultMap[envVar["name"].(string)] = envVar["value"].(string)
			}

			require.Equal(t, tt.expectedEnvVars, resultMap)
		})
	}
}

func TestDeduplicateEnvVars_PreservesValueFrom(t *testing.T) {
	valueFrom := map[string]any{
		"secretKeyRef": map[string]any{
			"name": "my-secret",
			"key":  "password",
		},
	}
	spec := map[string]any{
		"customize": map[string]any{
			"envVars": []any{
				map[string]any{"name": "SECRET_VAR", "valueFrom": valueFrom},
				map[string]any{"name": "PLAIN_VAR", "value": "hello"},
			},
		},
	}

	require.NoError(t, DeduplicateInUnstructured(spec, "customize", "envVars"))

	customize := spec["customize"].(map[string]any)
	envVars := customize["envVars"].([]any)
	require.Len(t, envVars, 2)

	secretVar := envVars[0].(map[string]any)
	require.Equal(t, "SECRET_VAR", secretVar["name"])
	require.Equal(t, valueFrom, secretVar["valueFrom"])
	_, hasValue := secretVar["value"]
	require.False(t, hasValue, "valueFrom entry should not have a value field")
}

func TestDeduplicateEnvVars_PreservesDependencyOrder(t *testing.T) {
	spec := map[string]any{
		"customize": map[string]any{
			"envVars": []any{
				map[string]any{"name": "BASE_URL", "value": "https://example.com"},
				map[string]any{"name": "API_URL", "value": "$(BASE_URL)/api"},
				map[string]any{"name": "BASE_URL", "value": "https://override.com"},
			},
		},
	}

	require.NoError(t, DeduplicateInUnstructured(spec, "customize", "envVars"))

	customize := spec["customize"].(map[string]any)
	envVars := customize["envVars"].([]any)
	require.Len(t, envVars, 2)

	require.Equal(t, "BASE_URL", envVars[0].(map[string]any)["name"])
	require.Equal(t, "https://override.com", envVars[0].(map[string]any)["value"])
	require.Equal(t, "API_URL", envVars[1].(map[string]any)["name"])
	require.Equal(t, "$(BASE_URL)/api", envVars[1].(map[string]any)["value"])
}

func TestFindUnresolvedRefs(t *testing.T) {
	tests := []struct {
		name     string
		envVars  []any
		expected []UnresolvedRef
	}{
		{
			name: "no references",
			envVars: []any{
				map[string]any{"name": "A", "value": "hello"},
				map[string]any{"name": "B", "value": "world"},
			},
			expected: nil,
		},
		{
			name: "resolved forward reference",
			envVars: []any{
				map[string]any{"name": "BASE", "value": "https://example.com"},
				map[string]any{"name": "URL", "value": "$(BASE)/api"},
			},
			expected: nil,
		},
		{
			name: "unresolved: referent defined later",
			envVars: []any{
				map[string]any{"name": "URL", "value": "$(BASE)/api"},
				map[string]any{"name": "BASE", "value": "https://example.com"},
			},
			expected: []UnresolvedRef{
				{Name: "URL", RefName: "BASE"},
			},
		},
		{
			name: "unresolved: referent not defined",
			envVars: []any{
				map[string]any{"name": "URL", "value": "$(MISSING)/api"},
			},
			expected: []UnresolvedRef{
				{Name: "URL", RefName: "MISSING"},
			},
		},
		{
			name: "multiple references in one value",
			envVars: []any{
				map[string]any{"name": "FULL", "value": "$(PROTO)://$(HOST)"},
				map[string]any{"name": "PROTO", "value": "https"},
			},
			expected: []UnresolvedRef{
				{Name: "FULL", RefName: "PROTO"},
				{Name: "FULL", RefName: "HOST"},
			},
		},
		{
			name: "escaped $$(VAR) is not a reference",
			envVars: []any{
				map[string]any{"name": "LITERAL", "value": "$$(NOT_A_REF)"},
			},
			expected: nil,
		},
		{
			name: "$$$(VAR) is literal $ followed by a real reference",
			envVars: []any{
				map[string]any{"name": "BASE", "value": "x"},
				map[string]any{"name": "COMBO", "value": "$$$(BASE)"},
			},
			expected: nil,
		},
		{
			name: "$$$(VAR) with undefined referent is flagged",
			envVars: []any{
				map[string]any{"name": "COMBO", "value": "$$$(UNDEF)"},
			},
			expected: []UnresolvedRef{
				{Name: "COMBO", RefName: "UNDEF"},
			},
		},
		{
			name: "valueFrom entries are skipped",
			envVars: []any{
				map[string]any{"name": "SECRET", "valueFrom": map[string]any{"secretKeyRef": map[string]any{"name": "s", "key": "k"}}},
				map[string]any{"name": "PLAIN", "value": "$(SECRET)"},
			},
			expected: nil,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := FindUnresolvedRefs(tt.envVars)
			require.Equal(t, tt.expected, got)
		})
	}
}
