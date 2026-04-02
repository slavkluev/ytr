package user

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"strings"
	"testing"

	"github.com/slavkluev/go-yandex-tracker/tracker"

	"github.com/slavkluev/ytr/internal/config"
	"github.com/slavkluev/ytr/internal/output"
	"github.com/slavkluev/ytr/internal/testutil"
)

// mockUserMyself implements userMyself for testing.
type mockUserMyself struct {
	user *tracker.User
	resp *tracker.Response
	err  error
}

func (m *mockUserMyself) Myself(_ context.Context) (*tracker.User, *tracker.Response, error) {
	if m.err != nil {
		return nil, nil, m.err
	}
	return m.user, m.resp, nil
}

func fullUser() *tracker.User {
	return &tracker.User{
		UID:        testutil.IntPtr(12345),
		Display:    testutil.StrPtr("John Doe"),
		Login:      testutil.StrPtr("john.doe"),
		Email:      testutil.StrPtr("john@example.com"),
		FirstName:  testutil.StrPtr("John"),
		LastName:   testutil.StrPtr("Doe"),
		Dismissed:  testutil.BoolPtr(false),
		HasLicense: testutil.BoolPtr(true),
		External:   testutil.BoolPtr(false),
	}
}

func setupMyselfCmd(t *testing.T, mock *mockUserMyself, args []string) (string, error) {
	t.Helper()

	origMyself := newUserMyself
	newUserMyself = func(_ *config.ResolvedAuth) userMyself {
		return mock
	}
	t.Cleanup(func() { newUserMyself = origMyself })

	buf := &bytes.Buffer{}
	cmd := newMyselfCmd()
	cmd.SetOut(buf)
	cmd.SetErr(buf)

	// Simulate root persistent flags for auth.
	cmd.PersistentFlags().String("token", "test-token", "")
	cmd.PersistentFlags().String("org-id", "test-org", "")
	cmd.PersistentFlags().String("org-type", "360", "")

	cmd.SetArgs(args)
	err := cmd.Execute()
	return buf.String(), err
}

func TestMyself(t *testing.T) {
	tests := []struct {
		name  string
		mock  *mockUserMyself
		args  []string
		setup func(t *testing.T)
		check func(t *testing.T, out string, err error)
	}{
		{
			name: "table output",
			mock: &mockUserMyself{user: fullUser(), resp: &tracker.Response{}},
			args: []string{},
			check: func(t *testing.T, out string, err error) {
				t.Helper()
				if err != nil {
					t.Fatalf("unexpected error: %v", err)
				}
				for _, want := range []string{
					"12345", "John Doe", "john.doe", "john@example.com",
					"John", "Doe", "false", "true",
					"UID:", "Display:", "Login:", "Email:",
					"First Name:", "Last Name:", "Dismissed:", "Has License:", "External:",
				} {
					if !strings.Contains(out, want) {
						t.Errorf("output missing %q; got:\n%s", want, out)
					}
				}
			},
		},
		{
			name: "json output",
			mock: &mockUserMyself{user: fullUser(), resp: &tracker.Response{}},
			args: []string{},
			setup: func(t *testing.T) {
				t.Helper()
				output.JSONFields = UserDetailFields
			},
			check: func(t *testing.T, out string, err error) {
				t.Helper()
				if err != nil {
					t.Fatalf("unexpected error: %v", err)
				}
				var result map[string]any
				if jsonErr := json.Unmarshal([]byte(out), &result); jsonErr != nil {
					t.Fatalf("invalid JSON: %v\nraw: %s", jsonErr, out)
				}
				for _, field := range []string{"uid", "display", "login"} {
					if _, ok := result[field]; !ok {
						t.Errorf("JSON missing field %q", field)
					}
				}
				if uid, ok := result["uid"].(float64); !ok || uid != 12345 {
					t.Errorf("expected uid=12345, got %v", result["uid"])
				}
			},
		},
		{
			name: "quiet output",
			mock: &mockUserMyself{user: fullUser(), resp: &tracker.Response{}},
			args: []string{},
			setup: func(t *testing.T) {
				t.Helper()
				output.QuietFlag = true
			},
			check: func(t *testing.T, out string, err error) {
				t.Helper()
				if err != nil {
					t.Fatalf("unexpected error: %v", err)
				}
				trimmed := strings.TrimSpace(out)
				if trimmed != "12345" {
					t.Errorf("expected '12345', got %q", trimmed)
				}
			},
		},
		{
			name: "api error",
			mock: &mockUserMyself{err: errors.New("auth failed")},
			args: []string{},
			check: func(t *testing.T, _ string, err error) {
				t.Helper()
				if err == nil {
					t.Fatal("expected error, got nil")
				}
			},
		},
		{
			name: "jq filter",
			mock: &mockUserMyself{user: fullUser(), resp: &tracker.Response{}},
			args: []string{},
			setup: func(t *testing.T) {
				t.Helper()
				output.JQFilter = ".login"
			},
			check: func(t *testing.T, out string, err error) {
				t.Helper()
				if err != nil {
					t.Fatalf("unexpected error: %v", err)
				}
				trimmed := strings.TrimSpace(out)
				if !strings.Contains(trimmed, "john.doe") {
					t.Errorf("expected jq output to contain 'john.doe', got %q", trimmed)
				}
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			testutil.ResetOutputFlags(t)
			if tt.setup != nil {
				tt.setup(t)
			}
			out, err := setupMyselfCmd(t, tt.mock, tt.args)
			tt.check(t, out, err)
		})
	}
}
