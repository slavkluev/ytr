package link

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

// mockLinkDeleter implements linkDeleter for testing.
type mockLinkDeleter struct {
	resp        *tracker.Response
	err         error
	gotIssueKey string
	gotLinkID   string
}

func (m *mockLinkDeleter) DeleteLink(
	_ context.Context,
	issueKey, linkID string,
) (*tracker.Response, error) {
	m.gotIssueKey = issueKey
	m.gotLinkID = linkID
	if m.err != nil {
		return nil, m.err
	}
	return m.resp, nil
}

func setupDeleteCmd(t *testing.T, mock *mockLinkDeleter, args []string) (string, error) {
	t.Helper()

	origDeleter := newLinkDeleter
	newLinkDeleter = func(_ *config.ResolvedAuth) linkDeleter {
		return mock
	}
	t.Cleanup(func() { newLinkDeleter = origDeleter })

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
		mock      *mockLinkDeleter
		args      []string
		setup     func()
		wantOut   string
		wantErr   string
		jsonCheck func(t *testing.T, out string)
	}{
		{
			name: "table output",
			mock: &mockLinkDeleter{
				resp: &tracker.Response{},
			},
			args:    []string{"PROJ-123", "456"},
			wantOut: "Link 456 deleted",
		},
		{
			name: "json output",
			mock: &mockLinkDeleter{
				resp: &tracker.Response{},
			},
			args: []string{"PROJ-123", "456"},
			setup: func() {
				output.JSONFields = []string{"id"}
			},
			jsonCheck: func(t *testing.T, out string) {
				t.Helper()
				var result map[string]any
				if err := json.Unmarshal([]byte(out), &result); err != nil {
					t.Fatalf("invalid JSON: %v\nraw: %s", err, out)
				}
				// ID is string in JSON, not number.
				if result["id"] != "456" {
					t.Errorf("expected id=456 (string), got %v (%T)", result["id"], result["id"])
				}
				if result["deleted"] != true {
					t.Errorf("expected deleted=true, got %v", result["deleted"])
				}
			},
		},
		{
			name: "quiet output",
			mock: &mockLinkDeleter{
				resp: &tracker.Response{},
			},
			args: []string{"PROJ-123", "456"},
			setup: func() {
				output.QuietFlag = true
			},
			wantOut: "456",
		},
		{
			name: "jq filter",
			mock: &mockLinkDeleter{
				resp: &tracker.Response{},
			},
			args: []string{"PROJ-123", "456"},
			setup: func() {
				output.JQFilter = ".id"
			},
			wantOut: "456",
		},
		{
			name:    "invalid issue key",
			mock:    &mockLinkDeleter{},
			args:    []string{"bad", "456"},
			wantErr: "invalid issue key",
		},
		{
			name:    "empty link id",
			mock:    &mockLinkDeleter{},
			args:    []string{"PROJ-1", " "},
			wantErr: "invalid link ID",
		},
		{
			name: "api error",
			mock: &mockLinkDeleter{
				err: errors.New("connection refused"),
			},
			args:    []string{"PROJ-1", "456"},
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

	mock := &mockLinkDeleter{
		resp: &tracker.Response{},
	}

	_, err := setupDeleteCmd(t, mock, []string{"PROJ-123", "456"})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if mock.gotIssueKey != "PROJ-123" {
		t.Errorf("expected issueKey=PROJ-123, got %q", mock.gotIssueKey)
	}
	// Link ID is string, not int.
	if mock.gotLinkID != "456" {
		t.Errorf("expected linkID=456 (string), got %q", mock.gotLinkID)
	}
}

func TestDeleteRegistered(t *testing.T) {
	cmd := NewCmd()

	found := false
	for _, sub := range cmd.Commands() {
		if sub.Name() == "delete" {
			found = true
			break
		}
	}
	if !found {
		t.Error("'delete' not registered as subcommand of 'link'")
	}
}

func TestAllSubcommandsRegistered(t *testing.T) {
	cmd := NewCmd()

	subNames := make(map[string]bool)
	for _, sub := range cmd.Commands() {
		subNames[sub.Name()] = true
	}

	for _, name := range []string{"list", "create", "delete"} {
		if !subNames[name] {
			t.Errorf("%q not registered as subcommand of 'link'", name)
		}
	}
}
