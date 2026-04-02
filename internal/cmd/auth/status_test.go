package auth

import (
	"bytes"
	"encoding/json"
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/slavkluev/go-yandex-tracker/tracker"

	ytrerrors "github.com/slavkluev/ytr/internal/errors"
	"github.com/slavkluev/ytr/internal/output"
	"github.com/slavkluev/ytr/internal/testutil"
)

// writeConfig writes a config file with the given token, org_id, and org_type.
func writeConfig(t *testing.T, dir, token, orgID, orgType string) {
	t.Helper()
	cfgDir := dir
	if err := os.MkdirAll(cfgDir, 0700); err != nil {
		t.Fatalf("failed to create config dir: %v", err)
	}
	data := []byte(
		"token: " + token + "\norg_id: " + orgID + "\norg_type: " + orgType + "\n",
	)
	if err := os.WriteFile(filepath.Join(cfgDir, "config.yaml"), data, 0600); err != nil {
		t.Fatalf("failed to write config: %v", err)
	}
}

func TestStatus_Authenticated(t *testing.T) {
	dir := setupConfigDir(t)
	testutil.ResetOutputFlags(t)
	writeConfig(t, dir, "valid-token", "org-123", "cloud")

	display := "Status User"
	withMockValidator(t, &mockUserValidator{
		user: &tracker.User{Display: &display},
	})

	statusCmd := newStatusCmd()
	buf := new(bytes.Buffer)
	statusCmd.SetOut(buf)
	statusCmd.SetErr(buf)
	statusCmd.SetArgs([]string{})

	err := statusCmd.Execute()
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	out := buf.String()
	if !strings.Contains(out, "Status User") {
		t.Errorf("output %q does not contain username", out)
	}
	if !strings.Contains(out, "config") {
		t.Errorf("output %q does not contain token source", out)
	}
	if !strings.Contains(out, "org-123") {
		t.Errorf("output %q does not contain org ID", out)
	}
	if !strings.Contains(out, "cloud") {
		t.Errorf("output %q does not contain org type", out)
	}
}

func TestStatus_Authenticated_JSON(t *testing.T) {
	dir := setupConfigDir(t)
	testutil.ResetOutputFlags(t)
	writeConfig(t, dir, "valid-token", "org-123", "cloud")

	display := "JSON Status User"
	withMockValidator(t, &mockUserValidator{
		user: &tracker.User{Display: &display},
	})

	// Auth commands detect JSON via output.IsJSON() or cmd.Flags().Changed("json").
	output.JSONFields = []string{"dummy"}

	statusCmd := newStatusCmd()
	buf := new(bytes.Buffer)
	statusCmd.SetOut(buf)
	statusCmd.SetErr(buf)
	statusCmd.SetArgs([]string{})

	err := statusCmd.Execute()
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	var result map[string]string
	if err := json.Unmarshal(buf.Bytes(), &result); err != nil {
		t.Fatalf("failed to parse JSON: %v (output: %q)", err, buf.String())
	}

	if result["status"] != "authenticated" {
		t.Errorf("status = %q, want %q", result["status"], "authenticated")
	}
	if result["user"] != "JSON Status User" {
		t.Errorf("user = %q, want %q", result["user"], "JSON Status User")
	}
	if result["org_id"] != "org-123" {
		t.Errorf("org_id = %q, want %q", result["org_id"], "org-123")
	}
	if result["org_type"] != "cloud" {
		t.Errorf("org_type = %q, want %q", result["org_type"], "cloud")
	}
	if result["token_source"] != "config" {
		t.Errorf("token_source = %q, want %q", result["token_source"], "config")
	}
}

func TestStatus_NotAuthenticated(t *testing.T) {
	setupConfigDir(t) // empty dir, no config
	testutil.ResetOutputFlags(t)

	// Clear env vars that might provide auth
	t.Setenv("YTR_TOKEN", "")
	t.Setenv("YTR_ORG_ID", "")
	t.Setenv("YTR_ORG_TYPE", "")

	statusCmd := newStatusCmd()
	buf := new(bytes.Buffer)
	statusCmd.SetOut(buf)
	statusCmd.SetErr(buf)
	statusCmd.SetArgs([]string{})

	err := statusCmd.Execute()
	if err == nil {
		t.Fatal("expected error when not authenticated, got nil")
	}

	var exitErr *ytrerrors.ExitError
	if !isExitError(err, &exitErr) {
		t.Fatalf("expected ExitError, got %T: %v", err, err)
	}
	if exitErr.Code != "auth_error" {
		t.Errorf("error code = %q, want %q", exitErr.Code, "auth_error")
	}
}

func TestStatus_EnvVarSource(t *testing.T) {
	setupConfigDir(t) // no config file
	testutil.ResetOutputFlags(t)

	t.Setenv("YTR_TOKEN", "env-token")
	t.Setenv("YTR_ORG_ID", "env-org")
	t.Setenv("YTR_ORG_TYPE", "360")

	display := "Env User"
	withMockValidator(t, &mockUserValidator{
		user: &tracker.User{Display: &display},
	})

	// Auth commands detect JSON via output.IsJSON() or cmd.Flags().Changed("json").
	output.JSONFields = []string{"dummy"}

	statusCmd := newStatusCmd()
	buf := new(bytes.Buffer)
	statusCmd.SetOut(buf)
	statusCmd.SetErr(buf)
	statusCmd.SetArgs([]string{})

	err := statusCmd.Execute()
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	var result map[string]string
	if err := json.Unmarshal(buf.Bytes(), &result); err != nil {
		t.Fatalf("failed to parse JSON: %v (output: %q)", err, buf.String())
	}

	if result["token_source"] != "env" {
		t.Errorf("token_source = %q, want %q", result["token_source"], "env")
	}
	if result["org_type"] != "360" {
		t.Errorf("org_type = %q, want %q", result["org_type"], "360")
	}
}

func TestStatus_ValidationFails(t *testing.T) {
	dir := setupConfigDir(t)
	testutil.ResetOutputFlags(t)
	writeConfig(t, dir, "expired-token", "org-123", "360")

	withMockValidator(t, &mockUserValidator{
		err: errors.New("API request failed: token expired"),
	})

	statusCmd := newStatusCmd()
	buf := new(bytes.Buffer)
	statusCmd.SetOut(buf)
	statusCmd.SetErr(buf)
	statusCmd.SetArgs([]string{})

	err := statusCmd.Execute()
	if err == nil {
		t.Fatal("expected error when validation fails, got nil")
	}
}

// isExitError checks if err is an ExitError (helper for test assertions).
func isExitError(err error, target **ytrerrors.ExitError) bool {
	var exitErr *ytrerrors.ExitError
	if errors.As(err, &exitErr) {
		*target = exitErr
		return true
	}
	return false
}
