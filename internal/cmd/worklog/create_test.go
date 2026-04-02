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

// mockWorklogCreator implements worklogCreator for testing.
type mockWorklogCreator struct {
	worklog     *tracker.Worklog
	resp        *tracker.Response
	err         error
	gotIssueKey string
	gotReq      *tracker.WorklogRequest
}

func (m *mockWorklogCreator) CreateWorklog(
	_ context.Context,
	issueKey string,
	req *tracker.WorklogRequest,
) (*tracker.Worklog, *tracker.Response, error) {
	m.gotIssueKey = issueKey
	m.gotReq = req
	if m.err != nil {
		return nil, nil, m.err
	}
	return m.worklog, m.resp, nil
}

func makeCreatedWorklog(id, comment string) *tracker.Worklog {
	dur := tracker.Duration{Duration: 90 * time.Minute}
	ts := tracker.Timestamp{Time: time.Date(2026, 3, 30, 10, 0, 0, 0, time.UTC)}
	return &tracker.Worklog{
		ID:        testutil.FlexStringPtr(id),
		Comment:   testutil.StrPtr(comment),
		Duration:  &dur,
		Start:     &ts,
		CreatedBy: &tracker.User{Display: testutil.StrPtr("testuser")},
	}
}

func setupCreateCmd(t *testing.T, mock *mockWorklogCreator, args []string) (string, error) {
	t.Helper()

	origCreator := newWorklogCreator
	newWorklogCreator = func(_ *config.ResolvedAuth) worklogCreator {
		return mock
	}
	t.Cleanup(func() { newWorklogCreator = origCreator })

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

func TestCreateMinimalRequiredFlags(t *testing.T) {
	testutil.ResetOutputFlags(t)

	mock := &mockWorklogCreator{
		worklog: makeCreatedWorklog("wl-1", ""),
		resp:    &tracker.Response{},
	}

	out, err := setupCreateCmd(t, mock, []string{
		"PROJ-123",
		"--duration", "PT1H30M",
		"--start", "2026-03-30T10:00:00Z",
	})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	expected := "Worklog wl-1 created on PROJ-123"
	if !strings.Contains(out, expected) {
		t.Errorf("expected %q in output, got: %s", expected, out)
	}

	// Verify API call.
	if mock.gotIssueKey != "PROJ-123" {
		t.Errorf("expected issueKey=PROJ-123, got %q", mock.gotIssueKey)
	}
	if mock.gotReq == nil || mock.gotReq.Start == nil {
		t.Fatal("expected request start to be set")
	}
	if mock.gotReq.Duration == nil {
		t.Fatal("expected request duration to be set")
	}
	wantStart := time.Date(2026, 3, 30, 10, 0, 0, 0, time.UTC)
	if got := mock.gotReq.Start.Time; !got.Equal(wantStart) {
		t.Fatalf("expected start=%s, got %s", wantStart.Format(time.RFC3339), got.Format(time.RFC3339))
	}
}

func TestCreateAllFlags(t *testing.T) {
	testutil.ResetOutputFlags(t)

	mock := &mockWorklogCreator{
		worklog: makeCreatedWorklog("wl-2", "Code review"),
		resp:    &tracker.Response{},
	}

	out, err := setupCreateCmd(t, mock, []string{
		"PROJ-123",
		"--duration", "PT2H",
		"--comment", "Code review",
		"--start", "2026-03-30T10:00:00Z",
	})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if !strings.Contains(out, "Worklog wl-2 created on PROJ-123") {
		t.Errorf("unexpected output: %s", out)
	}
}

func TestCreateJSON(t *testing.T) {
	testutil.ResetOutputFlags(t)
	output.JSONFields = WorklogFields

	mock := &mockWorklogCreator{
		worklog: makeCreatedWorklog("wl-json", "Test"),
		resp:    &tracker.Response{},
	}

	out, err := setupCreateCmd(t, mock, []string{
		"PROJ-1",
		"--duration", "PT1H",
		"--start", "2026-03-30T10:00:00Z",
	})
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
	if item["author"] != "testuser" {
		t.Errorf("expected author=testuser, got %v", item["author"])
	}
}

func TestCreateQuiet(t *testing.T) {
	testutil.ResetOutputFlags(t)
	output.QuietFlag = true

	mock := &mockWorklogCreator{
		worklog: makeCreatedWorklog("wl-quiet", ""),
		resp:    &tracker.Response{},
	}

	out, err := setupCreateCmd(t, mock, []string{
		"PROJ-1",
		"--duration", "PT30M",
		"--start", "2026-03-30T10:00:00Z",
	})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	trimmed := strings.TrimSpace(out)
	if trimmed != "wl-quiet" {
		t.Errorf("expected 'wl-quiet', got %q", trimmed)
	}
}

func TestCreateInvalidDuration(t *testing.T) {
	testutil.ResetOutputFlags(t)

	mock := &mockWorklogCreator{}

	_, err := setupCreateCmd(t, mock, []string{
		"PROJ-1",
		"--duration", "invalid",
		"--start", "2026-03-30T10:00:00Z",
	})
	if err == nil {
		t.Fatal("expected error for invalid duration, got nil")
	}

	if !strings.Contains(err.Error(), "invalid ISO 8601 duration") {
		t.Errorf("expected duration error, got: %v", err)
	}
}

func TestCreateInvalidTimestamp(t *testing.T) {
	testutil.ResetOutputFlags(t)

	mock := &mockWorklogCreator{}

	_, err := setupCreateCmd(t, mock, []string{
		"PROJ-1", "--duration", "PT1H", "--start", "not-a-date",
	})
	if err == nil {
		t.Fatal("expected error for invalid timestamp, got nil")
	}

	if !strings.Contains(err.Error(), "invalid timestamp") {
		t.Errorf("expected timestamp error, got: %v", err)
	}
}

func TestCreateMissingStart(t *testing.T) {
	testutil.ResetOutputFlags(t)

	mock := &mockWorklogCreator{}

	_, err := setupCreateCmd(t, mock, []string{"PROJ-1", "--duration", "PT1H"})
	if err == nil {
		t.Fatal("expected error for missing start, got nil")
	}

	if !strings.Contains(err.Error(), "--start is required") {
		t.Errorf("expected missing start error, got: %v", err)
	}
}

func TestCreateFromJSON(t *testing.T) {
	testutil.ResetOutputFlags(t)

	mock := &mockWorklogCreator{
		worklog: makeCreatedWorklog("wl-fj", "From JSON"),
		resp:    &tracker.Response{},
	}

	out, err := setupCreateCmd(t, mock, []string{
		"PROJ-1", "--from-json", `{"start":"2026-03-30T10:00:00Z","duration":"PT1H","comment":"From JSON"}`,
	})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if !strings.Contains(out, "Worklog wl-fj created on PROJ-1") {
		t.Errorf("unexpected output: %s", out)
	}
}

func TestCreateFromJSONMissingStart(t *testing.T) {
	testutil.ResetOutputFlags(t)

	mock := &mockWorklogCreator{}

	_, err := setupCreateCmd(t, mock, []string{
		"PROJ-1", "--from-json", `{"duration":"PT1H","comment":"From JSON"}`,
	})
	if err == nil {
		t.Fatal("expected error for missing start in JSON, got nil")
	}

	if !strings.Contains(err.Error(), `missing required field "start"`) {
		t.Errorf("expected missing start validation error, got: %v", err)
	}
}

func TestCreateFromJSONMissingDuration(t *testing.T) {
	testutil.ResetOutputFlags(t)

	mock := &mockWorklogCreator{}

	_, err := setupCreateCmd(t, mock, []string{
		"PROJ-1", "--from-json", `{"start":"2026-03-30T10:00:00Z","comment":"From JSON"}`,
	})
	if err == nil {
		t.Fatal("expected error for missing duration in JSON, got nil")
	}

	if !strings.Contains(err.Error(), `missing required field "duration"`) {
		t.Errorf("expected missing duration validation error, got: %v", err)
	}
}

func TestCreateMutualExclusion(t *testing.T) {
	testutil.ResetOutputFlags(t)

	mock := &mockWorklogCreator{}

	_, err := setupCreateCmd(t, mock, []string{
		"PROJ-1", "--from-json", `{"duration":"PT1H"}`, "--duration", "PT2H",
	})
	if err == nil {
		t.Fatal("expected mutual exclusion error, got nil")
	}

	if !strings.Contains(err.Error(), "cannot use individual flags and --from-json together") {
		t.Errorf("expected mutual exclusion error, got: %v", err)
	}
}

func TestCreateMissingDuration(t *testing.T) {
	testutil.ResetOutputFlags(t)

	mock := &mockWorklogCreator{}

	_, err := setupCreateCmd(t, mock, []string{"PROJ-1"})
	if err == nil {
		t.Fatal("expected error for missing --duration, got nil")
	}

	if !strings.Contains(err.Error(), "--duration is required") {
		t.Errorf("expected duration required error, got: %v", err)
	}
}

func TestCreateAPIError(t *testing.T) {
	testutil.ResetOutputFlags(t)

	mock := &mockWorklogCreator{
		err: errors.New("connection refused"),
	}

	_, err := setupCreateCmd(t, mock, []string{
		"PROJ-1",
		"--duration", "PT1H",
		"--start", "2026-03-30T10:00:00Z",
	})
	if err == nil {
		t.Fatal("expected error from API, got nil")
	}

	if !strings.Contains(err.Error(), "API request failed") {
		t.Errorf("expected mapped API error, got: %v", err)
	}
}

func TestCreateRequestCapture(t *testing.T) {
	testutil.ResetOutputFlags(t)

	mock := &mockWorklogCreator{
		worklog: makeCreatedWorklog("wl-cap", ""),
		resp:    &tracker.Response{},
	}

	_, err := setupCreateCmd(t, mock, []string{
		"PROJ-123",
		"--duration", "PT1H30M",
		"--start", "2026-03-30T10:00:00Z",
	})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if mock.gotReq == nil {
		t.Fatal("expected request to be captured")
	}
	if mock.gotReq.Duration == nil {
		t.Fatal("expected Duration to be set in request")
	}
	wantStart := time.Date(2026, 3, 30, 10, 0, 0, 0, time.UTC)
	if mock.gotReq.Start == nil {
		t.Fatal("expected Start to be set in request")
	}
	if got := mock.gotReq.Start.Time; !got.Equal(wantStart) {
		t.Fatalf("expected start=%s, got %s", wantStart.Format(time.RFC3339), got.Format(time.RFC3339))
	}
	// PT1H30M = 90 minutes.
	if mock.gotReq.Duration.Duration != 90*time.Minute {
		t.Errorf("expected duration=90m, got %v", mock.gotReq.Duration.Duration)
	}
}
