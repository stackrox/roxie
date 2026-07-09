package paths

import (
	"fmt"
	"os"
	"path/filepath"
)

const appName = "roxie"

const envUserConfigPath = "ROXIE_USER_CONFIG_PATH"

func UserConfigPath(validateUserConfig bool) (string, error) {
	if overwrittenPath := os.Getenv(envUserConfigPath); overwrittenPath != "" {
		if validateUserConfig {
			if err := fileIsReadable(overwrittenPath); err != nil {
				return "", fmt.Errorf("checking if user config file %q from environment variable %s is readable: %w",
					overwrittenPath, envUserConfigPath, err)
			}
		}
		return overwrittenPath, nil
	}
	return getDefaultUserConfigPath()
}

func fileIsReadable(path string) error {
	fd, err := os.Open(path)
	if err != nil {
		return err
	}
	_ = fd.Close()
	return nil
}

func getDefaultUserConfigPath() (string, error) {
	dir, err := configDir()
	if err != nil {
		return "", fmt.Errorf("retrieving user config path: %w", err)
	}
	return filepath.Join(dir, "config.yaml"), nil
}

func UserConfigPathString() string {
	path, err := UserConfigPath(false)
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
