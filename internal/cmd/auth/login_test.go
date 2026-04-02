package auth

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/slavkluev/go-yandex-tracker/tracker"

	"gopkg.in/yaml.v3"

	"github.com/slavkluev/ytr/internal/api"
	"github.com/slavkluev/ytr/internal/config"
	ytrerrors "github.com/slavkluev/ytr/internal/errors"
	"github.com/slavkluev/ytr/internal/output"
	"github.com/slavkluev/ytr/internal/testutil"
)

// mockUserValidator is a test double for the userValidator interface.
type mockUserValidator struct {
	user *tracker.User
	resp *tracker.Response
	err  error
}

func (m *mockUserValidator) Myself(ctx context.Context) (*tracker.User, *tracker.Response, error) {
	return m.user, m.resp, m.err
}

type mockOrgTypeDetector struct {
	orgType config.OrgType
	user    *tracker.User
	err     error
}

func (m *mockOrgTypeDetector) Detect(
	ctx context.Context,
	token, orgID string,
) (config.OrgType, *tracker.User, error) {
	return m.orgType, m.user, m.err
}

func newTrackerError(status int, messages ...string) error {
	req, _ := http.NewRequest(http.MethodGet, "https://tracker.yandex.net/v3/myself", nil)

	return &tracker.ErrorResponse{
		Response: &http.Response{
			StatusCode: status,
			Request:    req,
		},
		ErrorMessages: messages,
	}
}

// setupConfigDir sets up a temporary config directory for testing.
func setupConfigDir(t *testing.T) string {
	t.Helper()
	dir := t.TempDir()
	t.Setenv("YTR_CONFIG_DIR", dir)
	return dir
}

// withMockValidator sets up a mock validator and restores the original on cleanup.
func withMockValidator(t *testing.T, mock *mockUserValidator) {
	t.Helper()
	orig := newValidator
	newValidator = func(auth *config.ResolvedAuth) userValidator {
		return mock
	}
	t.Cleanup(func() { newValidator = orig })
}

func withMockDetector(t *testing.T, mock *mockOrgTypeDetector) {
	t.Helper()
	orig := detectOrgType
	detectOrgType = mock
	t.Cleanup(func() { detectOrgType = orig })
}

func withValidatorFactory(
	t *testing.T,
	factory func(auth *config.ResolvedAuth) userValidator,
) {
	t.Helper()
	orig := newValidator
	newValidator = factory
	t.Cleanup(func() { newValidator = orig })
}

func TestLoginWithFlags(t *testing.T) {
	dir := setupConfigDir(t)
	testutil.ResetOutputFlags(t)

	display := "Test User"
	withMockValidator(t, &mockUserValidator{
		user: &tracker.User{Display: &display},
	})

	loginCmd := newLoginCmd()
	buf := new(bytes.Buffer)
	loginCmd.SetOut(buf)
	loginCmd.SetErr(buf)
	loginCmd.SetArgs([]string{
		"--token", "test-token",
		"--org-id", "test-org",
		"--org-type", "360",
	})

	err := loginCmd.Execute()
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	// Verify config was saved
	data, err := os.ReadFile(filepath.Join(dir, "config.yaml"))
	if err != nil {
		t.Fatalf("config file not created: %v", err)
	}

	var cfg config.Config
	if err := yaml.Unmarshal(data, &cfg); err != nil {
		t.Fatalf("failed to parse config: %v", err)
	}

	if cfg.Token != "test-token" {
		t.Errorf("token = %q, want %q", cfg.Token, "test-token")
	}
	if cfg.OrgID != "test-org" {
		t.Errorf("org_id = %q, want %q", cfg.OrgID, "test-org")
	}
	if cfg.OrgType != config.OrgType360 {
		t.Errorf("org_type = %q, want %q", cfg.OrgType, config.OrgType360)
	}

	// Verify output contains username
	out := buf.String()
	if !strings.Contains(out, "Test User") {
		t.Errorf("output %q does not contain username", out)
	}
}

func TestLoginWithFlags_InvalidToken(t *testing.T) {
	setupConfigDir(t)
	testutil.ResetOutputFlags(t)

	withMockValidator(t, &mockUserValidator{
		err: errors.New("API request failed: unauthorized"),
	})

	loginCmd := newLoginCmd()
	buf := new(bytes.Buffer)
	loginCmd.SetOut(buf)
	loginCmd.SetErr(buf)
	loginCmd.SetArgs([]string{
		"--token", "bad-token",
		"--org-id", "test-org",
		"--org-type", "360",
	})

	err := loginCmd.Execute()
	if err == nil {
		t.Fatal("expected error for invalid token, got nil")
	}

	// Verify config was NOT created
	cfgPath, err := config.ConfigFilePath()
	if err != nil {
		t.Fatalf("ConfigFilePath() unexpected error: %v", err)
	}
	if _, statErr := os.Stat(cfgPath); statErr == nil {
		t.Error("config file should not exist after failed login")
	}
}

func TestLoginWithFlags_JSONOutput(t *testing.T) {
	setupConfigDir(t)
	testutil.ResetOutputFlags(t)

	display := "JSON User"
	withMockValidator(t, &mockUserValidator{
		user: &tracker.User{Display: &display},
	})

	// Auth commands detect JSON via output.IsJSON() or cmd.Flags().Changed("json").
	// Setting JSONFields makes IsJSON() return true.
	output.JSONFields = []string{"dummy"}

	loginCmd := newLoginCmd()
	buf := new(bytes.Buffer)
	loginCmd.SetOut(buf)
	loginCmd.SetErr(buf)
	loginCmd.SetArgs([]string{
		"--token", "test-token",
		"--org-id", "test-org",
		"--org-type", "cloud",
	})

	err := loginCmd.Execute()
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	var result map[string]string
	if err := json.Unmarshal(buf.Bytes(), &result); err != nil {
		t.Fatalf("failed to parse JSON output: %v (output: %q)", err, buf.String())
	}

	if result["status"] != "authenticated" {
		t.Errorf("status = %q, want %q", result["status"], "authenticated")
	}
	if result["user"] != "JSON User" {
		t.Errorf("user = %q, want %q", result["user"], "JSON User")
	}
	if result["org_id"] != "test-org" {
		t.Errorf("org_id = %q, want %q", result["org_id"], "test-org")
	}
	if result["org_type"] != "cloud" {
		t.Errorf("org_type = %q, want %q", result["org_type"], "cloud")
	}
	if result["config_path"] == "" {
		t.Error("config_path should not be empty")
	}
}

func TestLoginPipedStdin(t *testing.T) {
	dir := setupConfigDir(t)
	testutil.ResetOutputFlags(t)

	display := "Piped User"
	withMockDetector(t, &mockOrgTypeDetector{
		orgType: config.OrgTypeCloud,
		user:    &tracker.User{Display: &display},
	})

	// Create a pipe to simulate piped stdin
	r, w, err := os.Pipe()
	if err != nil {
		t.Fatalf("failed to create pipe: %v", err)
	}

	_, err = w.WriteString("piped-token\n")
	if err != nil {
		t.Fatalf("failed to write to pipe: %v", err)
	}
	if err := w.Close(); err != nil {
		t.Fatalf("failed to close pipe writer: %v", err)
	}

	origStdin := stdinFile
	stdinFile = r
	t.Cleanup(func() { stdinFile = origStdin })

	loginCmd := newLoginCmd()
	buf := new(bytes.Buffer)
	loginCmd.SetOut(buf)
	loginCmd.SetErr(buf)
	loginCmd.SetArgs([]string{"--org-id", "piped-org"})

	execErr := loginCmd.Execute()
	if execErr != nil {
		t.Fatalf("unexpected error: %v", execErr)
	}

	// Verify config saved with piped token
	data, readErr := os.ReadFile(filepath.Join(dir, "config.yaml"))
	if readErr != nil {
		t.Fatalf("config file not created: %v", readErr)
	}

	var cfg config.Config
	if err := yaml.Unmarshal(data, &cfg); err != nil {
		t.Fatalf("failed to parse config: %v", err)
	}

	if cfg.Token != "piped-token" {
		t.Errorf("token = %q, want %q", cfg.Token, "piped-token")
	}
	if cfg.OrgType != config.OrgTypeCloud {
		t.Errorf("org_type = %q, want %q", cfg.OrgType, config.OrgTypeCloud)
	}
}

func TestDefaultOrgTypeDetector_Prefers360(t *testing.T) {
	display := "360 User"
	var seen []config.OrgType

	withValidatorFactory(t, func(auth *config.ResolvedAuth) userValidator {
		seen = append(seen, auth.OrgType)
		if auth.OrgType != config.OrgType360 {
			t.Fatalf("unexpected fallback to %q", auth.OrgType)
		}
		return &mockUserValidator{
			user: &tracker.User{Display: &display},
		}
	})

	orgType, user, err := defaultOrgTypeDetector{}.Detect(
		context.Background(),
		"test-token",
		"test-org",
	)
	if err != nil {
		t.Fatalf("Detect() returned error: %v", err)
	}
	if orgType != config.OrgType360 {
		t.Errorf("orgType = %q, want %q", orgType, config.OrgType360)
	}
	if got := api.DerefUser(user, ""); got != display {
		t.Errorf("user = %q, want %q", got, display)
	}
	if len(seen) != 1 || seen[0] != config.OrgType360 {
		t.Errorf("seen order = %v, want [%q]", seen, config.OrgType360)
	}
}

func TestDefaultOrgTypeDetector_FallsBackToCloud(t *testing.T) {
	display := "Cloud User"
	var seen []config.OrgType

	withValidatorFactory(t, func(auth *config.ResolvedAuth) userValidator {
		seen = append(seen, auth.OrgType)
		switch auth.OrgType {
		case config.OrgType360:
			return &mockUserValidator{err: errors.New("360 failed")}
		case config.OrgTypeCloud:
			return &mockUserValidator{
				user: &tracker.User{Display: &display},
			}
		default:
			t.Fatalf("unexpected org type %q", auth.OrgType)
			return nil
		}
	})

	orgType, user, err := defaultOrgTypeDetector{}.Detect(
		context.Background(),
		"test-token",
		"test-org",
	)
	if err != nil {
		t.Fatalf("Detect() returned error: %v", err)
	}
	if orgType != config.OrgTypeCloud {
		t.Errorf("orgType = %q, want %q", orgType, config.OrgTypeCloud)
	}
	if got := api.DerefUser(user, ""); got != display {
		t.Errorf("user = %q, want %q", got, display)
	}
	wantOrder := []config.OrgType{config.OrgType360, config.OrgTypeCloud}
	if len(seen) != len(wantOrder) {
		t.Fatalf("seen len = %d, want %d (%v)", len(seen), len(wantOrder), seen)
	}
	for i := range wantOrder {
		if seen[i] != wantOrder[i] {
			t.Fatalf("seen[%d] = %q, want %q (full order: %v)", i, seen[i], wantOrder[i], seen)
		}
	}
}

func TestDefaultOrgTypeDetector_AllFail(t *testing.T) {
	var seen []config.OrgType
	org360Err := errors.New("360 failed")
	cloudErr := errors.New("cloud failed")

	withValidatorFactory(t, func(auth *config.ResolvedAuth) userValidator {
		seen = append(seen, auth.OrgType)
		switch auth.OrgType {
		case config.OrgType360:
			return &mockUserValidator{err: org360Err}
		case config.OrgTypeCloud:
			return &mockUserValidator{err: cloudErr}
		default:
			t.Fatalf("unexpected org type %q", auth.OrgType)
			return nil
		}
	})

	orgType, user, err := defaultOrgTypeDetector{}.Detect(
		context.Background(),
		"test-token",
		"test-org",
	)
	if err == nil {
		t.Fatal("Detect() should return error when both org types fail")
	}
	detectErr := &orgTypeDetectionError{}
	if !errors.As(err, &detectErr) {
		t.Fatalf("error type = %T, want *orgTypeDetectionError", err)
	}
	if len(detectErr.Failures) != 2 {
		t.Fatalf("Failures len = %d, want 2", len(detectErr.Failures))
	}
	if !strings.Contains(err.Error(), "failed to detect organization type") {
		t.Errorf("error = %q, want detect failure prefix", err.Error())
	}
	if !strings.Contains(err.Error(), "360: "+org360Err.Error()) {
		t.Errorf("error = %q, want 360 failure details", err.Error())
	}
	if !strings.Contains(err.Error(), "cloud: "+cloudErr.Error()) {
		t.Errorf("error = %q, want cloud failure details", err.Error())
	}
	if orgType != "" {
		t.Errorf("orgType = %q, want empty", orgType)
	}
	if user != nil {
		t.Errorf("user = %#v, want nil", user)
	}
	wantOrder := []config.OrgType{config.OrgType360, config.OrgTypeCloud}
	if len(seen) != len(wantOrder) {
		t.Fatalf("seen len = %d, want %d (%v)", len(seen), len(wantOrder), seen)
	}
	for i := range wantOrder {
		if seen[i] != wantOrder[i] {
			t.Fatalf("seen[%d] = %q, want %q (full order: %v)", i, seen[i], wantOrder[i], seen)
		}
	}
}

func TestLogin_AutodetectTransportFailuresUseNetworkSuggestion(t *testing.T) {
	setupConfigDir(t)
	testutil.ResetOutputFlags(t)

	withMockDetector(t, &mockOrgTypeDetector{
		err: &orgTypeDetectionError{
			Failures: []orgTypeAttemptFailure{
				{
					OrgType: config.OrgType360,
					Err:     errors.New("dial tcp tracker.yandex.net: i/o timeout"),
				},
				{
					OrgType: config.OrgTypeCloud,
					Err:     errors.New("dial tcp tracker.yandex.net: i/o timeout"),
				},
			},
		},
	})

	loginCmd := newLoginCmd()
	buf := new(bytes.Buffer)
	loginCmd.SetOut(buf)
	loginCmd.SetErr(buf)
	loginCmd.SetArgs([]string{
		"--token", "test-token",
		"--org-id", "test-org",
	})

	err := loginCmd.Execute()
	if err == nil {
		t.Fatal("expected autodetect failure, got nil")
	}

	exitErr := &ytrerrors.ExitError{}
	if !errors.As(err, &exitErr) {
		t.Fatalf("error type = %T, want *ExitError", err)
	}
	if exitErr.Code != "user_error" {
		t.Errorf("Code = %q, want %q", exitErr.Code, "user_error")
	}
	if !strings.Contains(exitErr.Message, "360: dial tcp tracker.yandex.net: i/o timeout") {
		t.Errorf("Message = %q, want 360 failure details", exitErr.Message)
	}
	if !strings.Contains(exitErr.Message, "cloud: dial tcp tracker.yandex.net: i/o timeout") {
		t.Errorf("Message = %q, want cloud failure details", exitErr.Message)
	}
	if strings.Contains(exitErr.Suggestion, "--org-type") {
		t.Errorf("Suggestion = %q, should not default to org-type retry", exitErr.Suggestion)
	}
	if !strings.Contains(exitErr.Suggestion, "network connectivity") {
		t.Errorf("Suggestion = %q, want transport guidance", exitErr.Suggestion)
	}
}

func TestLogin_AutodetectSharedUserErrorUsesNeutralSuggestion(t *testing.T) {
	setupConfigDir(t)
	testutil.ResetOutputFlags(t)

	withMockDetector(t, &mockOrgTypeDetector{
		err: &orgTypeDetectionError{
			Failures: []orgTypeAttemptFailure{
				{
					OrgType: config.OrgType360,
					Err:     newTrackerError(http.StatusBadRequest, "360 invalid request"),
				},
				{
					OrgType: config.OrgTypeCloud,
					Err:     newTrackerError(http.StatusBadRequest, "cloud invalid request"),
				},
			},
		},
	})

	loginCmd := newLoginCmd()
	buf := new(bytes.Buffer)
	loginCmd.SetOut(buf)
	loginCmd.SetErr(buf)
	loginCmd.SetArgs([]string{
		"--token", "test-token",
		"--org-id", "test-org",
	})

	err := loginCmd.Execute()
	if err == nil {
		t.Fatal("expected autodetect failure, got nil")
	}

	exitErr := &ytrerrors.ExitError{}
	if !errors.As(err, &exitErr) {
		t.Fatalf("error type = %T, want *ExitError", err)
	}
	if exitErr.Code != "user_error" {
		t.Errorf("Code = %q, want %q", exitErr.Code, "user_error")
	}
	if strings.Contains(exitErr.Suggestion, "--org-type") {
		t.Errorf("Suggestion = %q, should not default to org-type retry", exitErr.Suggestion)
	}
	if !strings.Contains(exitErr.Suggestion, "reported Tracker error details") {
		t.Errorf("Suggestion = %q, want neutral guidance", exitErr.Suggestion)
	}
}

func TestLogin_AutodetectSharedServerErrorSuggestsRetry(t *testing.T) {
	setupConfigDir(t)
	testutil.ResetOutputFlags(t)

	withMockDetector(t, &mockOrgTypeDetector{
		err: &orgTypeDetectionError{
			Failures: []orgTypeAttemptFailure{
				{
					OrgType: config.OrgType360,
					Err:     newTrackerError(http.StatusInternalServerError, "360 unavailable"),
				},
				{
					OrgType: config.OrgTypeCloud,
					Err:     newTrackerError(http.StatusBadGateway, "cloud unavailable"),
				},
			},
		},
	})

	loginCmd := newLoginCmd()
	buf := new(bytes.Buffer)
	loginCmd.SetOut(buf)
	loginCmd.SetErr(buf)
	loginCmd.SetArgs([]string{
		"--token", "test-token",
		"--org-id", "test-org",
	})

	err := loginCmd.Execute()
	if err == nil {
		t.Fatal("expected autodetect failure, got nil")
	}

	exitErr := &ytrerrors.ExitError{}
	if !errors.As(err, &exitErr) {
		t.Fatalf("error type = %T, want *ExitError", err)
	}
	if exitErr.Code != "user_error" {
		t.Errorf("Code = %q, want %q", exitErr.Code, "user_error")
	}
	if strings.Contains(exitErr.Suggestion, "--org-type") {
		t.Errorf("Suggestion = %q, should not default to org-type retry", exitErr.Suggestion)
	}
	if exitErr.Suggestion != "Retry later" {
		t.Errorf("Suggestion = %q, want %q", exitErr.Suggestion, "Retry later")
	}
}

func TestLogin_AutodetectMixedSemanticCodesSuggestsOrgType(t *testing.T) {
	setupConfigDir(t)
	testutil.ResetOutputFlags(t)

	withMockDetector(t, &mockOrgTypeDetector{
		err: &orgTypeDetectionError{
			Failures: []orgTypeAttemptFailure{
				{
					OrgType: config.OrgType360,
					Err:     newTrackerError(http.StatusForbidden, "360 access denied"),
				},
				{
					OrgType: config.OrgTypeCloud,
					Err:     newTrackerError(http.StatusNotFound, "cloud not found"),
				},
			},
		},
	})

	loginCmd := newLoginCmd()
	buf := new(bytes.Buffer)
	loginCmd.SetOut(buf)
	loginCmd.SetErr(buf)
	loginCmd.SetArgs([]string{
		"--token", "test-token",
		"--org-id", "test-org",
	})

	err := loginCmd.Execute()
	if err == nil {
		t.Fatal("expected autodetect failure, got nil")
	}

	exitErr := &ytrerrors.ExitError{}
	if !errors.As(err, &exitErr) {
		t.Fatalf("error type = %T, want *ExitError", err)
	}
	if exitErr.Code != "user_error" {
		t.Errorf("Code = %q, want %q", exitErr.Code, "user_error")
	}
	if !strings.Contains(exitErr.Suggestion, "--org-type 360") {
		t.Errorf("Suggestion = %q, want org-type guidance", exitErr.Suggestion)
	}
}

func TestLogin_AutodetectSharedSemanticCodeReturnsAuthError(t *testing.T) {
	setupConfigDir(t)
	testutil.ResetOutputFlags(t)

	withMockDetector(t, &mockOrgTypeDetector{
		err: &orgTypeDetectionError{
			Failures: []orgTypeAttemptFailure{
				{
					OrgType: config.OrgType360,
					Err:     newTrackerError(http.StatusForbidden, "360 access denied"),
				},
				{
					OrgType: config.OrgTypeCloud,
					Err:     newTrackerError(http.StatusForbidden, "cloud access denied"),
				},
			},
		},
	})

	loginCmd := newLoginCmd()
	buf := new(bytes.Buffer)
	loginCmd.SetOut(buf)
	loginCmd.SetErr(buf)
	loginCmd.SetArgs([]string{
		"--token", "test-token",
		"--org-id", "test-org",
	})

	err := loginCmd.Execute()
	if err == nil {
		t.Fatal("expected autodetect failure, got nil")
	}

	exitErr := &ytrerrors.ExitError{}
	if !errors.As(err, &exitErr) {
		t.Fatalf("error type = %T, want *ExitError", err)
	}
	if exitErr.Code != "auth_error" {
		t.Errorf("Code = %q, want %q", exitErr.Code, "auth_error")
	}
	if !strings.Contains(exitErr.Message, "360 access denied") {
		t.Errorf("Message = %q, want 360 failure details", exitErr.Message)
	}
	if !strings.Contains(exitErr.Message, "cloud access denied") {
		t.Errorf("Message = %q, want cloud failure details", exitErr.Message)
	}
	if strings.Contains(exitErr.Suggestion, "--org-type") {
		t.Errorf("Suggestion = %q, should not default to org-type retry", exitErr.Suggestion)
	}
	if !strings.Contains(exitErr.Suggestion, "token and access") {
		t.Errorf("Suggestion = %q, want auth guidance", exitErr.Suggestion)
	}
}

func TestLoginNoOrgID(t *testing.T) {
	setupConfigDir(t)
	testutil.ResetOutputFlags(t)

	// Provide a token via pipe but no org-id
	r, w, err := os.Pipe()
	if err != nil {
		t.Fatalf("failed to create pipe: %v", err)
	}
	_, err = w.WriteString("test-token\n")
	if err != nil {
		t.Fatalf("failed to write to pipe: %v", err)
	}
	if err := w.Close(); err != nil {
		t.Fatalf("failed to close pipe writer: %v", err)
	}

	origStdin := stdinFile
	stdinFile = r
	t.Cleanup(func() { stdinFile = origStdin })

	loginCmd := newLoginCmd()
	buf := new(bytes.Buffer)
	loginCmd.SetOut(buf)
	loginCmd.SetErr(buf)
	loginCmd.SetArgs([]string{})

	execErr := loginCmd.Execute()
	if execErr == nil {
		t.Fatal("expected error when --org-id is missing, got nil")
	}

	if !strings.Contains(execErr.Error(), "org-id") {
		t.Errorf("error %q does not mention --org-id", execErr.Error())
	}
}

func TestLogin_InvalidOrgType(t *testing.T) {
	setupConfigDir(t)
	testutil.ResetOutputFlags(t)

	loginCmd := newLoginCmd()
	buf := new(bytes.Buffer)
	loginCmd.SetOut(buf)
	loginCmd.SetErr(buf)
	loginCmd.SetArgs([]string{
		"--token", "test-token",
		"--org-id", "test-org",
		"--org-type", "invalid",
	})

	err := loginCmd.Execute()
	if err == nil {
		t.Fatal("expected error for invalid org type, got nil")
	}

	if !strings.Contains(err.Error(), "invalid org-type") {
		t.Errorf("error %q does not mention invalid org-type", err.Error())
	}
}

func TestLogin_RegisteredAsSubcommand(t *testing.T) {
	authCmd := NewCmd()

	if authCmd.Name() != "auth" {
		t.Errorf("auth command name = %q, want %q", authCmd.Name(), "auth")
	}

	// Find login subcommand
	var loginFound bool
	for _, c := range authCmd.Commands() {
		if c.Name() == "login" {
			loginFound = true
			break
		}
	}

	if !loginFound {
		t.Error("login subcommand not found under auth")
	}
}
