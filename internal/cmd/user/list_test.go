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

// mockUserLister implements userLister for testing.
type mockUserLister struct {
	users []*tracker.User
	resp  *tracker.Response
	err   error
	calls []mockListCall
	// multiPage holds page-indexed results for --all pagination tests.
	multiPage map[int][]*tracker.User
}

type mockListCall struct {
	opts *tracker.UserListOptions
}

func (m *mockUserLister) List(
	_ context.Context,
	opts *tracker.UserListOptions,
) ([]*tracker.User, *tracker.Response, error) {
	m.calls = append(m.calls, mockListCall{opts: opts})
	if m.err != nil {
		return nil, nil, m.err
	}
	if m.multiPage != nil {
		page := 1
		if opts != nil && opts.Page > 0 {
			page = opts.Page
		}
		users := m.multiPage[page]
		resp := &tracker.Response{TotalCount: m.resp.TotalCount}
		return users, resp, nil
	}
	return m.users, m.resp, nil
}

func makeUsers() []*tracker.User {
	return []*tracker.User{
		{
			UID:     testutil.IntPtr(100),
			Display: testutil.StrPtr("Alice"),
			Login:   testutil.StrPtr("alice"),
			Email:   testutil.StrPtr("alice@example.com"),
		},
		{
			UID:     testutil.IntPtr(200),
			Display: testutil.StrPtr("Bob"),
			Login:   testutil.StrPtr("bob"),
			Email:   testutil.StrPtr("bob@example.com"),
		},
	}
}

func setupListCmd(t *testing.T, mock *mockUserLister, args []string) (string, error) {
	t.Helper()

	origLister := newUserLister
	newUserLister = func(_ *config.ResolvedAuth) userLister {
		return mock
	}
	t.Cleanup(func() { newUserLister = origLister })

	buf := &bytes.Buffer{}
	cmd := newListCmd()
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

func TestList(t *testing.T) {
	tests := []struct {
		name  string
		mock  *mockUserLister
		args  []string
		setup func(t *testing.T)
		check func(t *testing.T, out string, err error, mock *mockUserLister)
	}{
		{
			name: "table output",
			mock: &mockUserLister{
				users: makeUsers(),
				resp:  &tracker.Response{TotalCount: 2},
			},
			args: []string{},
			check: func(t *testing.T, out string, err error, _ *mockUserLister) {
				t.Helper()
				if err != nil {
					t.Fatalf("unexpected error: %v", err)
				}
				for _, want := range []string{
					"UID", "DISPLAY", "LOGIN", "EMAIL",
					"100", "Alice", "alice", "alice@example.com",
					"200", "Bob", "bob", "bob@example.com",
				} {
					if !strings.Contains(out, want) {
						t.Errorf("table output missing %q; got:\n%s", want, out)
					}
				}
			},
		},
		{
			name: "json output",
			mock: &mockUserLister{
				users: makeUsers(),
				resp:  &tracker.Response{TotalCount: 2},
			},
			args: []string{},
			setup: func(t *testing.T) {
				t.Helper()
				output.JSONFields = UserListFields
			},
			check: func(t *testing.T, out string, err error, _ *mockUserLister) {
				t.Helper()
				if err != nil {
					t.Fatalf("unexpected error: %v", err)
				}
				var result map[string]any
				if jsonErr := json.Unmarshal([]byte(out), &result); jsonErr != nil {
					t.Fatalf("invalid JSON: %v\nraw: %s", jsonErr, out)
				}
				items, ok := result["items"].([]any)
				if !ok {
					t.Fatal("JSON missing 'items' array")
				}
				if len(items) != 2 {
					t.Errorf("expected 2 items, got %d", len(items))
				}
				pagination, ok := result["pagination"].(map[string]any)
				if !ok {
					t.Fatal("JSON missing 'pagination' object")
				}
				if _, ok := pagination["hasMore"]; !ok {
					t.Error("pagination missing 'hasMore' field")
				}
			},
		},
		{
			name: "quiet output",
			mock: &mockUserLister{
				users: makeUsers(),
				resp:  &tracker.Response{TotalCount: 2},
			},
			args: []string{},
			setup: func(t *testing.T) {
				t.Helper()
				output.QuietFlag = true
			},
			check: func(t *testing.T, out string, err error, _ *mockUserLister) {
				t.Helper()
				if err != nil {
					t.Fatalf("unexpected error: %v", err)
				}
				lines := strings.Split(strings.TrimSpace(out), "\n")
				if len(lines) != 2 {
					t.Fatalf("expected 2 lines, got %d: %v", len(lines), lines)
				}
				if lines[0] != "100" || lines[1] != "200" {
					t.Errorf("expected 100 and 200, got %v", lines)
				}
			},
		},
		{
			name: "empty result",
			mock: &mockUserLister{
				users: []*tracker.User{},
				resp:  &tracker.Response{},
			},
			args: []string{},
			check: func(t *testing.T, out string, err error, _ *mockUserLister) {
				t.Helper()
				if err != nil {
					t.Fatalf("unexpected error: %v", err)
				}
				if !strings.Contains(out, "No users found") {
					t.Errorf("expected 'No users found', got: %s", out)
				}
			},
		},
		{
			name: "api error",
			mock: &mockUserLister{
				err: errors.New("api error"),
			},
			args: []string{},
			check: func(t *testing.T, _ string, err error, _ *mockUserLister) {
				t.Helper()
				if err == nil {
					t.Fatal("expected error, got nil")
				}
			},
		},
		{
			name: "pagination cursor",
			mock: &mockUserLister{
				users: makeUsers(),
				resp:  &tracker.Response{TotalCount: 10},
			},
			args: []string{"--cursor", "2"},
			check: func(t *testing.T, _ string, err error, m *mockUserLister) {
				t.Helper()
				if err != nil {
					t.Fatalf("unexpected error: %v", err)
				}
				if len(m.calls) == 0 {
					t.Fatal("no list calls made")
				}
				if m.calls[0].opts.Page != 2 {
					t.Errorf("expected Page=2, got %d", m.calls[0].opts.Page)
				}
			},
		},
		{
			name: "jq filter",
			mock: &mockUserLister{
				users: makeUsers(),
				resp:  &tracker.Response{TotalCount: 2},
			},
			args: []string{},
			setup: func(t *testing.T) {
				t.Helper()
				output.JQFilter = ".items[0].login"
			},
			check: func(t *testing.T, out string, err error, _ *mockUserLister) {
				t.Helper()
				if err != nil {
					t.Fatalf("unexpected error: %v", err)
				}
				trimmed := strings.TrimSpace(out)
				if !strings.Contains(trimmed, "alice") {
					t.Errorf("expected jq output to contain 'alice', got %q", trimmed)
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
			out, err := setupListCmd(t, tt.mock, tt.args)
			tt.check(t, out, err, tt.mock)
		})
	}
}

func TestListInvalidCursor(t *testing.T) {
	testutil.ResetOutputFlags(t)

	mock := &mockUserLister{
		users: makeUsers(),
		resp:  &tracker.Response{TotalCount: 2},
	}

	_, err := setupListCmd(t, mock, []string{"--cursor", "abc"})
	if err == nil {
		t.Fatal("expected error for invalid cursor, got nil")
	}
	if !strings.Contains(err.Error(), "invalid cursor") {
		t.Errorf("expected invalid cursor error, got: %v", err)
	}
	if len(mock.calls) != 0 {
		t.Errorf("expected no API calls, got %d", len(mock.calls))
	}
}
