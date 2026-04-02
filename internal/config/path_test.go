package config_test

import (
	"os"
	"path/filepath"
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
	got, err := config.ConfigDir()
	if err != nil {
		t.Fatalf("ConfigDir() unexpected error: %v", err)
	}
	if got != want {
		t.Errorf("ConfigDir() = %q, want %q", got, want)
	}
}

func TestConfigDir_YTRConfigDir(t *testing.T) {
	t.Setenv("YTR_CONFIG_DIR", "/tmp/custom")

	got, err := config.ConfigDir()
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
	got, err := config.ConfigDir()
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
	got, err := config.ConfigFilePath()
	if err != nil {
		t.Fatalf("ConfigFilePath() unexpected error: %v", err)
	}
	if got != want {
		t.Errorf("ConfigFilePath() = %q, want %q", got, want)
	}
}
