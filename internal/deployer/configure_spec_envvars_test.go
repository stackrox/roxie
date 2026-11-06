package deployer

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
)

// TestConfigureSpec_EnvVarDeduplication tests that ConfigureSpec correctly merges
// feature flags with pre-existing env vars in the spec, producing no duplicates
// and preserving non-overlapping vars. Covers both Central and SecuredCluster.
func TestConfigureSpec_EnvVarDeduplication(t *testing.T) {
	tests := []struct {
		name            string
		existingEnvVars []any
		featureFlags    map[string]bool
		expectedEnvVars map[string]string
	}{
		{
			name: "config envVars preserved when features add non-overlapping flags",
			existingEnvVars: envVarSlice(
				map[string]string{"name": "MY_CUSTOM_VAR", "value": "hello"},
			),
			featureFlags: map[string]bool{
				"ROX_NEW": true,
			},
			expectedEnvVars: map[string]string{
				"MY_CUSTOM_VAR": "hello",
				"ROX_NEW":       "true",
			},
		},
		{
			name: "features override overlapping config envVar without duplicates",
			existingEnvVars: envVarSlice(
				map[string]string{"name": "ROX_FOO", "value": "true"},
			),
			featureFlags: map[string]bool{
				"ROX_FOO": false,
			},
			expectedEnvVars: map[string]string{
				"ROX_FOO": "false",
			},
		},
		{
			name: "mixed envVars with partial overlap",
			existingEnvVars: envVarSlice(
				map[string]string{"name": "MY_CUSTOM_VAR", "value": "hello"},
				map[string]string{"name": "ROX_FOO", "value": "true"},
			),
			featureFlags: map[string]bool{
				"ROX_FOO": false,
			},
			expectedEnvVars: map[string]string{
				"MY_CUSTOM_VAR": "hello",
				"ROX_FOO":       "false",
			},
		},
		{
			name:            "no existing envVars, features added cleanly",
			existingEnvVars: nil,
			featureFlags: map[string]bool{
				"ROX_A": true,
				"ROX_B": false,
			},
			expectedEnvVars: map[string]string{
				"ROX_A": "true",
				"ROX_B": "false",
			},
		},
	}

	for _, tt := range tests {
		roxie := NewRoxieConfig()
		roxie.FeatureFlags = tt.featureFlags

		t.Run("central/"+tt.name, func(t *testing.T) {
			cfg := NewCentralConfig()
			if tt.existingEnvVars != nil {
				cfg.Spec["customize"] = map[string]any{"envVars": tt.existingEnvVars}
			}

			require.NoError(t, cfg.ConfigureSpec(&roxie))

			got := extractEnvVars(t, cfg.Spec)
			assert.Equal(t, tt.expectedEnvVars, got)
		})

		t.Run("securedCluster/"+tt.name, func(t *testing.T) {
			cfg := SecuredClusterConfig{Namespace: "stackrox", Spec: make(map[string]any)}
			if tt.existingEnvVars != nil {
				cfg.Spec["customize"] = map[string]any{"envVars": tt.existingEnvVars}
			}

			require.NoError(t, cfg.ConfigureSpec(&roxie, &CentralConfig{Namespace: "acs-central"}))

			got := extractEnvVars(t, cfg.Spec)
			assert.Equal(t, tt.expectedEnvVars, got)
		})
	}
}

func envVarSlice(vars ...map[string]string) []any {
	result := make([]any, len(vars))
	for i, v := range vars {
		result[i] = map[string]any{"name": v["name"], "value": v["value"]}
	}
	return result
}

func extractEnvVars(t *testing.T, spec map[string]any) map[string]string {
	t.Helper()
	envVars, found, err := unstructured.NestedSlice(spec, "customize", "envVars")
	require.NoError(t, err)
	require.True(t, found, "customize.envVars not found in spec")

	result := make(map[string]string)
	for _, item := range envVars {
		ev := item.(map[string]any)
		name := ev["name"].(string)
		require.NotContains(t, result, name, "duplicate envVar %q in slice", name)
		result[name] = ev["value"].(string)
	}
	return result
}
