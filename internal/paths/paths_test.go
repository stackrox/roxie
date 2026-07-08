package paths

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestUserConfigPath(t *testing.T) {
	tests := []struct {
		name     string
		envValue string
		setEnv   bool
	}{
		{
			name:     "returns env var value when set",
			envValue: "/custom/path/config.yaml",
			setEnv:   true,
		},
		{
			name:   "returns default path when env var is not set",
			setEnv: false,
		},
		{
			name:     "returns default path when env var is empty",
			envValue: "",
			setEnv:   true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if tt.setEnv {
				t.Setenv(envUserConfigPath, tt.envValue)
			}

			got, err := UserConfigPath()
			require.NoError(t, err)

			if tt.setEnv && tt.envValue != "" {
				assert.Equal(t, tt.envValue, got)
			} else {
				assert.Contains(t, got, "roxie/config.yaml")
			}
		})
	}
}

func TestUserConfigPathString(t *testing.T) {
	t.Run("returns env var value when set", func(t *testing.T) {
		t.Setenv(envUserConfigPath, "/custom/path/config.yaml")
		assert.Equal(t, "/custom/path/config.yaml", UserConfigPathString())
	})

	t.Run("returns default path when env var is not set", func(t *testing.T) {
		assert.Contains(t, UserConfigPathString(), "roxie/config.yaml")
	})
}
