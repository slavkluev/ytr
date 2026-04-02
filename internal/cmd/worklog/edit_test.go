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

// mockWorklogEditor implements worklogEditor for testing.
type mockWorklogEditor struct {
	worklog      *tracker.Worklog
	resp         *tracker.Response
	err          error
	gotIssueKey  string
	gotWorklogID string
	gotReq       *tracker.WorklogRequest
}

func (m *mockWorklogEditor) EditWorklog(
	_ context.Context,
	issueKey, worklogID string,
	req *tracker.WorklogRequest,
) (*tracker.Worklog, *tracker.Response, error) {
	m.gotIssueKey = issueKey
	m.gotWorklogID = worklogID
	m.gotReq = req
	if m.err != nil {
		return nil, nil, m.err
	}
	return m.worklog, m.resp, nil
}

func setupEditCmd(t *testing.T, mock *mockWorklogEditor, args []string) (string, error) {
	t.Helper()

	origEditor := newWorklogEditor
	newWorklogEditor = func(_ *config.ResolvedAuth) worklogEditor {
		return mock
	}
	t.Cleanup(func() { newWorklogEditor = origEditor })

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

func TestEditDurationOnly(t *testing.T) {
	testutil.ResetOutputFlags(t)

	mock := &mockWorklogEditor{
		worklog: makeWorklog("wl-edit1", "Updated", 120),
		resp:    &tracker.Response{},
	}

	out, err := setupEditCmd(t, mock, []string{"PROJ-123", "wl-edit1", "--duration", "PT2H"})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	expected := "Worklog wl-edit1 updated on PROJ-123"
	if !strings.Contains(out, expected) {
		t.Errorf("expected %q in output, got: %s", expected, out)
	}
}

func TestEditComment(t *testing.T) {
	testutil.ResetOutputFlags(t)

	mock := &mockWorklogEditor{
		worklog: makeWorklog("wl-c", "New comment", 60),
		resp:    &tracker.Response{},
	}

	out, err := setupEditCmd(t, mock, []string{"PROJ-1", "wl-c", "--comment", "New comment"})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if !strings.Contains(out, "Worklog wl-c updated on PROJ-1") {
		t.Errorf("unexpected output: %s", out)
	}
}

func TestEditStart(t *testing.T) {
	testutil.ResetOutputFlags(t)

	mock := &mockWorklogEditor{
		worklog: makeWorklog("wl-s", "", 60),
		resp:    &tracker.Response{},
	}

	out, err := setupEditCmd(t, mock, []string{
		"PROJ-1", "wl-s", "--start", "2026-03-30T14:00:00Z",
	})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if !strings.Contains(out, "Worklog wl-s updated on PROJ-1") {
		t.Errorf("unexpected output: %s", out)
	}
}

func TestEditAllFlags(t *testing.T) {
	testutil.ResetOutputFlags(t)

	mock := &mockWorklogEditor{
		worklog: makeWorklog("wl-all", "All updated", 180),
		resp:    &tracker.Response{},
	}

	out, err := setupEditCmd(t, mock, []string{
		"PROJ-1", "wl-all",
		"--duration", "PT3H",
		"--comment", "All updated",
		"--start", "2026-03-30T09:00:00Z",
	})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if !strings.Contains(out, "Worklog wl-all updated on PROJ-1") {
		t.Errorf("unexpected output: %s", out)
	}
}

func TestEditJSON(t *testing.T) {
	testutil.ResetOutputFlags(t)
	output.JSONFields = WorklogFields

	mock := &mockWorklogEditor{
		worklog: makeWorklog("wl-json", "JSON edit", 60),
		resp:    &tracker.Response{},
	}

	out, err := setupEditCmd(t, mock, []string{"PROJ-1", "wl-json", "--duration", "PT1H"})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	var item map[string]any
	if err := json.Unmarshal([]byte(out), &item); err != nil {
		t.Fatalf("invalid JSON: %v\nraw: %s", err, out)
	}

	if item["id"] != "wl-json" {
		t.Errorf("expected id=wl-json, got %v", item["id"])
	}
}

func TestEditQuiet(t *testing.T) {
	testutil.ResetOutputFlags(t)
	output.QuietFlag = true

	mock := &mockWorklogEditor{
		worklog: makeWorklog("wl-quiet", "", 60),
		resp:    &tracker.Response{},
	}

	out, err := setupEditCmd(t, mock, []string{"PROJ-1", "wl-quiet", "--duration", "PT1H"})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	trimmed := strings.TrimSpace(out)
	if trimmed != "wl-quiet" {
		t.Errorf("expected 'wl-quiet', got %q", trimmed)
	}
}

func TestEditFromJSON(t *testing.T) {
	testutil.ResetOutputFlags(t)

	mock := &mockWorklogEditor{
		worklog: makeWorklog("wl-fj", "From JSON", 120),
		resp:    &tracker.Response{},
	}

	out, err := setupEditCmd(t, mock, []string{
		"PROJ-1", "wl-fj", "--from-json", `{"duration":"PT2H"}`,
	})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if !strings.Contains(out, "Worklog wl-fj updated on PROJ-1") {
		t.Errorf("unexpected output: %s", out)
	}
}

func TestEditMutualExclusion(t *testing.T) {
	testutil.ResetOutputFlags(t)

	mock := &mockWorklogEditor{}

	_, err := setupEditCmd(t, mock, []string{
		"PROJ-1", "wl-1", "--from-json", `{"duration":"PT1H"}`, "--duration", "PT2H",
	})
	if err == nil {
		t.Fatal("expected mutual exclusion error, got nil")
	}

	if !strings.Contains(err.Error(), "cannot use individual flags and --from-json together") {
		t.Errorf("expected mutual exclusion error, got: %v", err)
	}
}

func TestEditNoFlags(t *testing.T) {
	testutil.ResetOutputFlags(t)

	mock := &mockWorklogEditor{}

	_, err := setupEditCmd(t, mock, []string{"PROJ-1", "wl-1"})
	if err == nil {
		t.Fatal("expected error for no flags, got nil")
	}

	if !strings.Contains(err.Error(), "at least one of") {
		t.Errorf("expected 'at least one of' error, got: %v", err)
	}
}

func TestEditInvalidIssueKey(t *testing.T) {
	testutil.ResetOutputFlags(t)

	mock := &mockWorklogEditor{}

	_, err := setupEditCmd(t, mock, []string{"bad", "wl-1", "--duration", "PT1H"})
	if err == nil {
		t.Fatal("expected error for invalid issue key, got nil")
	}

	if !strings.Contains(err.Error(), "invalid issue key") {
		t.Errorf("expected validation error, got: %v", err)
	}
}

func TestEditEmptyWorklogID(t *testing.T) {
	testutil.ResetOutputFlags(t)

	mock := &mockWorklogEditor{}

	_, err := setupEditCmd(t, mock, []string{"PROJ-1", " ", "--duration", "PT1H"})
	if err == nil {
		t.Fatal("expected error for empty worklog ID, got nil")
	}

	if !strings.Contains(err.Error(), "invalid worklog ID") {
		t.Errorf("expected worklog ID validation error, got: %v", err)
	}
}

func TestEditAPIError(t *testing.T) {
	testutil.ResetOutputFlags(t)

	mock := &mockWorklogEditor{
		err: errors.New("connection refused"),
	}

	_, err := setupEditCmd(t, mock, []string{"PROJ-1", "wl-1", "--duration", "PT1H"})
	if err == nil {
		t.Fatal("expected error from API, got nil")
	}

	if !strings.Contains(err.Error(), "API request failed") {
		t.Errorf("expected mapped API error, got: %v", err)
	}
}

func TestEditRequestCapture(t *testing.T) {
	testutil.ResetOutputFlags(t)

	mock := &mockWorklogEditor{
		worklog: makeWorklog("wl-cap", "", 60),
		resp:    &tracker.Response{},
	}

	_, err := setupEditCmd(t, mock, []string{
		"PROJ-123", "wl-cap", "--duration", "PT1H",
	})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if mock.gotIssueKey != "PROJ-123" {
		t.Errorf("expected issueKey=PROJ-123, got %q", mock.gotIssueKey)
	}
	if mock.gotWorklogID != "wl-cap" {
		t.Errorf("expected worklogID=wl-cap, got %q", mock.gotWorklogID)
	}
	if mock.gotReq == nil {
		t.Fatal("expected request to be captured")
	}
	if mock.gotReq.Duration == nil {
		t.Fatal("expected Duration to be set in request")
	}
	if mock.gotReq.Duration.Duration != time.Hour {
		t.Errorf("expected duration=1h, got %v", mock.gotReq.Duration.Duration)
	}
	// Comment should NOT be set (only --duration was changed).
	if mock.gotReq.Comment != nil {
		t.Errorf("expected Comment to be nil (not changed), got %v", *mock.gotReq.Comment)
	}
}
