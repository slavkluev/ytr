package worklog

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

// mockWorklogDeleter implements worklogDeleter for testing.
type mockWorklogDeleter struct {
	resp         *tracker.Response
	err          error
	gotIssueKey  string
	gotWorklogID string
}

func (m *mockWorklogDeleter) DeleteWorklog(
	_ context.Context,
	issueKey, worklogID string,
) (*tracker.Response, error) {
	m.gotIssueKey = issueKey
	m.gotWorklogID = worklogID
	if m.err != nil {
		return nil, m.err
	}
	return m.resp, nil
}

func setupDeleteCmd(t *testing.T, mock *mockWorklogDeleter, args []string) (string, error) {
	t.Helper()

	origDeleter := newWorklogDeleter
	newWorklogDeleter = func(_ *config.ResolvedAuth) worklogDeleter {
		return mock
	}
	t.Cleanup(func() { newWorklogDeleter = origDeleter })

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
		mock      *mockWorklogDeleter
		args      []string
		setup     func()
		wantOut   string
		wantErr   string
		jsonCheck func(t *testing.T, out string)
	}{
		{
			name: "table output",
			mock: &mockWorklogDeleter{
				resp: &tracker.Response{},
			},
			args:    []string{"PROJ-123", "abc123"},
			wantOut: "Worklog abc123 deleted",
		},
		{
			name: "json output",
			mock: &mockWorklogDeleter{
				resp: &tracker.Response{},
			},
			args: []string{"PROJ-123", "abc123"},
			setup: func() {
				output.JSONFields = []string{"id"}
			},
			jsonCheck: func(t *testing.T, out string) {
				t.Helper()
				var result map[string]any
				if err := json.Unmarshal([]byte(out), &result); err != nil {
					t.Fatalf("invalid JSON: %v\nraw: %s", err, out)
				}
				// ID is string in JSON.
				if result["id"] != "abc123" {
					t.Errorf("expected id=abc123 (string), got %v (%T)", result["id"], result["id"])
				}
				if result["deleted"] != true {
					t.Errorf("expected deleted=true, got %v", result["deleted"])
				}
			},
		},
		{
			name: "quiet output",
			mock: &mockWorklogDeleter{
				resp: &tracker.Response{},
			},
			args: []string{"PROJ-123", "abc123"},
			setup: func() {
				output.QuietFlag = true
			},
			wantOut: "abc123",
		},
		{
			name: "jq filter",
			mock: &mockWorklogDeleter{
				resp: &tracker.Response{},
			},
			args: []string{"PROJ-123", "abc123"},
			setup: func() {
				output.JQFilter = ".id"
			},
			wantOut: "abc123",
		},
		{
			name:    "invalid issue key",
			mock:    &mockWorklogDeleter{},
			args:    []string{"bad", "abc123"},
			wantErr: "invalid issue key",
		},
		{
			name:    "empty worklog id",
			mock:    &mockWorklogDeleter{},
			args:    []string{"PROJ-1", " "},
			wantErr: "invalid worklog ID",
		},
		{
			name: "api error",
			mock: &mockWorklogDeleter{
				err: errors.New("connection refused"),
			},
			args:    []string{"PROJ-1", "abc123"},
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

	mock := &mockWorklogDeleter{
		resp: &tracker.Response{},
	}

	_, err := setupDeleteCmd(t, mock, []string{"PROJ-123", "abc123"})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if mock.gotIssueKey != "PROJ-123" {
		t.Errorf("expected issueKey=PROJ-123, got %q", mock.gotIssueKey)
	}
	// Worklog ID is string, not int.
	if mock.gotWorklogID != "abc123" {
		t.Errorf("expected worklogID=abc123 (string), got %q", mock.gotWorklogID)
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
			t.Errorf("%q not registered as subcommand of 'worklog'", name)
		}
	}
}
