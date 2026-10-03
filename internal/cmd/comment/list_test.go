package comment

import (
	"bytes"
	"context"
	"encoding/json"
	"slices"
	"strconv"
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

func setupListCmd(t *testing.T, mock *mockCommentLister, opts output.Options, args []string) (string, error) {
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
	err := cmd.ExecuteContext(output.NewContext(t.Context(), &opts))
	return buf.String(), err
}

func TestListTable(t *testing.T) {
	mock := &mockCommentLister{
		comments: makeComments("101", "202"),
		resp:     &tracker.Response{},
	}

	out, err := setupListCmd(t, mock, output.Options{}, []string{"PROJ-123"})
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

	out, err := setupListCmd(t, mock, output.Options{JSONFields: CommentFields}, []string{"PROJ-1"})
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
	mock := &mockCommentLister{
		comments: makeComments("10", "20", "30"),
		resp:     &tracker.Response{},
	}

	out, err := setupListCmd(t, mock, output.Options{Quiet: true}, []string{"PROJ-1"})
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
	mock := &mockCommentLister{
		comments: []*tracker.Comment{},
		resp:     &tracker.Response{},
	}

	out, err := setupListCmd(t, mock, output.Options{}, []string{"PROJ-1"})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if !strings.Contains(out, "No comments found") {
		t.Errorf("expected 'No comments found', got: %s", out)
	}
}

func TestListInvalidKey(t *testing.T) {
	mock := &mockCommentLister{}

	_, err := setupListCmd(t, mock, output.Options{}, []string{"bad-key"})
	if err == nil {
		t.Fatal("expected error for invalid key, got nil")
	}

	if !strings.Contains(err.Error(), "invalid issue key") {
		t.Errorf("expected validation error, got: %v", err)
	}
}

func TestListNilFields(t *testing.T) {
	// Comment with nil fields should not panic.
	mock := &mockCommentLister{
		comments: []*tracker.Comment{
			{
				// All fields nil
			},
		},
		resp: &tracker.Response{},
	}

	out, err := setupListCmd(t, mock, output.Options{}, []string{"PROJ-1"})
	if err != nil {
		t.Fatalf("unexpected error (panic?): %v", err)
	}

	// Should render without crashing.
	if out == "" {
		t.Error("expected some output, got empty")
	}
}

// cursorLister serves comments the way the Tracker endpoint pages them: up to
// opts.PerPage comments that come after the comment whose ID is opts.ID.
type cursorLister struct {
	comments []*tracker.Comment
	calls    []tracker.CommentListOptions
}

func (m *cursorLister) ListComments(
	_ context.Context,
	_ string,
	opts *tracker.CommentListOptions,
) ([]*tracker.Comment, *tracker.Response, error) {
	m.calls = append(m.calls, *opts)

	start := 0
	if opts.ID != "" {
		for i, c := range m.comments {
			if string(*c.ID) == opts.ID {
				start = i + 1
				break
			}
		}
	}
	end := min(start+opts.PerPage, len(m.comments))

	return m.comments[start:end], &tracker.Response{}, nil
}

// stuckCursorLister returns a full page on every call, and every page ends
// with the same comment ID.
type stuckCursorLister struct {
	calls int
}

func (m *stuckCursorLister) ListComments(
	_ context.Context,
	_ string,
	opts *tracker.CommentListOptions,
) ([]*tracker.Comment, *tracker.Response, error) {
	m.calls++
	if m.calls > 10 {
		return nil, &tracker.Response{}, nil
	}

	ids := make([]string, opts.PerPage)
	for i := range ids {
		ids[i] = "page-" + strconv.Itoa(m.calls) + "-" + strconv.Itoa(i)
	}
	ids[len(ids)-1] = "stuck"

	return makeComments(ids...), &tracker.Response{}, nil
}

func runCommentListQuiet(t *testing.T, lister commentLister) (string, error) {
	t.Helper()

	origLister := newCommentLister
	newCommentLister = func(_ *config.ResolvedAuth) commentLister { return lister }
	t.Cleanup(func() { newCommentLister = origLister })

	buf := &bytes.Buffer{}
	cmd := newListCmd()
	cmd.SetOut(buf)
	cmd.SetErr(buf)
	cmd.PersistentFlags().String("token", "t", "")
	cmd.PersistentFlags().String("org-id", "o", "")
	cmd.PersistentFlags().String("org-type", "360", "")
	cmd.SetArgs([]string{"PROJ-1"})

	err := cmd.ExecuteContext(output.NewContext(t.Context(), &output.Options{Quiet: true}))
	return buf.String(), err
}

func TestCommentListFollowsCursorPastFirstPage(t *testing.T) {
	ids := make([]string, 120)
	for i := range ids {
		ids[i] = strconv.Itoa(i + 1)
	}
	lister := &cursorLister{comments: makeComments(ids...)}

	out, err := runCommentListQuiet(t, lister)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	got := strings.Split(strings.TrimSpace(out), "\n")
	if !slices.Equal(got, ids) {
		t.Errorf("printed %d comment IDs, want all %d in order:\n%s", len(got), len(ids), out)
	}

	if len(lister.calls) != 2 {
		t.Fatalf("made %d list calls, want 2", len(lister.calls))
	}
	wantCursors := []string{"", "100"}
	for i, call := range lister.calls {
		if call.PerPage != 100 {
			t.Errorf("call %d: PerPage = %d, want 100", i+1, call.PerPage)
		}
		if call.ID != wantCursors[i] {
			t.Errorf("call %d: ID = %q, want %q, the last ID of the previous page", i+1, call.ID, wantCursors[i])
		}
	}
}

func TestCommentListStopsOnRepeatedCursor(t *testing.T) {
	lister := &stuckCursorLister{}

	if _, err := runCommentListQuiet(t, lister); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if lister.calls != 2 {
		t.Errorf("made %d list calls, want 2: the second page ends on the cursor it was asked for", lister.calls)
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

// namesakeComment builds a comment whose author shares a display name with
// every other namesake — only the user ID tells them apart.
func namesakeComment(commentID, userID string) *tracker.Comment {
	ts := tracker.Timestamp{Time: time.Date(2026, 3, 15, 10, 30, 0, 0, time.UTC)}
	return &tracker.Comment{
		ID:   testutil.FlexStringPtr(commentID),
		Text: testutil.StrPtr("body"),
		CreatedBy: &tracker.User{
			Display: testutil.StrPtr("Иван Петров"),
			ID:      testutil.FlexStringPtr(userID),
		},
		CreatedAt: &ts,
	}
}

func TestListNamesakesKeepDistinctAuthorIDs(t *testing.T) {
	mock := &mockCommentLister{
		comments: []*tracker.Comment{
			namesakeComment("1", "uid-a"),
			namesakeComment("2", "uid-b"),
		},
		resp: &tracker.Response{},
	}

	out, err := setupListCmd(t, mock, output.Options{JSONFields: CommentFields}, []string{"PROJ-1"})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	var items []map[string]any
	if err := json.Unmarshal([]byte(out), &items); err != nil {
		t.Fatalf("invalid JSON array: %v\nraw: %s", err, out)
	}
	if len(items) != 2 {
		t.Fatalf("expected 2 items, got %d", len(items))
	}

	if items[0]["author"] != items[1]["author"] {
		t.Fatalf("test premise broken: display names should collide, got %v and %v",
			items[0]["author"], items[1]["author"])
	}
	if items[0]["authorId"] != "uid-a" {
		t.Errorf("expected authorId=uid-a, got %v", items[0]["authorId"])
	}
	if items[1]["authorId"] != "uid-b" {
		t.Errorf("expected authorId=uid-b, got %v", items[1]["authorId"])
	}
}

func TestListAuthorIDFieldSelection(t *testing.T) {
	mock := &mockCommentLister{
		comments: []*tracker.Comment{namesakeComment("42", "uid-alice")},
		resp:     &tracker.Response{},
	}

	out, err := setupListCmd(t, mock, output.Options{JSONFields: []string{"authorId"}}, []string{"PROJ-1"})
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
	if len(items[0]) != 1 {
		t.Errorf("expected only the selected field, got %v", items[0])
	}
	if items[0]["authorId"] != "uid-alice" {
		t.Errorf("expected authorId=uid-alice, got %v", items[0]["authorId"])
	}
}

func TestListNilAuthorYieldsEmptyAuthorID(t *testing.T) {
	mock := &mockCommentLister{
		comments: []*tracker.Comment{{ID: testutil.FlexStringPtr("7")}},
		resp:     &tracker.Response{},
	}

	out, err := setupListCmd(t, mock, output.Options{JSONFields: CommentFields}, []string{"PROJ-1"})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	var items []map[string]any
	if err := json.Unmarshal([]byte(out), &items); err != nil {
		t.Fatalf("invalid JSON array: %v\nraw: %s", err, out)
	}
	// The key must be present even with no author — that is why it carries no
	// omitempty: consumers should not have to branch on a missing key.
	authorID, ok := items[0]["authorId"]
	if !ok {
		t.Fatalf("authorId key missing from output: %v", items[0])
	}
	if authorID != "" {
		t.Errorf("expected empty authorId, got %v", authorID)
	}
}

func TestListDateOffTTYIsRFC3339(t *testing.T) {
	created := tracker.Timestamp{
		Time: time.Date(2026, 9, 19, 14, 22, 31, 0, time.FixedZone("MSK", 3*60*60)),
	}
	mock := &mockCommentLister{
		comments: []*tracker.Comment{{
			ID:        testutil.FlexStringPtr("101"),
			Text:      testutil.StrPtr("body"),
			CreatedBy: &tracker.User{Display: testutil.StrPtr("john.doe")},
			CreatedAt: &created,
		}},
		resp: &tracker.Response{},
	}

	out, err := setupListCmd(t, mock, output.Options{}, []string{"PROJ-123"})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	want := "ID\tAUTHOR\tDATE\tBODY\n101\tjohn.doe\t2026-09-19T14:22:31+03:00\tbody\n"
	if out != want {
		t.Errorf("off-TTY list = %q, want %q", out, want)
	}
}

func TestListDateOnTTYIsRelative(t *testing.T) {
	mock := &mockCommentLister{
		comments: makeComments("101"),
		resp:     &tracker.Response{},
	}

	out, err := setupListCmd(t, mock, output.Options{TTY: true}, []string{"PROJ-123"})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if !strings.Contains(out, "just now") {
		t.Errorf("TTY list lost the relative date: %q", out)
	}
}

func TestListBodyOffTTYIsOneEscapedLine(t *testing.T) {
	created := tracker.Timestamp{Time: time.Date(2026, 9, 19, 14, 22, 31, 0, time.UTC)}
	mock := &mockCommentLister{
		comments: []*tracker.Comment{{
			ID:        testutil.FlexStringPtr("101"),
			Text:      testutil.StrPtr("first\nsecond\tthird"),
			CreatedBy: &tracker.User{Display: testutil.StrPtr("john.doe")},
			CreatedAt: &created,
		}},
		resp: &tracker.Response{},
	}

	out, err := setupListCmd(t, mock, output.Options{}, []string{"PROJ-123"})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if lines := strings.Count(out, "\n"); lines != 2 {
		t.Errorf("one comment must be one line, got %d lines in %q", lines, out)
	}
	if !strings.Contains(out, `first\nsecond\tthird`) {
		t.Errorf("control characters were not escaped: %q", out)
	}
}

func TestListOnTTYTruncatesALongBody(t *testing.T) {
	body := strings.Repeat("a long comment body ", 20)
	created := tracker.Timestamp{Time: time.Now()}
	mock := &mockCommentLister{
		comments: []*tracker.Comment{{
			ID:        testutil.FlexStringPtr("101"),
			Text:      testutil.StrPtr(body),
			CreatedBy: &tracker.User{Display: testutil.StrPtr("john.doe")},
			CreatedAt: &created,
		}},
		resp: &tracker.Response{},
	}

	out, err := setupListCmd(t, mock, output.Options{TTY: true, Colors: true}, []string{"PROJ-123"})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if strings.Contains(out, body) {
		t.Errorf("TTY list did not fit the body to the terminal: %q", out)
	}
	if !strings.Contains(out, "...") {
		t.Errorf("TTY list dropped the ellipsis: %q", out)
	}
}
