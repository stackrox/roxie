package paths

import (
	"fmt"
	"os"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

var (
	defaultUserConfigPath = func() string {
		// Ok to panic here. The tests would fail one way or another anyway.
		path, err := getDefaultUserConfigPath()
		if err != nil {
			panic(fmt.Sprintf("failed to obtain default user config path: %v", err))
		}
		return path
	}()
)

func TestUserConfigPath(t *testing.T) {
	existentUserConfig := createTempFile(t, "cfg.yaml")
	tests := []struct {
		name               string
		envValue           string
		setEnv             bool
		expected           string
		validateUserConfig bool
		expectErr          bool
	}{
		{
			name:               "returns env var value when set and file exists with validation",
			envValue:           existentUserConfig,
			setEnv:             true,
			expected:           existentUserConfig,
			validateUserConfig: true,
		},
		{
			name:               "validation returns error when env var value set to non-existent file",
			envValue:           "/non-existent/roxie.yaml",
			setEnv:             true,
			validateUserConfig: true,
			expectErr:          true,
		},
		{
			name:               "returns env var value when set to non-existent file with validation off",
			envValue:           "/non-existent/roxie.yaml",
			setEnv:             true,
			expected:           "/non-existent/roxie.yaml",
			validateUserConfig: false,
		},
		{
			name:               "returns default path when env var is not set with validation",
			setEnv:             false,
			expected:           defaultUserConfigPath,
			validateUserConfig: true,
		},
		{
			name:               "returns default path when env var is set but empty",
			envValue:           "",
			setEnv:             true,
			expected:           defaultUserConfigPath,
			validateUserConfig: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			cleanEnvironment(t)

			if tt.setEnv {
				t.Setenv(envUserConfigPath, tt.envValue)
			}
			got, err := UserConfigPath(tt.validateUserConfig)
			if tt.expectErr {
				assert.Error(t, err, "expected UserConfigPath() to return error")
				return
			}
			require.NoError(t, err)
			assert.Equal(t, tt.expected, got)
		})
	}
}

func TestUserConfigPathString(t *testing.T) {
	existentUserConfig := createTempFile(t, "cfg.yaml")
	tests := []struct {
		name     string
		envValue string
		setEnv   bool
		expected string
	}{
		{
			name:     "returns env var value when set and file does not exist",
			envValue: "/nonexistent/path.yaml",
			setEnv:   true,
			expected: "/nonexistent/path.yaml",
		},
		{
			name:     "returns env var value when set and file exists",
			envValue: existentUserConfig,
			setEnv:   true,
			expected: existentUserConfig,
		},
		{
			name:     "returns default path when env var is not set",
			setEnv:   false,
			expected: defaultUserConfigPath,
		},
		{
			name:     "returns default path when env var set but empty",
			envValue: "",
			setEnv:   true,
			expected: defaultUserConfigPath,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			cleanEnvironment(t)

			if tt.setEnv {
				t.Setenv(envUserConfigPath, tt.envValue)
			}
			assert.Equal(t, tt.expected, UserConfigPathString())
		})
	}
}

func createTempFile(t *testing.T, name string) string {
	tmpdir := t.TempDir()
	path := tmpdir + "/" + name
	err := os.WriteFile(path, nil, 0o600)
	require.NoErrorf(t, err, "failed to create temporary file %q", path)
	return path
}

func cleanEnvironment(t *testing.T) {
	// Clean environment for the duration of this test case.
	// The os.Unsetenv() in combination with the cleanup handler installed by t.Setenv()
	// guarantees that for the duration of the test, there is *no* variable set in the
	// environment at all, not even an empty one.
	t.Setenv(envUserConfigPath, "")
	os.Unsetenv(envUserConfigPath)
}
