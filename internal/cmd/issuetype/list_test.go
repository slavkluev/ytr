package issuetype

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

// mockIssueTypeLister implements issueTypeLister for testing.
type mockIssueTypeLister struct {
	issueTypes []*tracker.IssueType
	resp       *tracker.Response
	err        error
}

func (m *mockIssueTypeLister) List(_ context.Context) ([]*tracker.IssueType, *tracker.Response, error) {
	if m.err != nil {
		return nil, nil, m.err
	}
	return m.issueTypes, m.resp, nil
}

func setupListCmd(t *testing.T, mock *mockIssueTypeLister, args []string) (string, error) {
	t.Helper()

	origLister := newIssueTypeLister
	newIssueTypeLister = func(_ *config.ResolvedAuth) issueTypeLister { return mock }
	t.Cleanup(func() { newIssueTypeLister = origLister })

	buf := &bytes.Buffer{}
	cmd := newListCmd()
	cmd.SetOut(buf)
	cmd.SetErr(buf)
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
		mock  *mockIssueTypeLister
		args  []string
		setup func()
		check func(t *testing.T, out string, err error)
	}{
		{
			name: "table output",
			mock: &mockIssueTypeLister{
				issueTypes: []*tracker.IssueType{
					{ID: testutil.FlexStringPtr("1"), Key: testutil.StrPtr("bug"), Name: testutil.StrPtr("Bug")},
					{ID: testutil.FlexStringPtr("2"), Key: testutil.StrPtr("task"), Name: testutil.StrPtr("Task")},
				},
				resp: &tracker.Response{},
			},
			args: nil,
			check: func(t *testing.T, out string, err error) {
				t.Helper()
				if err != nil {
					t.Fatalf("unexpected error: %v", err)
				}
				for _, want := range []string{"ID", "KEY", "NAME", "1", "bug", "Bug", "2", "task", "Task"} {
					if !strings.Contains(out, want) {
						t.Errorf("table output missing %q; got:\n%s", want, out)
					}
				}
			},
		},
		{
			name: "json output",
			mock: &mockIssueTypeLister{
				issueTypes: []*tracker.IssueType{
					{ID: testutil.FlexStringPtr("5"), Key: testutil.StrPtr("epic"), Name: testutil.StrPtr("Epic")},
				},
				resp: &tracker.Response{},
			},
			args:  nil,
			setup: func() { output.JSONFields = IssueTypeListFields },
			check: func(t *testing.T, out string, err error) {
				t.Helper()
				if err != nil {
					t.Fatalf("unexpected error: %v", err)
				}
				var items []map[string]any
				if err := json.Unmarshal([]byte(out), &items); err != nil {
					t.Fatalf("invalid JSON: %v\nraw: %s", err, out)
				}
				if len(items) != 1 {
					t.Fatalf("expected 1 item, got %d", len(items))
				}
				if items[0]["id"] != "5" {
					t.Errorf("expected id=5, got %v", items[0]["id"])
				}
				if items[0]["key"] != "epic" {
					t.Errorf("expected key=epic, got %v", items[0]["key"])
				}
				if items[0]["name"] != "Epic" {
					t.Errorf("expected name=Epic, got %v", items[0]["name"])
				}
			},
		},
		{
			name: "quiet output",
			mock: &mockIssueTypeLister{
				issueTypes: []*tracker.IssueType{
					{Key: testutil.StrPtr("bug")},
					{Key: testutil.StrPtr("task")},
				},
				resp: &tracker.Response{},
			},
			args:  nil,
			setup: func() { output.QuietFlag = true },
			check: func(t *testing.T, out string, err error) {
				t.Helper()
				if err != nil {
					t.Fatalf("unexpected error: %v", err)
				}
				lines := strings.Split(strings.TrimSpace(out), "\n")
				if len(lines) != 2 {
					t.Fatalf("expected 2 lines, got %d: %v", len(lines), lines)
				}
				if lines[0] != "bug" || lines[1] != "task" {
					t.Errorf("expected bug, task; got %v", lines)
				}
			},
		},
		{
			name: "empty result",
			mock: &mockIssueTypeLister{
				issueTypes: []*tracker.IssueType{},
				resp:       &tracker.Response{},
			},
			args: nil,
			check: func(t *testing.T, out string, err error) {
				t.Helper()
				if err != nil {
					t.Fatalf("unexpected error: %v", err)
				}
				if !strings.Contains(out, "No issue types found") {
					t.Errorf("expected 'No issue types found', got: %s", out)
				}
			},
		},
		{
			name: "api error",
			mock: &mockIssueTypeLister{
				err: errors.New("connection refused"),
			},
			args: nil,
			check: func(t *testing.T, _ string, err error) {
				t.Helper()
				if err == nil {
					t.Fatal("expected error, got nil")
				}
			},
		},
		{
			name: "jq filter",
			mock: &mockIssueTypeLister{
				issueTypes: []*tracker.IssueType{
					{ID: testutil.FlexStringPtr("1"), Key: testutil.StrPtr("bug"), Name: testutil.StrPtr("Bug")},
				},
				resp: &tracker.Response{},
			},
			args:  nil,
			setup: func() { output.JQFilter = ".[0].key" },
			check: func(t *testing.T, out string, err error) {
				t.Helper()
				if err != nil {
					t.Fatalf("unexpected error: %v", err)
				}
				trimmed := strings.TrimSpace(out)
				if trimmed != "bug" {
					t.Errorf("expected 'bug', got %q", trimmed)
				}
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			testutil.ResetOutputFlags(t)
			if tt.setup != nil {
				tt.setup()
			}
			out, err := setupListCmd(t, tt.mock, tt.args)
			tt.check(t, out, err)
		})
	}
}
