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
// When pages is set, each call returns the next page (empty once exhausted);
// otherwise every call returns comments.
type mockCommentLister struct {
	comments []*tracker.Comment
	pages    [][]*tracker.Comment
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
	if m.pages != nil {
		idx := len(m.calls) - 1
		if idx >= len(m.pages) {
			return nil, m.resp, nil
		}
		return m.pages[idx], m.resp, nil
	}
	return m.comments, m.resp, nil
}

// paginatedOutput mirrors the JSON envelope emitted by comment list.
type paginatedOutput struct {
	Items      []map[string]any `json:"items"`
	Pagination struct {
		Cursor  string `json:"cursor"`
		HasMore bool   `json:"hasMore"`
	} `json:"pagination"`
}

func decodePaginated(t *testing.T, out string) paginatedOutput {
	t.Helper()

	var result paginatedOutput
	if err := json.Unmarshal([]byte(out), &result); err != nil {
		t.Fatalf("invalid JSON envelope: %v\nraw: %s", err, out)
	}
	return result
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

	result := decodePaginated(t, out)

	if len(result.Items) != 1 {
		t.Fatalf("expected 1 item, got %d", len(result.Items))
	}

	item := result.Items[0]
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

func TestListPassesLimitAndCursor(t *testing.T) {
	testutil.ResetOutputFlags(t)

	mock := &mockCommentLister{
		comments: makeComments("101"),
		resp:     &tracker.Response{},
	}

	if _, err := setupListCmd(t, mock, []string{"PROJ-1", "--limit", "10", "--cursor", "abc"}); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if len(mock.calls) != 1 {
		t.Fatalf("expected 1 API call, got %d", len(mock.calls))
	}
	opts := mock.calls[0].opts
	if opts == nil {
		t.Fatal("expected non-nil CommentListOptions")
	}
	if opts.PerPage != 10 {
		t.Errorf("expected PerPage=10, got %d", opts.PerPage)
	}
	if opts.ID != "abc" {
		t.Errorf("expected ID=abc, got %q", opts.ID)
	}
}

func TestListLimitClamped(t *testing.T) {
	tests := []struct {
		name  string
		limit string
		want  int
	}{
		{"zero falls back to default", "0", defaultLimit},
		{"negative falls back to default", "-5", defaultLimit},
		{"above max is capped", "5000", maxLimit},
		{"in range is kept", "25", 25},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			testutil.ResetOutputFlags(t)

			mock := &mockCommentLister{
				comments: makeComments("1"),
				resp:     &tracker.Response{},
			}

			if _, err := setupListCmd(t, mock, []string{"PROJ-1", "--limit", tt.limit}); err != nil {
				t.Fatalf("unexpected error: %v", err)
			}

			if len(mock.calls) != 1 {
				t.Fatalf("expected 1 API call, got %d", len(mock.calls))
			}
			if got := mock.calls[0].opts.PerPage; got != tt.want {
				t.Errorf("expected PerPage=%d, got %d", tt.want, got)
			}
		})
	}
}

func TestListJSONFullPageHasMore(t *testing.T) {
	testutil.ResetOutputFlags(t)
	output.JSONFields = CommentFields

	mock := &mockCommentLister{
		comments: makeComments("101", "202"),
		resp:     &tracker.Response{},
	}

	out, err := setupListCmd(t, mock, []string{"PROJ-1", "--limit", "2"})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	result := decodePaginated(t, out)
	if !result.Pagination.HasMore {
		t.Error("expected hasMore=true for a full page")
	}
	if result.Pagination.Cursor != "202" {
		t.Errorf("expected cursor=202 (last comment ID), got %q", result.Pagination.Cursor)
	}
}

func TestListJSONPartialPageHasNoMore(t *testing.T) {
	testutil.ResetOutputFlags(t)
	output.JSONFields = CommentFields

	mock := &mockCommentLister{
		comments: makeComments("101"),
		resp:     &tracker.Response{},
	}

	out, err := setupListCmd(t, mock, []string{"PROJ-1", "--limit", "2"})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	result := decodePaginated(t, out)
	if result.Pagination.HasMore {
		t.Error("expected hasMore=false for a partial page")
	}
	if result.Pagination.Cursor != "" {
		t.Errorf("expected empty cursor, got %q", result.Pagination.Cursor)
	}
}

func TestListAllFetchesEveryPage(t *testing.T) {
	testutil.ResetOutputFlags(t)
	output.JSONFields = CommentFields

	mock := &mockCommentLister{
		pages: [][]*tracker.Comment{
			makeComments("1", "2"),
			makeComments("3"),
		},
		resp: &tracker.Response{},
	}

	out, err := setupListCmd(t, mock, []string{"PROJ-1", "--limit", "2", "--all"})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	result := decodePaginated(t, out)
	if len(result.Items) != 3 {
		t.Fatalf("expected 3 accumulated comments, got %d", len(result.Items))
	}
	if len(mock.calls) != 2 {
		t.Fatalf("expected 2 API calls (stop on partial page), got %d", len(mock.calls))
	}
	if mock.calls[0].opts.ID != "" {
		t.Errorf("expected first call without cursor, got %q", mock.calls[0].opts.ID)
	}
	if mock.calls[1].opts.ID != "2" {
		t.Errorf("expected second call to resume after ID 2, got %q", mock.calls[1].opts.ID)
	}
	if result.Pagination.HasMore || result.Pagination.Cursor != "" {
		t.Errorf("expected no pagination state with --all, got %+v", result.Pagination)
	}
}

func TestListAllStartsFromCursor(t *testing.T) {
	testutil.ResetOutputFlags(t)

	mock := &mockCommentLister{
		pages: [][]*tracker.Comment{makeComments("9")},
		resp:  &tracker.Response{},
	}

	if _, err := setupListCmd(t, mock, []string{"PROJ-1", "--limit", "2", "--all", "--cursor", "c0"}); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if len(mock.calls) == 0 {
		t.Fatal("no API calls made")
	}
	if mock.calls[0].opts.ID != "c0" {
		t.Errorf("expected --all to resume from cursor c0, got %q", mock.calls[0].opts.ID)
	}
}

func TestListAllStopsOnEmptyPage(t *testing.T) {
	testutil.ResetOutputFlags(t)
	output.JSONFields = CommentFields

	mock := &mockCommentLister{
		pages: [][]*tracker.Comment{
			makeComments("1", "2"),
			{},
		},
		resp: &tracker.Response{},
	}

	out, err := setupListCmd(t, mock, []string{"PROJ-1", "--limit", "2", "--all"})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	result := decodePaginated(t, out)
	if len(result.Items) != 2 {
		t.Fatalf("expected 2 comments, got %d", len(result.Items))
	}
	if len(mock.calls) != 2 {
		t.Errorf("expected 2 API calls, got %d", len(mock.calls))
	}
}

func TestListAllStopsOnMissingID(t *testing.T) {
	testutil.ResetOutputFlags(t)

	// A full page whose last comment carries no ID leaves no cursor to advance
	// to; paging must stop instead of refetching the same page forever.
	mock := &mockCommentLister{
		pages: [][]*tracker.Comment{
			{
				{ID: testutil.FlexStringPtr("1")},
				{},
			},
		},
		resp: &tracker.Response{},
	}

	if _, err := setupListCmd(t, mock, []string{"PROJ-1", "--limit", "2", "--all"}); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if len(mock.calls) != 1 {
		t.Errorf("expected paging to stop after 1 call, got %d", len(mock.calls))
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
