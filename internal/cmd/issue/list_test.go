package issue

import (
	"bytes"
	"context"
	"encoding/json"
	"strings"
	"testing"

	"github.com/slavkluev/go-yandex-tracker/tracker"

	"github.com/slavkluev/ytr/internal/config"
	"github.com/slavkluev/ytr/internal/output"
	"github.com/slavkluev/ytr/internal/testutil"
)

// mockSearcher implements issueSearcher for testing.
type mockSearcher struct {
	issues []*tracker.Issue
	resp   *tracker.Response
	err    error
	calls  []mockSearchCall
	// multiPage holds page-indexed results for --all pagination tests.
	multiPage map[int][]*tracker.Issue
}

type mockSearchCall struct {
	req  *tracker.IssueSearchRequest
	opts *tracker.IssueSearchOptions
}

func (m *mockSearcher) Search(
	_ context.Context,
	req *tracker.IssueSearchRequest,
	opts *tracker.IssueSearchOptions,
) ([]*tracker.Issue, *tracker.Response, error) {
	m.calls = append(m.calls, mockSearchCall{req: req, opts: opts})
	if m.err != nil {
		return nil, nil, m.err
	}
	if m.multiPage != nil {
		page := 1
		if opts != nil && opts.Page > 0 {
			page = opts.Page
		}
		issues := m.multiPage[page]
		resp := &tracker.Response{TotalCount: m.resp.TotalCount}
		return issues, resp, nil
	}
	return m.issues, m.resp, nil
}

func makeIssues(keys ...string) []*tracker.Issue {
	issues := make([]*tracker.Issue, len(keys))
	for i, key := range keys {
		statusKey := "open"
		statusDisplay := "Open"
		assigneeDisplay := "user" + key
		summary := "Summary for " + key
		issues[i] = &tracker.Issue{
			Key:     testutil.StrPtr(key),
			Summary: testutil.StrPtr(summary),
			Status: &tracker.Status{
				Key:     testutil.StrPtr(statusKey),
				Display: testutil.StrPtr(statusDisplay),
			},
			Assignee: &tracker.User{
				Display: testutil.StrPtr(assigneeDisplay),
			},
		}
	}
	return issues
}

func setupListCmd(t *testing.T, mock *mockSearcher, args []string) (string, error) {
	t.Helper()

	origSearcher := newSearcher
	newSearcher = func(_ *config.ResolvedAuth) issueSearcher {
		return mock
	}
	t.Cleanup(func() { newSearcher = origSearcher })

	buf := &bytes.Buffer{}
	cmd := newListCmd()
	cmd.SetOut(buf)
	cmd.SetErr(buf)

	// Simulate root persistent flags for auth.
	// When cmd has no parent, cmd.Root() returns itself.
	cmd.PersistentFlags().String("token", "test-token", "")
	cmd.PersistentFlags().String("org-id", "test-org", "")
	cmd.PersistentFlags().String("org-type", "360", "")

	cmd.SetArgs(args)
	err := cmd.Execute()
	return buf.String(), err
}

func TestListTable(t *testing.T) {
	testutil.ResetOutputFlags(t)

	mock := &mockSearcher{
		issues: makeIssues("PROJ-1", "PROJ-2"),
		resp:   &tracker.Response{TotalCount: 2},
	}

	out, err := setupListCmd(t, mock, []string{"--queue", "PROJ"})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	for _, want := range []string{"KEY", "STATUS", "ASSIGNEE", "SUMMARY", "PROJ-1", "PROJ-2", "Open"} {
		if !strings.Contains(out, want) {
			t.Errorf("table output missing %q; got:\n%s", want, out)
		}
	}
}

func TestListJSON(t *testing.T) {
	testutil.ResetOutputFlags(t)
	output.JSONFields = IssueListFields

	mock := &mockSearcher{
		issues: makeIssues("PROJ-1", "PROJ-2"),
		resp:   &tracker.Response{TotalCount: 2},
	}

	out, err := setupListCmd(t, mock, []string{"--queue", "PROJ"})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	var result map[string]any
	if err := json.Unmarshal([]byte(out), &result); err != nil {
		t.Fatalf("invalid JSON: %v\nraw: %s", err, out)
	}

	if _, ok := result["items"]; !ok {
		t.Error("JSON missing 'items' key")
	}
	pagination, ok := result["pagination"].(map[string]any)
	if !ok {
		t.Fatal("JSON missing 'pagination' object")
	}
	if _, ok := pagination["hasMore"]; !ok {
		t.Error("pagination missing 'hasMore' field")
	}
}

func TestListQuiet(t *testing.T) {
	testutil.ResetOutputFlags(t)
	output.QuietFlag = true

	mock := &mockSearcher{
		issues: makeIssues("PROJ-1", "PROJ-2"),
		resp:   &tracker.Response{TotalCount: 2},
	}

	out, err := setupListCmd(t, mock, []string{"--queue", "PROJ"})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	lines := strings.Split(strings.TrimSpace(out), "\n")
	if len(lines) != 2 {
		t.Fatalf("expected 2 lines, got %d: %v", len(lines), lines)
	}
	if lines[0] != "PROJ-1" || lines[1] != "PROJ-2" {
		t.Errorf("expected PROJ-1 and PROJ-2, got %v", lines)
	}
}

func TestListFilterQueue(t *testing.T) {
	testutil.ResetOutputFlags(t)

	mock := &mockSearcher{
		issues: []*tracker.Issue{},
		resp:   &tracker.Response{},
	}

	_, _ = setupListCmd(t, mock, []string{"--queue", "MYQUEUE"})

	if len(mock.calls) == 0 {
		t.Fatal("no search calls made")
	}
	filter := mock.calls[0].req.Filter
	if v, ok := filter["queue"]; !ok || v != "MYQUEUE" {
		t.Errorf("expected filter[queue]=MYQUEUE, got %v", filter)
	}
}

func TestListFilterStatus(t *testing.T) {
	testutil.ResetOutputFlags(t)

	mock := &mockSearcher{
		issues: []*tracker.Issue{},
		resp:   &tracker.Response{},
	}

	_, _ = setupListCmd(t, mock, []string{"--status", "open,inProgress"})

	if len(mock.calls) == 0 {
		t.Fatal("no search calls made")
	}
	filter := mock.calls[0].req.Filter
	statuses, ok := filter["status"].([]string)
	if !ok || len(statuses) != 2 {
		t.Fatalf("expected []string{open, inProgress}, got %v (type %T)", filter["status"], filter["status"])
	}
	if statuses[0] != "open" || statuses[1] != "inProgress" {
		t.Errorf("expected [open inProgress], got %v", statuses)
	}
}

func TestListFilterAssignee(t *testing.T) {
	testutil.ResetOutputFlags(t)

	mock := &mockSearcher{
		issues: []*tracker.Issue{},
		resp:   &tracker.Response{},
	}

	_, _ = setupListCmd(t, mock, []string{"--assignee", "user1"})

	if len(mock.calls) == 0 {
		t.Fatal("no search calls made")
	}
	filter := mock.calls[0].req.Filter
	if v, ok := filter["assignee"]; !ok || v != "user1" {
		t.Errorf("expected filter[assignee]=user1, got %v", filter)
	}
}

func TestListAssigneeMeShorthand(t *testing.T) {
	testutil.ResetOutputFlags(t)

	mock := &mockSearcher{
		issues: []*tracker.Issue{},
		resp:   &tracker.Response{},
	}

	_, _ = setupListCmd(t, mock, []string{"--assignee", "me"})

	if len(mock.calls) == 0 {
		t.Fatal("no search calls made")
	}
	filter := mock.calls[0].req.Filter
	if v, ok := filter["assignee"]; !ok || v != "me()" {
		t.Errorf("expected filter[assignee]=me(), got %v", filter)
	}
}

func TestListLimit(t *testing.T) {
	testutil.ResetOutputFlags(t)

	mock := &mockSearcher{
		issues: []*tracker.Issue{},
		resp:   &tracker.Response{},
	}

	_, _ = setupListCmd(t, mock, []string{"--limit", "10"})

	if len(mock.calls) == 0 {
		t.Fatal("no search calls made")
	}
	if mock.calls[0].opts.PerPage != 10 {
		t.Errorf("expected PerPage=10, got %d", mock.calls[0].opts.PerPage)
	}
}

func TestListLimitMax(t *testing.T) {
	testutil.ResetOutputFlags(t)

	mock := &mockSearcher{
		issues: []*tracker.Issue{},
		resp:   &tracker.Response{},
	}

	_, _ = setupListCmd(t, mock, []string{"--limit", "2000"})

	if len(mock.calls) == 0 {
		t.Fatal("no search calls made")
	}
	if mock.calls[0].opts.PerPage != 1000 {
		t.Errorf("expected PerPage capped at 1000, got %d", mock.calls[0].opts.PerPage)
	}
}

func TestListLimitDefault(t *testing.T) {
	testutil.ResetOutputFlags(t)

	mock := &mockSearcher{
		issues: []*tracker.Issue{},
		resp:   &tracker.Response{},
	}

	_, _ = setupListCmd(t, mock, []string{})

	if len(mock.calls) == 0 {
		t.Fatal("no search calls made")
	}
	if mock.calls[0].opts.PerPage != 50 {
		t.Errorf("expected default PerPage=50, got %d", mock.calls[0].opts.PerPage)
	}
}

func TestListCursor(t *testing.T) {
	testutil.ResetOutputFlags(t)

	mock := &mockSearcher{
		issues: []*tracker.Issue{},
		resp:   &tracker.Response{},
	}

	_, _ = setupListCmd(t, mock, []string{"--cursor", "3"})

	if len(mock.calls) == 0 {
		t.Fatal("no search calls made")
	}
	if mock.calls[0].opts.Page != 3 {
		t.Errorf("expected Page=3, got %d", mock.calls[0].opts.Page)
	}
}

func TestListInvalidCursor(t *testing.T) {
	testutil.ResetOutputFlags(t)

	mock := &mockSearcher{
		issues: []*tracker.Issue{},
		resp:   &tracker.Response{},
	}

	_, err := setupListCmd(t, mock, []string{"--cursor", "abc"})
	if err == nil {
		t.Fatal("expected error for invalid cursor, got nil")
	}
	if !strings.Contains(err.Error(), "invalid cursor") {
		t.Errorf("expected invalid cursor error, got: %v", err)
	}
	if len(mock.calls) != 0 {
		t.Errorf("expected no API calls, got %d", len(mock.calls))
	}
}

func TestListAll(t *testing.T) {
	testutil.ResetOutputFlags(t)
	output.QuietFlag = true

	// Page 1 returns 2 issues (full page), page 2 returns 1 issue (partial = done).
	mock := &mockSearcher{
		multiPage: map[int][]*tracker.Issue{
			1: makeIssues("A-1", "A-2"),
			2: makeIssues("A-3"),
		},
		resp: &tracker.Response{TotalCount: 3},
	}

	out, err := setupListCmd(t, mock, []string{"--all", "--limit", "2"})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	lines := strings.Split(strings.TrimSpace(out), "\n")
	if len(lines) != 3 {
		t.Fatalf("expected 3 keys, got %d: %v", len(lines), lines)
	}
	if len(mock.calls) != 2 {
		t.Errorf("expected 2 search calls for pagination, got %d", len(mock.calls))
	}
}

func TestListEmpty_Table(t *testing.T) {
	testutil.ResetOutputFlags(t)

	mock := &mockSearcher{
		issues: []*tracker.Issue{},
		resp:   &tracker.Response{},
	}

	out, err := setupListCmd(t, mock, []string{})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if !strings.Contains(out, "No issues found") {
		t.Errorf("expected 'No issues found', got: %s", out)
	}
}

func TestListEmpty_JSON(t *testing.T) {
	testutil.ResetOutputFlags(t)
	output.JSONFields = IssueListFields

	mock := &mockSearcher{
		issues: []*tracker.Issue{},
		resp:   &tracker.Response{},
	}

	out, err := setupListCmd(t, mock, []string{})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	var result map[string]any
	if err := json.Unmarshal([]byte(out), &result); err != nil {
		t.Fatalf("invalid JSON: %v\nraw: %s", err, out)
	}

	items, ok := result["items"].([]any)
	if !ok {
		t.Fatal("items is not an array")
	}
	if len(items) != 0 {
		t.Errorf("expected empty items, got %d", len(items))
	}

	pagination, ok := result["pagination"].(map[string]any)
	if !ok {
		t.Fatal("missing pagination")
	}
	if pagination["hasMore"] != false {
		t.Error("expected hasMore=false")
	}
}

func TestListNilFields(t *testing.T) {
	testutil.ResetOutputFlags(t)

	// Issue with nil Assignee and nil Status should not panic.
	issues := []*tracker.Issue{
		{
			Key:     testutil.StrPtr("NIL-1"),
			Summary: testutil.StrPtr("Test nil fields"),
			// Status, Assignee intentionally nil
		},
	}
	mock := &mockSearcher{
		issues: issues,
		resp:   &tracker.Response{TotalCount: 1},
	}

	out, err := setupListCmd(t, mock, []string{})
	if err != nil {
		t.Fatalf("unexpected error (panic?): %v", err)
	}

	if !strings.Contains(out, "NIL-1") {
		t.Errorf("expected NIL-1 in output, got: %s", out)
	}
	// Verify fallback "-" is used for nil fields.
	if !strings.Contains(out, "-") {
		t.Errorf("expected '-' fallback for nil fields, got: %s", out)
	}
}

func TestList_RegisteredAsSubcommand(t *testing.T) {
	cmd := NewCmd()
	if cmd.Use != "issue" {
		t.Errorf("expected Use='issue', got %q", cmd.Use)
	}

	found := false
	for _, sub := range cmd.Commands() {
		if sub.Use == "list" {
			found = true
			break
		}
	}
	if !found {
		t.Error("'list' not registered as subcommand of 'issue'")
	}
}
