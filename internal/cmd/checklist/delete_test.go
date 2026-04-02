package checklist

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

// mockChecklistDeleter implements checklistDeleter for testing.
type mockChecklistDeleter struct {
	issue       *tracker.Issue
	resp        *tracker.Response
	err         error
	gotIssueKey string
	gotItemID   string
}

func (m *mockChecklistDeleter) DeleteChecklistItem(
	_ context.Context,
	issueKey, itemID string,
) (*tracker.Issue, *tracker.Response, error) {
	m.gotIssueKey = issueKey
	m.gotItemID = itemID
	if m.err != nil {
		return nil, nil, m.err
	}
	return m.issue, m.resp, nil
}

func setupDeleteCmd(t *testing.T, mock *mockChecklistDeleter, args []string) (string, error) {
	t.Helper()

	origDeleter := newChecklistDeleter
	newChecklistDeleter = func(_ *config.ResolvedAuth) checklistDeleter {
		return mock
	}
	t.Cleanup(func() { newChecklistDeleter = origDeleter })

	buf := &bytes.Buffer{}
	cmd := newDeleteCmd()
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

func TestDelete(t *testing.T) {
	tests := []struct {
		name      string
		mock      *mockChecklistDeleter
		args      []string
		setup     func()
		wantOut   string
		wantErr   string
		jsonCheck func(t *testing.T, out string)
	}{
		{
			name: "table output",
			mock: &mockChecklistDeleter{
				issue: &tracker.Issue{},
				resp:  &tracker.Response{},
			},
			args:    []string{"PROJ-123", "item-1"},
			wantOut: "Checklist item item-1 deleted",
		},
		{
			name: "json output",
			mock: &mockChecklistDeleter{
				issue: &tracker.Issue{},
				resp:  &tracker.Response{},
			},
			args: []string{"PROJ-123", "item-1"},
			setup: func() {
				output.JSONFields = []string{"id"}
			},
			jsonCheck: func(t *testing.T, out string) {
				t.Helper()
				var result map[string]any
				if err := json.Unmarshal([]byte(out), &result); err != nil {
					t.Fatalf("invalid JSON: %v\nraw: %s", err, out)
				}
				if result["id"] != "item-1" {
					t.Errorf("expected id=item-1, got %v", result["id"])
				}
				if result["deleted"] != true {
					t.Errorf("expected deleted=true, got %v", result["deleted"])
				}
			},
		},
		{
			name: "quiet output",
			mock: &mockChecklistDeleter{
				issue: &tracker.Issue{},
				resp:  &tracker.Response{},
			},
			args: []string{"PROJ-123", "item-1"},
			setup: func() {
				output.QuietFlag = true
			},
			wantOut: "item-1",
		},
		{
			name: "jq filter",
			mock: &mockChecklistDeleter{
				issue: &tracker.Issue{},
				resp:  &tracker.Response{},
			},
			args: []string{"PROJ-123", "item-1"},
			setup: func() {
				output.JQFilter = ".id"
			},
			wantOut: "item-1",
		},
		{
			name:    "invalid issue key",
			mock:    &mockChecklistDeleter{},
			args:    []string{"bad", "item-1"},
			wantErr: "invalid issue key",
		},
		{
			name:    "empty item id",
			mock:    &mockChecklistDeleter{},
			args:    []string{"PROJ-1", " "},
			wantErr: "invalid checklist item ID",
		},
		{
			name: "api error",
			mock: &mockChecklistDeleter{
				err: errors.New("connection refused"),
			},
			args:    []string{"PROJ-1", "item-1"},
			wantErr: "API request failed",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			testutil.ResetOutputFlags(t)
			if tt.setup != nil {
				tt.setup()
			}

			out, err := setupDeleteCmd(t, tt.mock, tt.args)

			if tt.wantErr != "" {
				if err == nil {
					t.Fatalf("expected error containing %q, got nil", tt.wantErr)
				}
				if !strings.Contains(err.Error(), tt.wantErr) {
					t.Errorf("expected error containing %q, got: %v", tt.wantErr, err)
				}
				return
			}

			if err != nil {
				t.Fatalf("unexpected error: %v", err)
			}

			if tt.jsonCheck != nil {
				tt.jsonCheck(t, out)
				return
			}

			trimmed := strings.TrimSpace(out)
			wantTrimmed := strings.TrimSpace(tt.wantOut)
			if wantTrimmed != "" && !strings.Contains(trimmed, wantTrimmed) {
				t.Errorf("expected output containing %q, got: %q", wantTrimmed, trimmed)
			}
		})
	}
}

func TestDeleteRequestCapture(t *testing.T) {
	testutil.ResetOutputFlags(t)

	mock := &mockChecklistDeleter{
		issue: &tracker.Issue{},
		resp:  &tracker.Response{},
	}

	_, err := setupDeleteCmd(t, mock, []string{"PROJ-123", "item-del"})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if mock.gotIssueKey != "PROJ-123" {
		t.Errorf("expected issueKey=PROJ-123, got %q", mock.gotIssueKey)
	}
	if mock.gotItemID != "item-del" {
		t.Errorf("expected itemID=item-del, got %q", mock.gotItemID)
	}
}

func TestAllSubcommandsRegistered(t *testing.T) {
	cmd := NewCmd()

	subNames := make(map[string]bool)
	for _, sub := range cmd.Commands() {
		subNames[sub.Name()] = true
	}

	for _, name := range []string{"list", "create", "edit", "delete"} {
		if !subNames[name] {
			t.Errorf("%q not registered as subcommand of 'checklist'", name)
		}
	}
}
