package worklog

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"strings"
	"testing"
	"time"

	"github.com/slavkluev/go-yandex-tracker/tracker"

	"github.com/slavkluev/ytr/internal/config"
	"github.com/slavkluev/ytr/internal/output"
	"github.com/slavkluev/ytr/internal/testutil"
)

// mockWorklogLister implements worklogLister for testing.
type mockWorklogLister struct {
	worklogs    []*tracker.Worklog
	resp        *tracker.Response
	err         error
	gotIssueKey string
}

func (m *mockWorklogLister) ListWorklogs(
	_ context.Context,
	issueKey string,
) ([]*tracker.Worklog, *tracker.Response, error) {
	m.gotIssueKey = issueKey
	if m.err != nil {
		return nil, nil, m.err
	}
	return m.worklogs, m.resp, nil
}

func setupListCmd(t *testing.T, mock *mockWorklogLister, args []string) (string, error) {
	t.Helper()

	origLister := newWorklogLister
	newWorklogLister = func(_ *config.ResolvedAuth) worklogLister {
		return mock
	}
	t.Cleanup(func() { newWorklogLister = origLister })

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

func makeWorklog(id, comment string, durationMinutes int) *tracker.Worklog {
	dur := tracker.Duration{Duration: time.Duration(durationMinutes) * time.Minute}
	ts := tracker.Timestamp{Time: time.Date(2026, 3, 30, 10, 0, 0, 0, time.UTC)}
	return &tracker.Worklog{
		ID:        testutil.FlexStringPtr(id),
		Comment:   testutil.StrPtr(comment),
		Duration:  &dur,
		Start:     &ts,
		CreatedBy: &tracker.User{Display: testutil.StrPtr("testuser")},
	}
}

func TestListTable(t *testing.T) {
	testutil.ResetOutputFlags(t)

	mock := &mockWorklogLister{
		worklogs: []*tracker.Worklog{
			makeWorklog("abc123", "Bug fix", 90),
		},
		resp: &tracker.Response{},
	}

	out, err := setupListCmd(t, mock, []string{"PROJ-123"})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	for _, want := range []string{"ID", "AUTHOR", "DURATION", "START", "abc123"} {
		if !strings.Contains(out, want) {
			t.Errorf("table output missing %q; got:\n%s", want, out)
		}
	}

	// Verify issue key was passed correctly.
	if mock.gotIssueKey != "PROJ-123" {
		t.Errorf("expected issueKey=PROJ-123, got %q", mock.gotIssueKey)
	}
}

func TestListEmpty(t *testing.T) {
	testutil.ResetOutputFlags(t)

	mock := &mockWorklogLister{
		worklogs: []*tracker.Worklog{},
		resp:     &tracker.Response{},
	}

	out, err := setupListCmd(t, mock, []string{"PROJ-1"})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if !strings.Contains(out, "No worklogs found") {
		t.Errorf("expected 'No worklogs found', got: %s", out)
	}
}

func TestListJSON(t *testing.T) {
	testutil.ResetOutputFlags(t)
	output.JSONFields = WorklogFields

	mock := &mockWorklogLister{
		worklogs: []*tracker.Worklog{
			makeWorklog("wl-42", "Code review", 120),
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
	if item["id"] != "wl-42" {
		t.Errorf("expected id=wl-42, got %v", item["id"])
	}
	if item["author"] != "testuser" {
		t.Errorf("expected author=testuser, got %v", item["author"])
	}
	if item["comment"] != "Code review" {
		t.Errorf("expected comment='Code review', got %v", item["comment"])
	}
	// Verify ISO 8601 start date.
	if !strings.Contains(item["start"].(string), "2026-03-30") {
		t.Errorf("expected ISO date in start, got %v", item["start"])
	}
}

func TestListQuiet(t *testing.T) {
	testutil.ResetOutputFlags(t)
	output.QuietFlag = true

	mock := &mockWorklogLister{
		worklogs: []*tracker.Worklog{
			makeWorklog("id1", "", 60),
			makeWorklog("id2", "", 30),
		},
		resp: &tracker.Response{},
	}

	out, err := setupListCmd(t, mock, []string{"PROJ-1"})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	lines := strings.Split(strings.TrimSpace(out), "\n")
	if len(lines) != 2 {
		t.Fatalf("expected 2 lines, got %d: %v", len(lines), lines)
	}
	if lines[0] != "id1" || lines[1] != "id2" {
		t.Errorf("expected id1, id2 got %v", lines)
	}
}

func TestListJQFilter(t *testing.T) {
	testutil.ResetOutputFlags(t)
	output.JQFilter = ".[].id"

	mock := &mockWorklogLister{
		worklogs: []*tracker.Worklog{
			makeWorklog("abc", "", 60),
		},
		resp: &tracker.Response{},
	}

	out, err := setupListCmd(t, mock, []string{"PROJ-1"})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	trimmed := strings.TrimSpace(out)
	if !strings.Contains(trimmed, "abc") {
		t.Errorf("expected jq result containing 'abc', got: %s", trimmed)
	}
}

func TestListInvalidKey(t *testing.T) {
	testutil.ResetOutputFlags(t)

	mock := &mockWorklogLister{}

	_, err := setupListCmd(t, mock, []string{"bad-key"})
	if err == nil {
		t.Fatal("expected error for invalid key, got nil")
	}

	if !strings.Contains(err.Error(), "invalid issue key") {
		t.Errorf("expected validation error, got: %v", err)
	}
}

func TestListAPIError(t *testing.T) {
	testutil.ResetOutputFlags(t)

	mock := &mockWorklogLister{
		err: errors.New("connection refused"),
	}

	_, err := setupListCmd(t, mock, []string{"PROJ-1"})
	if err == nil {
		t.Fatal("expected error from API, got nil")
	}

	if !strings.Contains(err.Error(), "API request failed") {
		t.Errorf("expected mapped API error, got: %v", err)
	}
}
