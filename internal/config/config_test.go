package config_test

import (
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/slavkluev/ytr/internal/config"
	ytrerrors "github.com/slavkluev/ytr/internal/errors"
)

func TestLoad_NoFile(t *testing.T) {
	dir := t.TempDir()
	t.Setenv("YTR_CONFIG_DIR", dir)

	cfg, err := config.Load()
	if err != nil {
		t.Fatalf("Load() returned error: %v", err)
	}
	if cfg.Token != "" || cfg.OrgID != "" || cfg.OrgType != "" {
		t.Errorf("Load() = %+v, want empty Config", cfg)
	}
}

func TestLoad_ValidYAML(t *testing.T) {
	dir := t.TempDir()
	t.Setenv("YTR_CONFIG_DIR", dir)

	content := "token: tok\norg_id: org\norg_type: cloud\n"
	if err := os.WriteFile(filepath.Join(dir, "config.yaml"), []byte(content), 0600); err != nil {
		t.Fatal(err)
	}

	cfg, err := config.Load()
	if err != nil {
		t.Fatalf("Load() returned error: %v", err)
	}
	if cfg.Token != "tok" {
		t.Errorf("Token = %q, want %q", cfg.Token, "tok")
	}
	if cfg.OrgID != "org" {
		t.Errorf("OrgID = %q, want %q", cfg.OrgID, "org")
	}
	if cfg.OrgType != config.OrgTypeCloud {
		t.Errorf("OrgType = %q, want %q", cfg.OrgType, config.OrgTypeCloud)
	}
}

func TestLoad_InvalidYAML(t *testing.T) {
	dir := t.TempDir()
	t.Setenv("YTR_CONFIG_DIR", dir)

	content := "token:\n  - nested: [invalid\n"
	if err := os.WriteFile(filepath.Join(dir, "config.yaml"), []byte(content), 0600); err != nil {
		t.Fatal(err)
	}

	_, err := config.Load()
	if err == nil {
		t.Fatal("Load() should return error for invalid YAML")
	}
	if got := err.Error(); !strings.Contains(got, "failed to parse config") {
		t.Errorf("error = %q, want to contain %q", got, "failed to parse config")
	}
}

func TestSave_CreatesDir(t *testing.T) {
	dir := filepath.Join(t.TempDir(), "nested", "dir")
	t.Setenv("YTR_CONFIG_DIR", dir)

	cfg := &config.Config{
		Token:   "t",
		OrgID:   "o",
		OrgType: config.OrgType360,
	}
	if err := config.Save(cfg); err != nil {
		t.Fatalf("Save() returned error: %v", err)
	}

	info, err := os.Stat(dir)
	if err != nil {
		t.Fatalf("config dir not created: %v", err)
	}
	if !info.IsDir() {
		t.Error("config dir is not a directory")
	}
	if perm := info.Mode().Perm(); perm != 0700 {
		t.Errorf("config dir perm = %o, want %o", perm, 0700)
	}
}

func TestSave_WritesYAML(t *testing.T) {
	dir := t.TempDir()
	t.Setenv("YTR_CONFIG_DIR", dir)

	cfg := &config.Config{
		Token:   "mytoken",
		OrgID:   "myorg",
		OrgType: config.OrgTypeCloud,
	}
	if err := config.Save(cfg); err != nil {
		t.Fatalf("Save() returned error: %v", err)
	}

	loaded, err := config.Load()
	if err != nil {
		t.Fatalf("Load() returned error: %v", err)
	}
	if loaded.Token != "mytoken" {
		t.Errorf("Token = %q, want %q", loaded.Token, "mytoken")
	}
	if loaded.OrgID != "myorg" {
		t.Errorf("OrgID = %q, want %q", loaded.OrgID, "myorg")
	}
	if loaded.OrgType != config.OrgTypeCloud {
		t.Errorf("OrgType = %q, want %q", loaded.OrgType, config.OrgTypeCloud)
	}
}

func TestSave_FilePermissions(t *testing.T) {
	dir := t.TempDir()
	t.Setenv("YTR_CONFIG_DIR", dir)

	cfg := &config.Config{
		Token:   "t",
		OrgID:   "o",
		OrgType: config.OrgType360,
	}
	if err := config.Save(cfg); err != nil {
		t.Fatalf("Save() returned error: %v", err)
	}

	info, err := os.Stat(filepath.Join(dir, "config.yaml"))
	if err != nil {
		t.Fatal(err)
	}
	if perm := info.Mode().Perm(); perm != 0600 {
		t.Errorf("file perm = %o, want %o", perm, 0600)
	}
}

func TestSave_AtomicRename(t *testing.T) {
	dir := t.TempDir()
	t.Setenv("YTR_CONFIG_DIR", dir)

	cfg := &config.Config{
		Token:   "t",
		OrgID:   "o",
		OrgType: config.OrgType360,
	}
	if err := config.Save(cfg); err != nil {
		t.Fatalf("Save() returned error: %v", err)
	}

	entries, _ := os.ReadDir(dir)
	for _, entry := range entries {
		if filepath.Ext(entry.Name()) == ".tmp" {
			t.Errorf("temp file %q still exists", entry.Name())
		}
	}

	if _, err := os.Stat(filepath.Join(dir, "config.yaml")); err != nil {
		t.Errorf("config.yaml not found: %v", err)
	}
}

func TestResolveAuth_Flags(t *testing.T) {
	auth, err := config.ResolveAuth("flagtoken", "flagorg", "360")
	if err != nil {
		t.Fatalf("ResolveAuth() returned error: %v", err)
	}
	if auth.Token != "flagtoken" {
		t.Errorf("Token = %q, want %q", auth.Token, "flagtoken")
	}
	if auth.OrgID != "flagorg" {
		t.Errorf("OrgID = %q, want %q", auth.OrgID, "flagorg")
	}
	if auth.OrgType != config.OrgType360 {
		t.Errorf("OrgType = %q, want %q", auth.OrgType, config.OrgType360)
	}
	if auth.TokenSource != "flag" {
		t.Errorf("TokenSource = %q, want %q", auth.TokenSource, "flag")
	}
}

func TestResolveAuth_EnvVars(t *testing.T) {
	t.Setenv("YTR_TOKEN", "envtoken")
	t.Setenv("YTR_ORG_ID", "envorg")
	t.Setenv("YTR_ORG_TYPE", "cloud")
	t.Setenv("YTR_CONFIG_DIR", t.TempDir())

	auth, err := config.ResolveAuth("", "", "")
	if err != nil {
		t.Fatalf("ResolveAuth() returned error: %v", err)
	}
	if auth.Token != "envtoken" {
		t.Errorf("Token = %q, want %q", auth.Token, "envtoken")
	}
	if auth.OrgID != "envorg" {
		t.Errorf("OrgID = %q, want %q", auth.OrgID, "envorg")
	}
	if auth.OrgType != config.OrgTypeCloud {
		t.Errorf("OrgType = %q, want %q", auth.OrgType, config.OrgTypeCloud)
	}
	if auth.TokenSource != "env" {
		t.Errorf("TokenSource = %q, want %q", auth.TokenSource, "env")
	}
}

func TestResolveAuth_ConfigFile(t *testing.T) {
	dir := t.TempDir()
	t.Setenv("YTR_CONFIG_DIR", dir)
	t.Setenv("YTR_TOKEN", "")
	t.Setenv("YTR_ORG_ID", "")
	t.Setenv("YTR_ORG_TYPE", "")

	content := "token: cfgtoken\norg_id: cfgorg\norg_type: 360\n"
	if err := os.WriteFile(filepath.Join(dir, "config.yaml"), []byte(content), 0600); err != nil {
		t.Fatal(err)
	}

	auth, err := config.ResolveAuth("", "", "")
	if err != nil {
		t.Fatalf("ResolveAuth() returned error: %v", err)
	}
	if auth.Token != "cfgtoken" {
		t.Errorf("Token = %q, want %q", auth.Token, "cfgtoken")
	}
	if auth.OrgID != "cfgorg" {
		t.Errorf("OrgID = %q, want %q", auth.OrgID, "cfgorg")
	}
	if auth.OrgType != config.OrgType360 {
		t.Errorf("OrgType = %q, want %q", auth.OrgType, config.OrgType360)
	}
	if auth.TokenSource != "config" {
		t.Errorf("TokenSource = %q, want %q", auth.TokenSource, "config")
	}
}

func TestResolveAuth_InvalidConfigIsSurfaced(t *testing.T) {
	dir := t.TempDir()
	t.Setenv("YTR_CONFIG_DIR", dir)
	t.Setenv("YTR_TOKEN", "")
	t.Setenv("YTR_ORG_ID", "")
	t.Setenv("YTR_ORG_TYPE", "")

	content := "token:\n  - nested: [invalid\n"
	if err := os.WriteFile(filepath.Join(dir, "config.yaml"), []byte(content), 0600); err != nil {
		t.Fatal(err)
	}

	_, err := config.ResolveAuth("", "", "")
	if err == nil {
		t.Fatal("ResolveAuth() should return error for invalid config")
	}

	exitErr := &ytrerrors.ExitError{}
	if !errors.As(err, &exitErr) {
		t.Fatalf("error type = %T, want *ExitError", err)
	}
	if exitErr.Code != "user_error" {
		t.Errorf("Code = %q, want %q", exitErr.Code, "user_error")
	}
	if !strings.Contains(exitErr.Message, "failed to load config") {
		t.Errorf("Message = %q, want to contain %q", exitErr.Message, "failed to load config")
	}
	if !strings.Contains(exitErr.Suggestion, filepath.Join(dir, "config.yaml")) {
		t.Errorf("Suggestion = %q, want to contain config path", exitErr.Suggestion)
	}
}

func TestResolveAuth_Precedence(t *testing.T) {
	dir := t.TempDir()
	t.Setenv("YTR_CONFIG_DIR", dir)
	t.Setenv("YTR_TOKEN", "envtoken")
	t.Setenv("YTR_ORG_ID", "envorg")
	t.Setenv("YTR_ORG_TYPE", "cloud")

	content := "token: cfgtoken\norg_id: cfgorg\norg_type: 360\n"
	if err := os.WriteFile(filepath.Join(dir, "config.yaml"), []byte(content), 0600); err != nil {
		t.Fatal(err)
	}

	auth, err := config.ResolveAuth("flagtoken", "flagorg", "360")
	if err != nil {
		t.Fatalf("ResolveAuth() returned error: %v", err)
	}
	if auth.TokenSource != "flag" {
		t.Errorf("TokenSource = %q, want %q (flags beat env)", auth.TokenSource, "flag")
	}

	auth, err = config.ResolveAuth("", "", "")
	if err != nil {
		t.Fatalf("ResolveAuth() returned error: %v", err)
	}
	if auth.TokenSource != "env" {
		t.Errorf("TokenSource = %q, want %q (env beats config)", auth.TokenSource, "env")
	}
	if auth.OrgType != config.OrgTypeCloud {
		t.Errorf("OrgType = %q, want %q", auth.OrgType, config.OrgTypeCloud)
	}
}

func TestResolveAuth_PartialEnvFallsBackToConfig(t *testing.T) {
	dir := t.TempDir()
	t.Setenv("YTR_CONFIG_DIR", dir)
	t.Setenv("YTR_TOKEN", "envtoken")
	t.Setenv("YTR_ORG_ID", "envorg")
	t.Setenv("YTR_ORG_TYPE", "")

	content := "token: cfgtoken\norg_id: cfgorg\norg_type: 360\n"
	if err := os.WriteFile(filepath.Join(dir, "config.yaml"), []byte(content), 0600); err != nil {
		t.Fatal(err)
	}

	auth, err := config.ResolveAuth("", "", "")
	if err != nil {
		t.Fatalf("ResolveAuth() returned error: %v", err)
	}
	if auth.TokenSource != "config" {
		t.Errorf("TokenSource = %q, want %q", auth.TokenSource, "config")
	}
	if auth.Token != "cfgtoken" {
		t.Errorf("Token = %q, want %q", auth.Token, "cfgtoken")
	}
	if auth.OrgType != config.OrgType360 {
		t.Errorf("OrgType = %q, want %q", auth.OrgType, config.OrgType360)
	}
}

func TestResolveAuth_PartialFlagsRejected(t *testing.T) {
	t.Setenv("YTR_CONFIG_DIR", t.TempDir())
	t.Setenv("YTR_TOKEN", "envtoken")
	t.Setenv("YTR_ORG_ID", "envorg")
	t.Setenv("YTR_ORG_TYPE", "cloud")

	_, err := config.ResolveAuth("flagtoken", "flagorg", "")
	if err == nil {
		t.Fatal("ResolveAuth() should reject incomplete flag authentication")
	}

	exitErr := &ytrerrors.ExitError{}
	if !errors.As(err, &exitErr) {
		t.Fatalf("error type = %T, want *ExitError", err)
	}
	if exitErr.Code != "user_error" {
		t.Errorf("Code = %q, want %q", exitErr.Code, "user_error")
	}
	if !strings.Contains(exitErr.Message, "incomplete flag authentication") {
		t.Errorf("Message = %q, want incomplete flag auth error", exitErr.Message)
	}
	if !strings.Contains(exitErr.Suggestion, "--token") {
		t.Errorf("Suggestion = %q, want flag guidance", exitErr.Suggestion)
	}
}

func TestResolveAuth_InvalidPartialFlagsRejected(t *testing.T) {
	t.Setenv("YTR_CONFIG_DIR", t.TempDir())
	t.Setenv("YTR_TOKEN", "envtoken")
	t.Setenv("YTR_ORG_ID", "envorg")
	t.Setenv("YTR_ORG_TYPE", "cloud")

	_, err := config.ResolveAuth("flagtoken", "flagorg", "invalid")
	if err == nil {
		t.Fatal("ResolveAuth() should reject invalid partial flag org type")
	}

	exitErr := &ytrerrors.ExitError{}
	if !errors.As(err, &exitErr) {
		t.Fatalf("error type = %T, want *ExitError", err)
	}
	if exitErr.Code != "user_error" {
		t.Errorf("Code = %q, want %q", exitErr.Code, "user_error")
	}
	if !strings.Contains(exitErr.Suggestion, "--org-type 360") {
		t.Errorf("Suggestion = %q, want flag guidance", exitErr.Suggestion)
	}
}

func TestResolveAuth_InvalidPartialEnvFallsBackToConfig(t *testing.T) {
	dir := t.TempDir()
	t.Setenv("YTR_CONFIG_DIR", dir)
	t.Setenv("YTR_TOKEN", "")
	t.Setenv("YTR_ORG_ID", "")
	t.Setenv("YTR_ORG_TYPE", "invalid")

	content := "token: cfgtoken\norg_id: cfgorg\norg_type: cloud\n"
	if err := os.WriteFile(filepath.Join(dir, "config.yaml"), []byte(content), 0600); err != nil {
		t.Fatal(err)
	}

	auth, err := config.ResolveAuth("", "", "")
	if err != nil {
		t.Fatalf("ResolveAuth() returned error: %v", err)
	}
	if auth.TokenSource != "config" {
		t.Errorf("TokenSource = %q, want %q", auth.TokenSource, "config")
	}
	if auth.OrgType != config.OrgTypeCloud {
		t.Errorf("OrgType = %q, want %q", auth.OrgType, config.OrgTypeCloud)
	}
}

func TestResolveAuth_PartialConfigRejected(t *testing.T) {
	dir := t.TempDir()
	t.Setenv("YTR_CONFIG_DIR", dir)
	t.Setenv("YTR_TOKEN", "")
	t.Setenv("YTR_ORG_ID", "")
	t.Setenv("YTR_ORG_TYPE", "")

	content := "token: cfgtoken\norg_id: cfgorg\n"
	if err := os.WriteFile(filepath.Join(dir, "config.yaml"), []byte(content), 0600); err != nil {
		t.Fatal(err)
	}

	_, err := config.ResolveAuth("", "", "")
	if err == nil {
		t.Fatal("ResolveAuth() should reject incomplete config authentication")
	}

	exitErr := &ytrerrors.ExitError{}
	if !errors.As(err, &exitErr) {
		t.Fatalf("error type = %T, want *ExitError", err)
	}
	if exitErr.Code != "user_error" {
		t.Errorf("Code = %q, want %q", exitErr.Code, "user_error")
	}
	if !strings.Contains(exitErr.Message, "incomplete config authentication") {
		t.Errorf("Message = %q, want incomplete config auth error", exitErr.Message)
	}
	if !strings.Contains(exitErr.Suggestion, filepath.Join(dir, "config.yaml")) {
		t.Errorf("Suggestion = %q, want config path", exitErr.Suggestion)
	}
}

func TestResolveAuth_InvalidPartialConfigRejected(t *testing.T) {
	dir := t.TempDir()
	t.Setenv("YTR_CONFIG_DIR", dir)
	t.Setenv("YTR_TOKEN", "")
	t.Setenv("YTR_ORG_ID", "")
	t.Setenv("YTR_ORG_TYPE", "")

	content := "org_type: invalid\n"
	if err := os.WriteFile(filepath.Join(dir, "config.yaml"), []byte(content), 0600); err != nil {
		t.Fatal(err)
	}

	_, err := config.ResolveAuth("", "", "")
	if err == nil {
		t.Fatal("ResolveAuth() should reject invalid partial config org type")
	}

	exitErr := &ytrerrors.ExitError{}
	if !errors.As(err, &exitErr) {
		t.Fatalf("error type = %T, want *ExitError", err)
	}
	if exitErr.Code != "user_error" {
		t.Errorf("Code = %q, want %q", exitErr.Code, "user_error")
	}
	if !strings.Contains(exitErr.Message, `invalid org-type "invalid"`) {
		t.Errorf("Message = %q, want invalid org-type error", exitErr.Message)
	}
	if !strings.Contains(exitErr.Suggestion, filepath.Join(dir, "config.yaml")) {
		t.Errorf("Suggestion = %q, want config guidance", exitErr.Suggestion)
	}
}

func TestResolveAuth_InvalidOrgTypeRejected(t *testing.T) {
	_, err := config.ResolveAuth("flagtoken", "flagorg", "invalid")
	if err == nil {
		t.Fatal("ResolveAuth() should reject invalid org type")
	}

	exitErr := &ytrerrors.ExitError{}
	if !errors.As(err, &exitErr) {
		t.Fatalf("error type = %T, want *ExitError", err)
	}
	if exitErr.Code != "user_error" {
		t.Errorf("Code = %q, want %q", exitErr.Code, "user_error")
	}
	if !strings.Contains(exitErr.Suggestion, "--org-type 360") {
		t.Errorf("Suggestion = %q, want flag guidance", exitErr.Suggestion)
	}
}

func TestResolveAuth_NoAuth(t *testing.T) {
	dir := t.TempDir()
	t.Setenv("YTR_CONFIG_DIR", dir)
	t.Setenv("YTR_TOKEN", "")
	t.Setenv("YTR_ORG_ID", "")
	t.Setenv("YTR_ORG_TYPE", "")

	_, err := config.ResolveAuth("", "", "")
	if err == nil {
		t.Fatal("ResolveAuth() should return error when no auth configured")
	}

	exitErr := &ytrerrors.ExitError{}
	if !errors.As(err, &exitErr) {
		t.Fatalf("error type = %T, want *ExitError", err)
	}
	if exitErr.Code != "auth_error" {
		t.Errorf("Code = %q, want %q", exitErr.Code, "auth_error")
	}
	if !strings.Contains(exitErr.Suggestion, "ytr auth login") {
		t.Errorf("Suggestion = %q, want to contain %q", exitErr.Suggestion, "ytr auth login")
	}
	if !strings.Contains(exitErr.Suggestion, "YTR_ORG_TYPE") {
		t.Errorf("Suggestion = %q, want to contain %q", exitErr.Suggestion, "YTR_ORG_TYPE")
	}
}
