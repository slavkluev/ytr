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

// mockCommentCreator implements commentCreator for testing.
type mockCommentCreator struct {
	comment *tracker.Comment
	resp    *tracker.Response
	err     error
	calls   []mockCreateCall
}

type mockCreateCall struct {
	issueKey string
	req      *tracker.CommentRequest
}

func (m *mockCommentCreator) CreateComment(
	_ context.Context,
	issueKey string,
	req *tracker.CommentRequest,
) (*tracker.Comment, *tracker.Response, error) {
	m.calls = append(m.calls, mockCreateCall{issueKey: issueKey, req: req})
	if m.err != nil {
		return nil, nil, m.err
	}
	return m.comment, m.resp, nil
}

func makeCreatedComment(id string, body string) *tracker.Comment {
	return &tracker.Comment{
		ID:        testutil.FlexStringPtr(id),
		Text:      testutil.StrPtr(body),
		CreatedBy: &tracker.User{Display: testutil.StrPtr("testuser")},
	}
}

func setupCreateCmd(t *testing.T, mock *mockCommentCreator, args []string) (string, error) {
	t.Helper()

	origCreator := newCommentCreator
	newCommentCreator = func(_ *config.ResolvedAuth) commentCreator {
		return mock
	}
	t.Cleanup(func() { newCommentCreator = origCreator })

	buf := &bytes.Buffer{}
	cmd := newCreateCmd()
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

func TestCreateWithBody(t *testing.T) {
	testutil.ResetOutputFlags(t)

	mock := &mockCommentCreator{
		comment: makeCreatedComment("555", "Hello"),
		resp:    &tracker.Response{},
	}

	out, err := setupCreateCmd(t, mock, []string{"PROJ-123", "--body", "Hello"})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	// Verify API call.
	if len(mock.calls) == 0 {
		t.Fatal("no API calls made")
	}
	if mock.calls[0].issueKey != "PROJ-123" {
		t.Errorf("expected issueKey=PROJ-123, got %q", mock.calls[0].issueKey)
	}
	if mock.calls[0].req.Text == nil || *mock.calls[0].req.Text != "Hello" {
		t.Errorf("expected Text='Hello', got %v", mock.calls[0].req.Text)
	}

	// Table output: "Comment NNN added to ISSUE-KEY"
	expected := "Comment 555 added to PROJ-123"
	if !strings.Contains(out, expected) {
		t.Errorf("expected %q in output, got: %s", expected, out)
	}
}

func TestCreateJSON(t *testing.T) {
	testutil.ResetOutputFlags(t)
	output.JSONFields = CommentFields

	mock := &mockCommentCreator{
		comment: makeCreatedComment("42", "JSON test"),
		resp:    &tracker.Response{},
	}

	out, err := setupCreateCmd(t, mock, []string{"PROJ-1", "--body", "JSON test"})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	var item map[string]any
	if err := json.Unmarshal([]byte(out), &item); err != nil {
		t.Fatalf("invalid JSON: %v\nraw: %s", err, out)
	}

	if item["id"] != "42" {
		t.Errorf("expected id=42, got %v", item["id"])
	}
	if item["body"] != "JSON test" {
		t.Errorf("expected body='JSON test', got %v", item["body"])
	}
	if item["author"] != "testuser" {
		t.Errorf("expected author='testuser', got %v", item["author"])
	}
}

func TestCreateQuiet(t *testing.T) {
	testutil.ResetOutputFlags(t)
	output.QuietFlag = true

	mock := &mockCommentCreator{
		comment: makeCreatedComment("99", "Quiet test"),
		resp:    &tracker.Response{},
	}

	out, err := setupCreateCmd(t, mock, []string{"PROJ-1", "--body", "Quiet test"})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	trimmed := strings.TrimSpace(out)
	if trimmed != "99" {
		t.Errorf("expected '99', got %q", trimmed)
	}
}

func TestCreateTable(t *testing.T) {
	testutil.ResetOutputFlags(t)

	mock := &mockCommentCreator{
		comment: makeCreatedComment("777", "Table test"),
		resp:    &tracker.Response{},
	}

	out, err := setupCreateCmd(t, mock, []string{"PROJ-5", "--body", "Table test"})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	expected := "Comment 777 added to PROJ-5"
	if !strings.Contains(out, expected) {
		t.Errorf("expected %q, got: %s", expected, out)
	}
}

func TestCreateMissingBody(t *testing.T) {
	testutil.ResetOutputFlags(t)

	mock := &mockCommentCreator{}

	_, err := setupCreateCmd(t, mock, []string{"PROJ-1"})
	if err == nil {
		t.Fatal("expected error for missing --body flag, got nil")
	}

	if !strings.Contains(err.Error(), "body") {
		t.Errorf("expected error about 'body' flag, got: %v", err)
	}
}

func TestCreateInvalidKey(t *testing.T) {
	testutil.ResetOutputFlags(t)

	mock := &mockCommentCreator{}

	_, err := setupCreateCmd(t, mock, []string{"bad-key", "--body", "text"})
	if err == nil {
		t.Fatal("expected error for invalid key, got nil")
	}

	if !strings.Contains(err.Error(), "invalid issue key") {
		t.Errorf("expected validation error, got: %v", err)
	}
}

func TestCreateControlChars(t *testing.T) {
	testutil.ResetOutputFlags(t)

	mock := &mockCommentCreator{}

	// Body with null character (control char).
	_, err := setupCreateCmd(t, mock, []string{"PROJ-1", "--body", "hello\x00world"})
	if err == nil {
		t.Fatal("expected error for control characters, got nil")
	}

	if !strings.Contains(err.Error(), "control character") {
		t.Errorf("expected control character error, got: %v", err)
	}
}

func TestCreateAPIError(t *testing.T) {
	testutil.ResetOutputFlags(t)

	mock := &mockCommentCreator{
		err: errors.New("connection refused"),
	}

	_, err := setupCreateCmd(t, mock, []string{"PROJ-1", "--body", "test"})
	if err == nil {
		t.Fatal("expected error from API, got nil")
	}

	if !strings.Contains(err.Error(), "API request failed") {
		t.Errorf("expected mapped API error, got: %v", err)
	}
}

func TestCommentRegisteredOnRoot(t *testing.T) {
	cmd := NewCmd()

	subNames := make(map[string]bool)
	for _, sub := range cmd.Commands() {
		subNames[sub.Name()] = true
	}

	if !subNames["list"] {
		t.Error("'list' not registered as subcommand of 'comment'")
	}
	if !subNames["create"] {
		t.Error("'create' not registered as subcommand of 'comment'")
	}
}
