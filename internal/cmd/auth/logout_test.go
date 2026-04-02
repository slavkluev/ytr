package auth

import (
	"bytes"
	"encoding/json"
	"os"
	"path/filepath"
	"testing"

	"gopkg.in/yaml.v3"

	"github.com/slavkluev/ytr/internal/config"
	"github.com/slavkluev/ytr/internal/output"
	"github.com/slavkluev/ytr/internal/testutil"
)

func TestLogout_ClearsAuth(t *testing.T) {
	dir := setupConfigDir(t)
	testutil.ResetOutputFlags(t)
	writeConfig(t, dir, "existing-token", "existing-org", "cloud")

	logoutCmd := newLogoutCmd()
	buf := new(bytes.Buffer)
	logoutCmd.SetOut(buf)
	logoutCmd.SetErr(buf)
	logoutCmd.SetArgs([]string{})

	err := logoutCmd.Execute()
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	// Read config file back
	data, err := os.ReadFile(filepath.Join(dir, "config.yaml"))
	if err != nil {
		t.Fatalf("config file should still exist: %v", err)
	}

	var cfg config.Config
	if err := yaml.Unmarshal(data, &cfg); err != nil {
		t.Fatalf("failed to parse config: %v", err)
	}

	if cfg.Token != "" {
		t.Errorf("token = %q, want empty string", cfg.Token)
	}
	if cfg.OrgID != "" {
		t.Errorf("org_id = %q, want empty string", cfg.OrgID)
	}
	if cfg.OrgType != "" {
		t.Errorf("org_type = %q, want empty string", cfg.OrgType)
	}
}

func TestLogout_NoConfig(t *testing.T) {
	setupConfigDir(t) // empty dir, no config file
	testutil.ResetOutputFlags(t)

	logoutCmd := newLogoutCmd()
	buf := new(bytes.Buffer)
	logoutCmd.SetOut(buf)
	logoutCmd.SetErr(buf)
	logoutCmd.SetArgs([]string{})

	err := logoutCmd.Execute()
	if err != nil {
		t.Fatalf("logout should succeed silently when no config: %v", err)
	}
}

func TestLogout_PreservesFile(t *testing.T) {
	dir := setupConfigDir(t)
	testutil.ResetOutputFlags(t)
	writeConfig(t, dir, "some-token", "some-org", "360")

	logoutCmd := newLogoutCmd()
	buf := new(bytes.Buffer)
	logoutCmd.SetOut(buf)
	logoutCmd.SetErr(buf)
	logoutCmd.SetArgs([]string{})

	err := logoutCmd.Execute()
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	// Verify file still exists
	cfgPath := filepath.Join(dir, "config.yaml")
	if _, statErr := os.Stat(cfgPath); statErr != nil {
		t.Errorf("config file should still exist after logout: %v", statErr)
	}
}

func TestLogout_JSON(t *testing.T) {
	dir := setupConfigDir(t)
	testutil.ResetOutputFlags(t)
	writeConfig(t, dir, "some-token", "some-org", "360")

	// Auth commands detect JSON via output.IsJSON() or cmd.Flags().Changed("json").
	output.JSONFields = []string{"dummy"}

	logoutCmd := newLogoutCmd()
	buf := new(bytes.Buffer)
	logoutCmd.SetOut(buf)
	logoutCmd.SetErr(buf)
	logoutCmd.SetArgs([]string{})

	err := logoutCmd.Execute()
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	var result map[string]string
	if err := json.Unmarshal(buf.Bytes(), &result); err != nil {
		t.Fatalf("failed to parse JSON: %v (output: %q)", err, buf.String())
	}

	if result["status"] != "logged_out" {
		t.Errorf("status = %q, want %q", result["status"], "logged_out")
	}
	if result["config_path"] == "" {
		t.Error("config_path should not be empty")
	}
}

func TestLogout_HumanOutputGoesToStderr(t *testing.T) {
	dir := setupConfigDir(t)
	testutil.ResetOutputFlags(t)
	writeConfig(t, dir, "some-token", "some-org", "360")

	logoutCmd := newLogoutCmd()
	stdoutBuf := new(bytes.Buffer)
	stderrBuf := new(bytes.Buffer)
	logoutCmd.SetOut(stdoutBuf)
	logoutCmd.SetErr(stderrBuf)
	logoutCmd.SetArgs([]string{})

	err := logoutCmd.Execute()
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if stdoutBuf.Len() != 0 {
		t.Errorf("expected no stdout output in human mode, got %q", stdoutBuf.String())
	}
	if !bytes.Contains(stderrBuf.Bytes(), []byte("Logged out. Credentials removed")) {
		t.Errorf("expected stderr confirmation, got %q", stderrBuf.String())
	}
}
