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

	secretVar := envVars[1].(map[string]any) // sorted: PLAIN_VAR, SECRET_VAR
	require.Equal(t, "SECRET_VAR", secretVar["name"])
	require.Equal(t, valueFrom, secretVar["valueFrom"])
	_, hasValue := secretVar["value"]
	require.False(t, hasValue, "valueFrom entry should not have a value field")
}
