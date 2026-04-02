package comment

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

// mockCommentDeleter implements commentDeleter for testing.
type mockCommentDeleter struct {
	resp         *tracker.Response
	err          error
	gotIssueKey  string
	gotCommentID string
}

func (m *mockCommentDeleter) DeleteComment(
	_ context.Context,
	issueKey string,
	commentID string,
) (*tracker.Response, error) {
	m.gotIssueKey = issueKey
	m.gotCommentID = commentID
	if m.err != nil {
		return nil, m.err
	}
	return m.resp, nil
}

func setupDeleteCmd(t *testing.T, mock *mockCommentDeleter, args []string) (string, error) {
	t.Helper()

	origDeleter := newCommentDeleter
	newCommentDeleter = func(_ *config.ResolvedAuth) commentDeleter {
		return mock
	}
	t.Cleanup(func() { newCommentDeleter = origDeleter })

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
		mock      *mockCommentDeleter
		args      []string
		wantOut   string
		wantErr   string
		jsonCheck func(t *testing.T, out string)
	}{
		{
			name: "table output",
			mock: &mockCommentDeleter{
				resp: &tracker.Response{},
			},
			args:    []string{"PROJ-123", "42"},
			wantOut: "Comment 42 deleted\n",
		},
		{
			name: "json output",
			mock: &mockCommentDeleter{
				resp: &tracker.Response{},
			},
			args: []string{"PROJ-123", "42"},
			jsonCheck: func(t *testing.T, out string) {
				t.Helper()
				var result map[string]any
				if err := json.Unmarshal([]byte(out), &result); err != nil {
					t.Fatalf("invalid JSON: %v\nraw: %s", err, out)
				}
				if result["id"] != "42" {
					t.Errorf("expected id=42, got %v", result["id"])
				}
				if result["deleted"] != true {
					t.Errorf("expected deleted=true, got %v", result["deleted"])
				}
			},
		},
		{
			name: "quiet output",
			mock: &mockCommentDeleter{
				resp: &tracker.Response{},
			},
			args:    []string{"PROJ-123", "42"},
			wantOut: "42",
		},
		{
			name: "jq filter",
			mock: &mockCommentDeleter{
				resp: &tracker.Response{},
			},
			args:    []string{"PROJ-123", "42"},
			wantOut: "42",
		},
		{
			name:    "invalid issue key",
			mock:    &mockCommentDeleter{},
			args:    []string{"bad", "42"},
			wantErr: "invalid issue key",
		},
		{
			name:    "invalid comment id",
			mock:    &mockCommentDeleter{},
			args:    []string{"PROJ-1", "abc"},
			wantErr: "invalid comment ID",
		},
		{
			name: "api error",
			mock: &mockCommentDeleter{
				err: errors.New("connection refused"),
			},
			args:    []string{"PROJ-1", "42"},
			wantErr: "API request failed",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			testutil.ResetOutputFlags(t)

			// Configure output mode for specific test cases.
			if tt.jsonCheck != nil {
				output.JSONFields = []string{"id"}
			}
			if tt.name == "quiet output" {
				output.QuietFlag = true
			}
			if tt.name == "jq filter" {
				output.JQFilter = ".id"
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

	mock := &mockCommentDeleter{
		resp: &tracker.Response{},
	}

	_, err := setupDeleteCmd(t, mock, []string{"PROJ-123", "42"})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if mock.gotIssueKey != "PROJ-123" {
		t.Errorf("expected issueKey=PROJ-123, got %q", mock.gotIssueKey)
	}
	if mock.gotCommentID != "42" {
		t.Errorf("expected commentID=42, got %q", mock.gotCommentID)
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
		t.Error("'delete' not registered as subcommand of 'comment'")
	}
}

func TestEditRegistered(t *testing.T) {
	cmd := NewCmd()

	found := false
	for _, sub := range cmd.Commands() {
		if sub.Name() == "edit" {
			found = true
			break
		}
	}
	if !found {
		t.Error("'edit' not registered as subcommand of 'comment'")
	}
}
