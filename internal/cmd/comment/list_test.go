package comment

import (
	"bytes"
	"context"
	"encoding/json"
	"strings"
	"testing"
	"time"

	"github.com/slavkluev/go-yandex-tracker/tracker"

	"github.com/slavkluev/ytr/internal/config"
	"github.com/slavkluev/ytr/internal/output"
	"github.com/slavkluev/ytr/internal/testutil"
)

// mockCommentLister implements commentLister for testing.
type mockCommentLister struct {
	comments []*tracker.Comment
	resp     *tracker.Response
	err      error
	calls    []mockListCall
}

type mockListCall struct {
	issueKey string
	opts     *tracker.CommentListOptions
}

func (m *mockCommentLister) ListComments(
	_ context.Context,
	issueKey string,
	opts *tracker.CommentListOptions,
) ([]*tracker.Comment, *tracker.Response, error) {
	m.calls = append(m.calls, mockListCall{issueKey: issueKey, opts: opts})
	if m.err != nil {
		return nil, nil, m.err
	}
	return m.comments, m.resp, nil
}

func makeComments(ids ...string) []*tracker.Comment {
	comments := make([]*tracker.Comment, len(ids))
	for i, id := range ids {
		author := "author" + strings.Repeat("x", i)
		body := "Comment body " + strings.Repeat("text ", i)
		ts := tracker.Timestamp{Time: time.Now().Add(-time.Duration(i) * time.Hour)}
		comments[i] = &tracker.Comment{
			ID:        testutil.FlexStringPtr(id),
			Text:      testutil.StrPtr(body),
			CreatedBy: &tracker.User{Display: testutil.StrPtr(author)},
			CreatedAt: &ts,
		}
	}
	return comments
}

func setupListCmd(t *testing.T, mock *mockCommentLister, args []string) (string, error) {
	t.Helper()

	origLister := newCommentLister
	newCommentLister = func(_ *config.ResolvedAuth) commentLister {
		return mock
	}
	t.Cleanup(func() { newCommentLister = origLister })

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

func TestListTable(t *testing.T) {
	testutil.ResetOutputFlags(t)

	mock := &mockCommentLister{
		comments: makeComments("101", "202"),
		resp:     &tracker.Response{},
	}

	out, err := setupListCmd(t, mock, []string{"PROJ-123"})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	for _, want := range []string{"ID", "AUTHOR", "DATE", "BODY", "101", "202"} {
		if !strings.Contains(out, want) {
			t.Errorf("table output missing %q; got:\n%s", want, out)
		}
	}

	// Verify issue key was passed correctly.
	if len(mock.calls) == 0 {
		t.Fatal("no API calls made")
	}
	if mock.calls[0].issueKey != "PROJ-123" {
		t.Errorf("expected issueKey=PROJ-123, got %q", mock.calls[0].issueKey)
	}
}

func TestListJSON(t *testing.T) {
	testutil.ResetOutputFlags(t)
	output.JSONFields = CommentFields

	ts := tracker.Timestamp{Time: time.Date(2026, 3, 15, 10, 30, 0, 0, time.UTC)}
	mock := &mockCommentLister{
		comments: []*tracker.Comment{
			{
				ID:        testutil.FlexStringPtr("42"),
				Text:      testutil.StrPtr("Hello world"),
				CreatedBy: &tracker.User{Display: testutil.StrPtr("alice")},
				CreatedAt: &ts,
				UpdatedAt: &ts,
			},
		},
		resp: &tracker.Response{},
	}

	out, err := setupListCmd(t, mock, []string{"PROJ-1"})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	var items []map[string]any
	if err := json.Unmarshal([]byte(out), &items); err != nil {
		t.Fatalf("invalid JSON array: %v\nraw: %s", err, out)
	}

	if len(items) != 1 {
		t.Fatalf("expected 1 item, got %d", len(items))
	}

	item := items[0]
	if item["id"] != "42" {
		t.Errorf("expected id=42, got %v", item["id"])
	}
	if item["author"] != "alice" {
		t.Errorf("expected author=alice, got %v", item["author"])
	}
	if item["body"] != "Hello world" {
		t.Errorf("expected body='Hello world', got %v", item["body"])
	}
	// Verify ISO 8601 format.
	if !strings.Contains(item["createdAt"].(string), "2026-03-15") {
		t.Errorf("expected ISO date, got %v", item["createdAt"])
	}
}

func TestListQuiet(t *testing.T) {
	testutil.ResetOutputFlags(t)
	output.QuietFlag = true

	mock := &mockCommentLister{
		comments: makeComments("10", "20", "30"),
		resp:     &tracker.Response{},
	}

	out, err := setupListCmd(t, mock, []string{"PROJ-1"})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	lines := strings.Split(strings.TrimSpace(out), "\n")
	if len(lines) != 3 {
		t.Fatalf("expected 3 lines, got %d: %v", len(lines), lines)
	}
	if lines[0] != "10" || lines[1] != "20" || lines[2] != "30" {
		t.Errorf("expected 10, 20, 30 got %v", lines)
	}
}

func TestListEmpty(t *testing.T) {
	testutil.ResetOutputFlags(t)

	mock := &mockCommentLister{
		comments: []*tracker.Comment{},
		resp:     &tracker.Response{},
	}

	out, err := setupListCmd(t, mock, []string{"PROJ-1"})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if !strings.Contains(out, "No comments found") {
		t.Errorf("expected 'No comments found', got: %s", out)
	}
}

func TestListInvalidKey(t *testing.T) {
	testutil.ResetOutputFlags(t)

	mock := &mockCommentLister{}

	_, err := setupListCmd(t, mock, []string{"bad-key"})
	if err == nil {
		t.Fatal("expected error for invalid key, got nil")
	}

	if !strings.Contains(err.Error(), "invalid issue key") {
		t.Errorf("expected validation error, got: %v", err)
	}
}

func TestListNilFields(t *testing.T) {
	testutil.ResetOutputFlags(t)

	// Comment with nil fields should not panic.
	mock := &mockCommentLister{
		comments: []*tracker.Comment{
			{
				// All fields nil
			},
		},
		resp: &tracker.Response{},
	}

	out, err := setupListCmd(t, mock, []string{"PROJ-1"})
	if err != nil {
		t.Fatalf("unexpected error (panic?): %v", err)
	}

	// Should render without crashing.
	if out == "" {
		t.Error("expected some output, got empty")
	}
}

func TestListRegistered(t *testing.T) {
	cmd := NewCmd()
	if cmd.Use != "comment" {
		t.Errorf("expected Use='comment', got %q", cmd.Use)
	}

	found := false
	for _, sub := range cmd.Commands() {
		if sub.Use == "list ISSUE-KEY" {
			found = true
			break
		}
	}
	if !found {
		t.Error("'list' not registered as subcommand of 'comment'")
	}
}
