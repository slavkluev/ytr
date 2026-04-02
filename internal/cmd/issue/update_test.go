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

// mockEditor implements issueEditor for testing.
type mockEditor struct {
	issue *tracker.Issue
	resp  *tracker.Response
	err   error
	calls []mockEditCall
}

type mockEditCall struct {
	issueKey string
	req      *tracker.IssueRequest
	opts     *tracker.IssueEditOptions
}

func (m *mockEditor) Edit(
	_ context.Context,
	issueKey string,
	req *tracker.IssueRequest,
	opts *tracker.IssueEditOptions,
) (*tracker.Issue, *tracker.Response, error) {
	m.calls = append(m.calls, mockEditCall{issueKey: issueKey, req: req, opts: opts})
	if m.err != nil {
		return nil, nil, m.err
	}
	return m.issue, m.resp, nil
}

func setupUpdateCmd(t *testing.T, mock *mockEditor, args []string) (string, error) {
	t.Helper()

	origEditor := newEditor
	newEditor = func(_ *config.ResolvedAuth) issueEditor {
		return mock
	}
	t.Cleanup(func() { newEditor = origEditor })

	buf := &bytes.Buffer{}
	cmd := newUpdateCmd()
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

func TestUpdateSummaryOnly(t *testing.T) {
	testutil.ResetOutputFlags(t)
	mock := &mockEditor{
		issue: makeCreatedIssue("PROJ-123", "new title", "Open"),
		resp:  &tracker.Response{},
	}

	_, err := setupUpdateCmd(t, mock, []string{"PROJ-123", "--summary", "new title"})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if len(mock.calls) != 1 {
		t.Fatalf("expected 1 Edit call, got %d", len(mock.calls))
	}

	call := mock.calls[0]
	if call.issueKey != "PROJ-123" {
		t.Errorf("expected issueKey=PROJ-123, got %q", call.issueKey)
	}
	if call.req.Summary == nil || *call.req.Summary != "new title" {
		t.Errorf("expected Summary='new title', got %v", call.req.Summary)
	}
	// Description must NOT be set when not passed.
	if call.req.Description != nil {
		t.Errorf("expected Description=nil, got %v", call.req.Description)
	}
}

func TestUpdateMultipleFields(t *testing.T) {
	testutil.ResetOutputFlags(t)
	mock := &mockEditor{
		issue: makeCreatedIssue("PROJ-123", "updated", "InProgress"),
		resp:  &tracker.Response{},
	}

	_, err := setupUpdateCmd(t, mock, []string{
		"PROJ-123",
		"--summary", "updated",
		"--priority", "critical",
	})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	req := mock.calls[0].req
	if req.Summary == nil || *req.Summary != "updated" {
		t.Errorf("expected Summary='updated', got %v", req.Summary)
	}
	if req.Priority == nil || *req.Priority != "critical" {
		t.Errorf("expected Priority='critical', got %v", req.Priority)
	}
}

func TestUpdateFromJSON(t *testing.T) {
	testutil.ResetOutputFlags(t)
	mock := &mockEditor{
		issue: makeCreatedIssue("PROJ-123", "json update", "Open"),
		resp:  &tracker.Response{},
	}

	_, err := setupUpdateCmd(t, mock, []string{
		"PROJ-123",
		"--from-json", `{"summary":"json update"}`,
	})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	req := mock.calls[0].req
	if req.Summary == nil || *req.Summary != "json update" {
		t.Errorf("expected Summary='json update', got %v", req.Summary)
	}
}

func TestUpdateFromJSONMutualExclusion(t *testing.T) {
	testutil.ResetOutputFlags(t)
	mock := &mockEditor{
		issue: makeCreatedIssue("PROJ-123", "test", "Open"),
		resp:  &tracker.Response{},
	}

	_, err := setupUpdateCmd(t, mock, []string{
		"PROJ-123",
		"--from-json", `{"summary":"test"}`,
		"--summary", "extra",
	})
	if err == nil {
		t.Fatal("expected mutual exclusion error, got nil")
	}

	if !strings.Contains(err.Error(), "from-json OR individual flags") {
		t.Errorf("error %q should mention mutual exclusion", err.Error())
	}

	if len(mock.calls) != 0 {
		t.Error("Edit should not have been called")
	}
}

func TestUpdateNoFlags(t *testing.T) {
	testutil.ResetOutputFlags(t)
	mock := &mockEditor{
		issue: makeCreatedIssue("PROJ-123", "test", "Open"),
		resp:  &tracker.Response{},
	}

	_, err := setupUpdateCmd(t, mock, []string{"PROJ-123"})
	if err == nil {
		t.Fatal("expected error for no flags, got nil")
	}

	if !strings.Contains(err.Error(), "at least one field flag or --from-json required") {
		t.Errorf("error %q should mention 'at least one field flag'", err.Error())
	}
}

func TestUpdateInvalidKey(t *testing.T) {
	testutil.ResetOutputFlags(t)
	mock := &mockEditor{
		issue: makeCreatedIssue("PROJ-123", "test", "Open"),
		resp:  &tracker.Response{},
	}

	_, err := setupUpdateCmd(t, mock, []string{"bad-key", "--summary", "test"})
	if err == nil {
		t.Fatal("expected error for invalid key, got nil")
	}

	if !strings.Contains(err.Error(), "invalid issue key") {
		t.Errorf("error %q should mention 'invalid issue key'", err.Error())
	}
}

func TestUpdateJSON(t *testing.T) {
	testutil.ResetOutputFlags(t)
	output.JSONFields = IssueDetailFields
	mock := &mockEditor{
		issue: makeCreatedIssue("PROJ-123", "updated", "Open"),
		resp:  &tracker.Response{},
	}

	out, err := setupUpdateCmd(t, mock, []string{"PROJ-123", "--summary", "updated"})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	var result map[string]any
	if err := json.Unmarshal([]byte(out), &result); err != nil {
		t.Fatalf("invalid JSON output: %v\nraw: %s", err, out)
	}

	if result["key"] != "PROJ-123" {
		t.Errorf("expected key=PROJ-123, got %v", result["key"])
	}
}

func TestUpdateQuiet(t *testing.T) {
	testutil.ResetOutputFlags(t)
	output.QuietFlag = true
	mock := &mockEditor{
		issue: makeCreatedIssue("PROJ-123", "updated", "Open"),
		resp:  &tracker.Response{},
	}

	out, err := setupUpdateCmd(t, mock, []string{"PROJ-123", "--summary", "updated"})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	trimmed := strings.TrimSpace(out)
	if trimmed != "PROJ-123" {
		t.Errorf("expected 'PROJ-123', got %q", trimmed)
	}
}

func TestUpdateTable(t *testing.T) {
	testutil.ResetOutputFlags(t)
	mock := &mockEditor{
		issue: makeCreatedIssue("PROJ-123", "updated title", "InProgress"),
		resp:  &tracker.Response{},
	}

	out, err := setupUpdateCmd(t, mock, []string{"PROJ-123", "--summary", "updated title"})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	for _, want := range []string{"Key:", "PROJ-123", "Summary:", "updated title", "Status:"} {
		if !strings.Contains(out, want) {
			t.Errorf("table output missing %q; got:\n%s", want, out)
		}
	}
}

func TestUpdate_RegisteredAsSubcommand(t *testing.T) {
	cmd := NewCmd()
	found := false
	for _, sub := range cmd.Commands() {
		if sub.Use == "update ISSUE-KEY" {
			found = true
			break
		}
	}
	if !found {
		t.Error("'update' not registered as subcommand of 'issue'")
	}
}

func TestCreate_RegisteredAsSubcommand(t *testing.T) {
	cmd := NewCmd()
	found := false
	for _, sub := range cmd.Commands() {
		if sub.Use == "create" {
			found = true
			break
		}
	}
	if !found {
		t.Error("'create' not registered as subcommand of 'issue'")
	}
}
