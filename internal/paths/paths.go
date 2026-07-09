package paths

import (
	"fmt"
	"os"
	"path/filepath"
)

const appName = "roxie"

const envUserConfigPath = "ROXIE_USER_CONFIG_PATH"

func UserConfigPath() (string, error) {
	if overwrittenPath := os.Getenv(envUserConfigPath); overwrittenPath != "" {
		if _, err := os.ReadFile(overwrittenPath); err != nil {
			return "", fmt.Errorf("config file %q specified in environment variable %s cannot be read: %w",
				overwrittenPath, envUserConfigPath, err)
		}
		return overwrittenPath, nil
	}
	dir, err := configDir()
	if err != nil {
		return "", fmt.Errorf("retrieving user config path: %w", err)
	}
	return filepath.Join(dir, "config.yaml"), nil
}

func UserConfigPathString() string {
	path, err := UserConfigPath()
	if err != nil {
		return "(UNAVAILABLE)"
	}
	return path
}

func configDir() (string, error) {
	dir, err := os.UserConfigDir()
	if err != nil {
		return "", err
	}
	return filepath.Join(dir, appName), nil
}

// CacheDir returns the cache directory to be used by roxie.
// This directory might not yet exist, it is the responsibility of the caller
// to make sure this directory exists before writing to it.
func CacheDir() (string, error) {
	dir, err := os.UserCacheDir()
	if err != nil {
		return "", fmt.Errorf("retrieving user cache path: %w", err)
	}
	return filepath.Join(dir, appName), nil
}
