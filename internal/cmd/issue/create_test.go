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

// mockCreator implements issueCreator for testing.
type mockCreator struct {
	issue *tracker.Issue
	resp  *tracker.Response
	err   error
	calls []mockCreateCall
}

type mockCreateCall struct {
	req *tracker.IssueRequest
}

func (m *mockCreator) Create(_ context.Context, req *tracker.IssueRequest) (*tracker.Issue, *tracker.Response, error) {
	m.calls = append(m.calls, mockCreateCall{req: req})
	if m.err != nil {
		return nil, nil, m.err
	}
	return m.issue, m.resp, nil
}

func makeCreatedIssue(key, summary, status string) *tracker.Issue {
	statusDisplay := status
	statusKey := strings.ToLower(status)
	return &tracker.Issue{
		Key:     testutil.StrPtr(key),
		Summary: testutil.StrPtr(summary),
		Status: &tracker.Status{
			Key:     testutil.StrPtr(statusKey),
			Display: testutil.StrPtr(statusDisplay),
		},
	}
}

func setupCreateCmd(t *testing.T, mock *mockCreator, args []string) (string, error) {
	t.Helper()

	origCreator := newCreator
	newCreator = func(_ *config.ResolvedAuth) issueCreator {
		return mock
	}
	t.Cleanup(func() { newCreator = origCreator })

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

func TestCreateWithRequiredFlags(t *testing.T) {
	testutil.ResetOutputFlags(t)
	mock := &mockCreator{
		issue: makeCreatedIssue("PROJ-1", "test issue", "Open"),
		resp:  &tracker.Response{},
	}

	_, err := setupCreateCmd(t, mock, []string{"--queue", "PROJ", "--summary", "test issue"})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if len(mock.calls) != 1 {
		t.Fatalf("expected 1 Create call, got %d", len(mock.calls))
	}

	req := mock.calls[0].req
	if req.Queue == nil || *req.Queue != "PROJ" {
		t.Errorf("expected Queue=PROJ, got %v", req.Queue)
	}
	if req.Summary == nil || *req.Summary != "test issue" {
		t.Errorf("expected Summary='test issue', got %v", req.Summary)
	}
}

func TestCreateWithAllFlags(t *testing.T) {
	testutil.ResetOutputFlags(t)
	mock := &mockCreator{
		issue: makeCreatedIssue("PROJ-1", "full issue", "Open"),
		resp:  &tracker.Response{},
	}

	_, err := setupCreateCmd(t, mock, []string{
		"--queue", "PROJ",
		"--summary", "full issue",
		"--description", "a description",
		"--type", "task",
		"--priority", "normal",
		"--assignee", "user1",
		"--parent", "PROJ-0",
	})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	req := mock.calls[0].req
	if req.Description == nil || *req.Description != "a description" {
		t.Errorf("expected Description='a description', got %v", req.Description)
	}
	if req.Type == nil || *req.Type != "task" {
		t.Errorf("expected Type='task', got %v", req.Type)
	}
	if req.Priority == nil || *req.Priority != "normal" {
		t.Errorf("expected Priority='normal', got %v", req.Priority)
	}
	if req.Assignee == nil || *req.Assignee != "user1" {
		t.Errorf("expected Assignee='user1', got %v", req.Assignee)
	}
	if req.Parent == nil || *req.Parent != "PROJ-0" {
		t.Errorf("expected Parent='PROJ-0', got %v", req.Parent)
	}
}

func TestCreateJSON(t *testing.T) {
	testutil.ResetOutputFlags(t)
	output.JSONFields = IssueDetailFields
	mock := &mockCreator{
		issue: makeCreatedIssue("PROJ-1", "test", "Open"),
		resp:  &tracker.Response{},
	}

	out, err := setupCreateCmd(t, mock, []string{"--queue", "PROJ", "--summary", "test"})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	var result map[string]any
	if err := json.Unmarshal([]byte(out), &result); err != nil {
		t.Fatalf("invalid JSON output: %v\nraw: %s", err, out)
	}

	if result["key"] != "PROJ-1" {
		t.Errorf("expected key=PROJ-1, got %v", result["key"])
	}
	if result["summary"] != "test" {
		t.Errorf("expected summary=test, got %v", result["summary"])
	}
}

func TestCreateQuiet(t *testing.T) {
	testutil.ResetOutputFlags(t)
	output.QuietFlag = true
	mock := &mockCreator{
		issue: makeCreatedIssue("PROJ-42", "test", "Open"),
		resp:  &tracker.Response{},
	}

	out, err := setupCreateCmd(t, mock, []string{"--queue", "PROJ", "--summary", "test"})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	trimmed := strings.TrimSpace(out)
	if trimmed != "PROJ-42" {
		t.Errorf("expected 'PROJ-42', got %q", trimmed)
	}
}

func TestCreateFromJSON(t *testing.T) {
	testutil.ResetOutputFlags(t)
	mock := &mockCreator{
		issue: makeCreatedIssue("PROJ-5", "from json", "Open"),
		resp:  &tracker.Response{},
	}

	_, err := setupCreateCmd(t, mock, []string{
		"--from-json", `{"queue":"PROJ","summary":"from json"}`,
	})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if len(mock.calls) != 1 {
		t.Fatalf("expected 1 Create call, got %d", len(mock.calls))
	}

	req := mock.calls[0].req
	if req.Queue == nil || *req.Queue != "PROJ" {
		t.Errorf("expected Queue=PROJ, got %v", req.Queue)
	}
	if req.Summary == nil || *req.Summary != "from json" {
		t.Errorf("expected Summary='from json', got %v", req.Summary)
	}
}

func TestCreateFromJSONMutualExclusion(t *testing.T) {
	testutil.ResetOutputFlags(t)
	mock := &mockCreator{
		issue: makeCreatedIssue("PROJ-1", "test", "Open"),
		resp:  &tracker.Response{},
	}

	_, err := setupCreateCmd(t, mock, []string{
		"--from-json", `{"queue":"PROJ","summary":"test"}`,
		"--summary", "extra",
	})
	if err == nil {
		t.Fatal("expected mutual exclusion error, got nil")
	}

	if !strings.Contains(err.Error(), "from-json OR individual flags") {
		t.Errorf("error %q should mention mutual exclusion", err.Error())
	}

	if len(mock.calls) != 0 {
		t.Error("Create should not have been called")
	}
}

func TestCreateMissingRequiredFlags(t *testing.T) {
	testutil.ResetOutputFlags(t)
	mock := &mockCreator{
		issue: makeCreatedIssue("PROJ-1", "test", "Open"),
		resp:  &tracker.Response{},
	}

	// Missing --summary.
	_, err := setupCreateCmd(t, mock, []string{"--queue", "PROJ"})
	if err == nil {
		t.Fatal("expected error for missing --summary, got nil")
	}
	if !strings.Contains(err.Error(), "summary") {
		t.Errorf("error %q should mention summary", err.Error())
	}

	// Missing --queue.
	_, err = setupCreateCmd(t, mock, []string{"--summary", "test"})
	if err == nil {
		t.Fatal("expected error for missing --queue, got nil")
	}
	if !strings.Contains(err.Error(), "queue") {
		t.Errorf("error %q should mention queue", err.Error())
	}
}

func TestCreateControlChars(t *testing.T) {
	testutil.ResetOutputFlags(t)
	mock := &mockCreator{
		issue: makeCreatedIssue("PROJ-1", "test", "Open"),
		resp:  &tracker.Response{},
	}

	_, err := setupCreateCmd(t, mock, []string{
		"--queue", "PROJ",
		"--summary", "hello\x00world",
	})
	if err == nil {
		t.Fatal("expected control character error, got nil")
	}
	if !strings.Contains(err.Error(), "U+0000") {
		t.Errorf("error %q should mention U+0000", err.Error())
	}
}

func TestCreateTable(t *testing.T) {
	testutil.ResetOutputFlags(t)
	mock := &mockCreator{
		issue: makeCreatedIssue("PROJ-7", "table test", "Open"),
		resp:  &tracker.Response{},
	}

	out, err := setupCreateCmd(t, mock, []string{"--queue", "PROJ", "--summary", "table test"})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	for _, want := range []string{"Key:", "PROJ-7", "Summary:", "table test", "Status:", "Open"} {
		if !strings.Contains(out, want) {
			t.Errorf("table output missing %q; got:\n%s", want, out)
		}
	}
}
