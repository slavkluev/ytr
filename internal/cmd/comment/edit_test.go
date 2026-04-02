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

// mockCommentEditor implements commentEditor for testing.
type mockCommentEditor struct {
	comment      *tracker.Comment
	resp         *tracker.Response
	err          error
	gotIssueKey  string
	gotCommentID string
	gotReq       *tracker.CommentRequest
}

func (m *mockCommentEditor) EditComment(
	_ context.Context,
	issueKey string,
	commentID string,
	req *tracker.CommentRequest,
) (*tracker.Comment, *tracker.Response, error) {
	m.gotIssueKey = issueKey
	m.gotCommentID = commentID
	m.gotReq = req
	if m.err != nil {
		return nil, nil, m.err
	}
	return m.comment, m.resp, nil
}

func setupEditCmd(t *testing.T, mock *mockCommentEditor, args []string) (string, error) {
	t.Helper()

	origEditor := newCommentEditor
	newCommentEditor = func(_ *config.ResolvedAuth) commentEditor {
		return mock
	}
	t.Cleanup(func() { newCommentEditor = origEditor })

	buf := &bytes.Buffer{}
	cmd := newEditCmd()
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

func TestEdit(t *testing.T) {
	testComment := &tracker.Comment{
		ID:        testutil.FlexStringPtr("42"),
		Text:      testutil.StrPtr("updated text"),
		CreatedBy: &tracker.User{Display: testutil.StrPtr("author")},
	}

	tests := []struct {
		name      string
		mock      *mockCommentEditor
		args      []string
		wantOut   string
		wantErr   string
		jsonCheck func(t *testing.T, out string)
	}{
		{
			name: "table output with body",
			mock: &mockCommentEditor{
				comment: testComment,
				resp:    &tracker.Response{},
			},
			args:    []string{"PROJ-123", "42", "--body", "updated"},
			wantOut: "Comment 42 updated on PROJ-123",
		},
		{
			name: "json output",
			mock: &mockCommentEditor{
				comment: testComment,
				resp:    &tracker.Response{},
			},
			args: []string{"PROJ-123", "42", "--body", "updated"},
			jsonCheck: func(t *testing.T, out string) {
				t.Helper()
				var item map[string]any
				if err := json.Unmarshal([]byte(out), &item); err != nil {
					t.Fatalf("invalid JSON: %v\nraw: %s", err, out)
				}
				if item["id"] != "42" {
					t.Errorf("expected id=42, got %v", item["id"])
				}
				if item["body"] != "updated text" {
					t.Errorf("expected body='updated text', got %v", item["body"])
				}
			},
		},
		{
			name: "quiet output",
			mock: &mockCommentEditor{
				comment: testComment,
				resp:    &tracker.Response{},
			},
			args:    []string{"PROJ-123", "42", "--body", "updated"},
			wantOut: "42",
		},
		{
			name: "from-json input",
			mock: &mockCommentEditor{
				comment: testComment,
				resp:    &tracker.Response{},
			},
			args:    []string{"PROJ-123", "42", "--from-json", `{"text":"new body"}`},
			wantOut: "Comment 42 updated on PROJ-123",
		},
		{
			name:    "body and from-json conflict",
			mock:    &mockCommentEditor{},
			args:    []string{"PROJ-123", "42", "--body", "text", "--from-json", `{"text":"x"}`},
			wantErr: "cannot use --body and --from-json together",
		},
		{
			name:    "neither body nor from-json",
			mock:    &mockCommentEditor{},
			args:    []string{"PROJ-123", "42"},
			wantErr: "either --body or --from-json is required",
		},
		{
			name:    "invalid issue key",
			mock:    &mockCommentEditor{},
			args:    []string{"bad-key", "42", "--body", "x"},
			wantErr: "invalid issue key",
		},
		{
			name:    "invalid comment id",
			mock:    &mockCommentEditor{},
			args:    []string{"PROJ-1", "abc", "--body", "x"},
			wantErr: "invalid comment ID",
		},
		{
			name: "api error",
			mock: &mockCommentEditor{
				err: errors.New("connection refused"),
			},
			args:    []string{"PROJ-1", "42", "--body", "x"},
			wantErr: "API request failed",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			testutil.ResetOutputFlags(t)

			// Configure output mode for specific test cases.
			if tt.jsonCheck != nil {
				output.JSONFields = CommentFields
			}
			if tt.name == "quiet output" {
				output.QuietFlag = true
			}

			out, err := setupEditCmd(t, tt.mock, tt.args)

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

			if tt.wantOut != "" && !strings.Contains(strings.TrimSpace(out), tt.wantOut) {
				t.Errorf("expected output containing %q, got: %s", tt.wantOut, out)
			}
		})
	}
}

func TestEditRequestCapture(t *testing.T) {
	testutil.ResetOutputFlags(t)

	testComment := &tracker.Comment{
		ID:        testutil.FlexStringPtr("42"),
		Text:      testutil.StrPtr("updated"),
		CreatedBy: &tracker.User{Display: testutil.StrPtr("author")},
	}

	mock := &mockCommentEditor{
		comment: testComment,
		resp:    &tracker.Response{},
	}

	_, err := setupEditCmd(t, mock, []string{"PROJ-123", "42", "--body", "new text"})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if mock.gotIssueKey != "PROJ-123" {
		t.Errorf("expected issueKey=PROJ-123, got %q", mock.gotIssueKey)
	}
	if mock.gotCommentID != "42" {
		t.Errorf("expected commentID=42, got %q", mock.gotCommentID)
	}
	if mock.gotReq == nil || mock.gotReq.Text == nil || *mock.gotReq.Text != "new text" {
		t.Errorf("expected Text='new text', got %v", mock.gotReq)
	}
}

func TestEditFromJSONRequest(t *testing.T) {
	testutil.ResetOutputFlags(t)

	testComment := &tracker.Comment{
		ID:        testutil.FlexStringPtr("42"),
		Text:      testutil.StrPtr("from json"),
		CreatedBy: &tracker.User{Display: testutil.StrPtr("author")},
	}

	mock := &mockCommentEditor{
		comment: testComment,
		resp:    &tracker.Response{},
	}

	_, err := setupEditCmd(t, mock, []string{"PROJ-1", "42", "--from-json", `{"text":"from json"}`})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if mock.gotReq == nil || mock.gotReq.Text == nil || *mock.gotReq.Text != "from json" {
		t.Errorf("expected Text='from json', got %v", mock.gotReq)
	}
}
