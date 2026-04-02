package config

import (
	"fmt"
	"os"
	"path/filepath"
	"runtime"
)

// ConfigDir returns the directory where ytr stores its configuration.
// Resolution order: YTR_CONFIG_DIR env var > platform default.
// On Windows, uses %APPDATA%\ytr. On Unix, uses XDG_CONFIG_HOME/ytr
// or falls back to ~/.config/ytr (following gh CLI pattern, not
// os.UserConfigDir which returns ~/Library/ on macOS).
func ConfigDir() (string, error) {
	if dir := os.Getenv("YTR_CONFIG_DIR"); dir != "" {
		return dir, nil
	}

	if runtime.GOOS == "windows" {
		if appData := os.Getenv("APPDATA"); appData != "" {
			return filepath.Join(appData, "ytr"), nil
		}
	}

	if xdgConfig := os.Getenv("XDG_CONFIG_HOME"); xdgConfig != "" {
		return filepath.Join(xdgConfig, "ytr"), nil
	}

	homeDir, err := os.UserHomeDir()
	if err != nil {
		return "", fmt.Errorf("cannot determine config directory: %w", err)
	}

	return filepath.Join(homeDir, ".config", "ytr"), nil
}

// ConfigFilePath returns the full path to the ytr config file.
func ConfigFilePath() (string, error) {
	dir, err := ConfigDir()
	if err != nil {
		return "", err
	}

	return filepath.Join(dir, "config.yaml"), nil
}
