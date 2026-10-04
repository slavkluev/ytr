package config_test

import (
	"context"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"testing"

	"github.com/slavkluev/ytr/internal/config"
)

func TestConfigDir_Default(t *testing.T) {
	t.Setenv("YTR_CONFIG_DIR", "")
	t.Setenv("XDG_CONFIG_HOME", "")

	homeDir, err := os.UserHomeDir()
	if err != nil {
		t.Fatal(err)
	}

	want := filepath.Join(homeDir, ".config", "ytr")
	got, err := config.ConfigDir(t.Context())
	if err != nil {
		t.Fatalf("ConfigDir() unexpected error: %v", err)
	}
	if got != want {
		t.Errorf("ConfigDir() = %q, want %q", got, want)
	}
}

func TestConfigDir_YTRConfigDir(t *testing.T) {
	t.Setenv("YTR_CONFIG_DIR", "/tmp/custom")

	got, err := config.ConfigDir(t.Context())
	if err != nil {
		t.Fatalf("ConfigDir() unexpected error: %v", err)
	}
	if got != "/tmp/custom" {
		t.Errorf("ConfigDir() = %q, want %q", got, "/tmp/custom")
	}
}

func TestConfigDir_XDGConfigHome(t *testing.T) {
	t.Setenv("YTR_CONFIG_DIR", "")
	t.Setenv("XDG_CONFIG_HOME", "/tmp/xdg")

	want := filepath.Join("/tmp/xdg", "ytr")
	got, err := config.ConfigDir(t.Context())
	if err != nil {
		t.Fatalf("ConfigDir() unexpected error: %v", err)
	}
	if got != want {
		t.Errorf("ConfigDir() = %q, want %q", got, want)
	}
}

func TestConfigFilePath(t *testing.T) {
	t.Setenv("YTR_CONFIG_DIR", "/tmp/testcfg")

	want := filepath.Join("/tmp/testcfg", "config.yaml")
	got, err := config.ConfigFilePath(t.Context())
	if err != nil {
		t.Fatalf("ConfigFilePath() unexpected error: %v", err)
	}
	if got != want {
		t.Errorf("ConfigFilePath() = %q, want %q", got, want)
	}
}

func withEnv(t *testing.T, env map[string]string) context.Context {
	t.Helper()

	return config.WithEnv(t.Context(), func(name string) (string, bool) {
		value, ok := env[name]
		return value, ok
	})
}

func TestConfigDirReadsTheEnvironmentItsContextCarries(t *testing.T) {
	t.Setenv("YTR_CONFIG_DIR", "/tmp/process")
	t.Setenv("XDG_CONFIG_HOME", "/tmp/process-xdg")

	got, err := config.ConfigDir(withEnv(t, map[string]string{"XDG_CONFIG_HOME": "/tmp/xdg", "HOME": "/tmp/home"}))
	if want := filepath.Join("/tmp/xdg", "ytr"); err != nil || got != want {
		t.Errorf("ConfigDir() = %q, %v, want %q", got, err, want)
	}

	if runtime.GOOS == "windows" {
		t.Skip("Windows reads APPDATA and USERPROFILE, not HOME")
	}

	got, err = config.ConfigDir(withEnv(t, map[string]string{"HOME": "/tmp/home"}))
	if want := filepath.Join("/tmp/home", ".config", "ytr"); err != nil || got != want {
		t.Errorf("ConfigDir() with only HOME = %q, %v, want %q", got, err, want)
	}

	if got, err = config.ConfigDir(withEnv(t, nil)); err == nil || !strings.Contains(err.Error(), "$HOME") {
		t.Errorf("ConfigDir() with no HOME = %q, %v, want an error naming $HOME", got, err)
	}
}
