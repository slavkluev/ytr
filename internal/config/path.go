package config

import (
	"context"
	"errors"
	"fmt"
	"path/filepath"
	"runtime"
)

// ConfigDir returns the directory where ytr stores its configuration.
// Resolution order: YTR_CONFIG_DIR env var > platform default.
// On Windows, uses %APPDATA%\ytr. On Unix, uses XDG_CONFIG_HOME/ytr
// or falls back to ~/.config/ytr (following gh CLI pattern, not
// os.UserConfigDir which returns ~/Library/ on macOS).
func ConfigDir(ctx context.Context) (string, error) {
	if dir := getenv(ctx, "YTR_CONFIG_DIR"); dir != "" {
		return dir, nil
	}

	if runtime.GOOS == "windows" {
		if appData := getenv(ctx, "APPDATA"); appData != "" {
			return filepath.Join(appData, "ytr"), nil
		}
	}

	if xdgConfig := getenv(ctx, "XDG_CONFIG_HOME"); xdgConfig != "" {
		return filepath.Join(xdgConfig, "ytr"), nil
	}

	homeDir, err := userHomeDir(ctx)
	if err != nil {
		return "", fmt.Errorf("cannot determine config directory: %w", err)
	}

	return filepath.Join(homeDir, ".config", "ytr"), nil
}

// userHomeDir is os.UserHomeDir on the platforms ytr ships for, reading the
// variable through ctx like every other one.
func userHomeDir(ctx context.Context) (string, error) {
	name, shown := "HOME", "$HOME"
	if runtime.GOOS == "windows" {
		name, shown = "USERPROFILE", "%userprofile%"
	}

	if dir := getenv(ctx, name); dir != "" {
		return dir, nil
	}

	return "", errors.New(shown + " is not defined")
}

// ConfigFilePath returns the full path to the ytr config file.
func ConfigFilePath(ctx context.Context) (string, error) {
	dir, err := ConfigDir(ctx)
	if err != nil {
		return "", err
	}

	return filepath.Join(dir, "config.yaml"), nil
}
