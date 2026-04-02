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

// mockUserGetter implements userGetter for testing.
type mockUserGetter struct {
	user *tracker.User
	resp *tracker.Response
	err  error
}

func (m *mockUserGetter) Get(_ context.Context, _ string) (*tracker.User, *tracker.Response, error) {
	if m.err != nil {
		return nil, nil, m.err
	}
	return m.user, m.resp, nil
}

func setupGetCmd(t *testing.T, mock *mockUserGetter, args []string) (string, error) {
	t.Helper()

	origGetter := newUserGetter
	newUserGetter = func(_ *config.ResolvedAuth) userGetter {
		return mock
	}
	t.Cleanup(func() { newUserGetter = origGetter })

	buf := &bytes.Buffer{}
	cmd := newGetCmd()
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

func TestGet(t *testing.T) {
	tests := []struct {
		name  string
		mock  *mockUserGetter
		args  []string
		setup func(t *testing.T)
		check func(t *testing.T, out string, err error)
	}{
		{
			name: "table output",
			mock: &mockUserGetter{user: fullUser(), resp: &tracker.Response{}},
			args: []string{"user123"},
			check: func(t *testing.T, out string, err error) {
				t.Helper()
				if err != nil {
					t.Fatalf("unexpected error: %v", err)
				}
				for _, want := range []string{
					"12345", "John Doe", "john.doe", "john@example.com",
					"John", "Doe", "false", "true",
					"UID:", "Display:", "Login:", "Email:",
				} {
					if !strings.Contains(out, want) {
						t.Errorf("output missing %q; got:\n%s", want, out)
					}
				}
			},
		},
		{
			name: "json output",
			mock: &mockUserGetter{user: fullUser(), resp: &tracker.Response{}},
			args: []string{"user123"},
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
				for _, field := range []string{"uid", "display", "login", "email"} {
					if _, ok := result[field]; !ok {
						t.Errorf("JSON missing field %q", field)
					}
				}
			},
		},
		{
			name: "quiet output",
			mock: &mockUserGetter{user: fullUser(), resp: &tracker.Response{}},
			args: []string{"user123"},
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
			mock: &mockUserGetter{err: errors.New("api error")},
			args: []string{"user123"},
			check: func(t *testing.T, _ string, err error) {
				t.Helper()
				if err == nil {
					t.Fatal("expected error, got nil")
				}
			},
		},
		{
			name: "user not found",
			mock: &mockUserGetter{err: errors.New("not found")},
			args: []string{"unknown-user"},
			check: func(t *testing.T, _ string, err error) {
				t.Helper()
				if err == nil {
					t.Fatal("expected error, got nil")
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
			out, err := setupGetCmd(t, tt.mock, tt.args)
			tt.check(t, out, err)
		})
	}
}
