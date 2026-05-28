package bulk

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"os"
	"strings"
	"testing"
	"time"

	"github.com/slavkluev/go-yandex-tracker/tracker"

	"github.com/slavkluev/ytr/internal/config"
	ytrerrors "github.com/slavkluev/ytr/internal/errors"
	"github.com/slavkluev/ytr/internal/output"
	"github.com/slavkluev/ytr/internal/testutil"
)

// mockBulkMover implements bulkMover for testing.
type mockBulkMover struct {
	bc    *tracker.BulkChange
	err   error
	calls []mockMoveCall
}

// mockMoveCall records a single Move invocation for assertion.
type mockMoveCall struct {
	req *tracker.BulkMoveRequest
}

func (m *mockBulkMover) Move(
	_ context.Context,
	req *tracker.BulkMoveRequest,
) (*tracker.BulkChange, *tracker.Response, error) {
	m.calls = append(m.calls, mockMoveCall{req: req})
	if m.err != nil {
		return nil, nil, m.err
	}
	return m.bc, &tracker.Response{}, nil
}

// mockPollGetter implements bulkStatusGetter for polling in tests.
// Returns the configured BulkChange immediately (COMPLETED status).
type mockPollGetter struct {
	bc  *tracker.BulkChange
	err error
}

func (m *mockPollGetter) GetStatus(
	_ context.Context,
	_ string,
) (*tracker.BulkChange, *tracker.Response, error) {
	if m.err != nil {
		return nil, nil, m.err
	}
	return m.bc, &tracker.Response{}, nil
}

// makeCompletedBulkChange creates a BulkChange with COMPLETED status.
func makeCompletedBulkChange(id string) *tracker.BulkChange {
	ts := tracker.Timestamp{
		Time: time.Date(2026, 3, 30, 12, 0, 0, 0, time.UTC),
	}
	return &tracker.BulkChange{
		ID:                    testutil.FlexStringPtr(id),
		Status:                testutil.StrPtr("COMPLETED"),
		StatusText:            testutil.StrPtr("Operation COMPLETED"),
		TotalIssues:           testutil.IntPtr(2),
		TotalCompletedIssues:  testutil.IntPtr(2),
		ExecutionIssuePercent: testutil.IntPtr(100),
		ExecutionChunkPercent: testutil.IntPtr(100),
		CreatedBy:             &tracker.User{Display: testutil.StrPtr("testuser")},
		CreatedAt:             &ts,
	}
}

// makeFailedBulkChange creates a BulkChange with FAILED status.
func makeFailedBulkChange(id string) *tracker.BulkChange {
	return &tracker.BulkChange{
		ID:                    testutil.FlexStringPtr(id),
		Status:                testutil.StrPtr("FAILED"),
		StatusText:            testutil.StrPtr("Operation FAILED"),
		TotalIssues:           testutil.IntPtr(2),
		TotalCompletedIssues:  testutil.IntPtr(0),
		ExecutionIssuePercent: testutil.IntPtr(0),
		ExecutionChunkPercent: testutil.IntPtr(0),
	}
}

// setupMoveCmd creates a move command with mocked dependencies.
func setupMoveCmd(
	t *testing.T,
	moverMock *mockBulkMover,
	pollMock *mockPollGetter,
	args []string,
) (string, error) {
	t.Helper()

	origMover := newBulkMover
	newBulkMover = func(_ *config.ResolvedAuth) bulkMover {
		return moverMock
	}
	t.Cleanup(func() { newBulkMover = origMover })

	origGetter := newBulkStatusGetter
	newBulkStatusGetter = func(_ *config.ResolvedAuth) bulkStatusGetter {
		return pollMock
	}
	t.Cleanup(func() { newBulkStatusGetter = origGetter })

	// Suppress progress output in tests.
	origStderr := stderrFile
	r, w, _ := os.Pipe()
	stderrFile = r
	t.Cleanup(func() {
		stderrFile = origStderr
		w.Close()
		r.Close()
	})

	buf := &bytes.Buffer{}
	cmd := newMoveCmd()
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

func TestMoveTable(t *testing.T) {
	testutil.ResetOutputFlags(t)

	bc := makeCompletedBulkChange("move-op-1")
	mover := &mockBulkMover{bc: bc}
	poll := &mockPollGetter{bc: bc}

	out, err := setupMoveCmd(t, mover, poll,
		[]string{"PROJ-1", "PROJ-2", "--queue", "TARGET"})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	// Verify table headers.
	for _, header := range []string{"ID", "STATUS", "TOTAL", "DONE", "PERCENT"} {
		if !strings.Contains(out, header) {
			t.Errorf("expected header %q in output, got: %s", header, out)
		}
	}

	if !strings.Contains(out, "COMPLETED") {
		t.Errorf("expected COMPLETED in output, got: %s", out)
	}

	// Verify mock called with correct request.
	if len(mover.calls) != 1 {
		t.Fatalf("expected 1 Move call, got %d", len(mover.calls))
	}

	req := mover.calls[0].req
	if req.Queue == nil || *req.Queue != "TARGET" {
		t.Errorf("expected queue=TARGET, got %v", req.Queue)
	}

	if len(req.Issues) != 2 || req.Issues[0] != "PROJ-1" || req.Issues[1] != "PROJ-2" {
		t.Errorf("expected issues=[PROJ-1,PROJ-2], got %v", req.Issues)
	}
}

func TestMoveJSON(t *testing.T) {
	testutil.ResetOutputFlags(t)
	output.JSONFields = BulkStatusFields

	bc := makeCompletedBulkChange("move-json-1")
	mover := &mockBulkMover{bc: bc}
	poll := &mockPollGetter{bc: bc}

	out, err := setupMoveCmd(t, mover, poll,
		[]string{"PROJ-1", "--queue", "TARGET"})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	var item map[string]any
	if err := json.Unmarshal([]byte(out), &item); err != nil {
		t.Fatalf("invalid JSON: %v\nraw: %s", err, out)
	}

	if item["id"] != "move-json-1" {
		t.Errorf("expected id=move-json-1, got %v", item["id"])
	}
	if item["status"] != "COMPLETED" {
		t.Errorf("expected status=COMPLETED, got %v", item["status"])
	}
}

func TestMoveQuiet(t *testing.T) {
	testutil.ResetOutputFlags(t)
	output.QuietFlag = true

	bc := makeCompletedBulkChange("move-quiet-1")
	mover := &mockBulkMover{bc: bc}
	poll := &mockPollGetter{bc: bc}

	out, err := setupMoveCmd(t, mover, poll,
		[]string{"PROJ-1", "--queue", "TARGET"})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	trimmed := strings.TrimSpace(out)
	if trimmed != "move-quiet-1" {
		t.Errorf("expected 'move-quiet-1', got %q", trimmed)
	}
}

func TestMoveWithFields(t *testing.T) {
	testutil.ResetOutputFlags(t)

	bc := makeCompletedBulkChange("move-fields-1")
	mover := &mockBulkMover{bc: bc}
	poll := &mockPollGetter{bc: bc}

	_, err := setupMoveCmd(t, mover, poll,
		[]string{"PROJ-1", "--queue", "TARGET", "--field", "priority=critical"})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if len(mover.calls) != 1 {
		t.Fatalf("expected 1 Move call, got %d", len(mover.calls))
	}

	req := mover.calls[0].req
	if req.Values == nil {
		t.Fatal("expected Values in request, got nil")
	}

	if req.Values["priority"] != "critical" {
		t.Errorf("expected priority=critical, got %v", req.Values["priority"])
	}
}

func TestMoveFromJSON(t *testing.T) {
	testutil.ResetOutputFlags(t)

	bc := makeCompletedBulkChange("move-fj-1")
	mover := &mockBulkMover{bc: bc}
	poll := &mockPollGetter{bc: bc}

	_, err := setupMoveCmd(t, mover, poll,
		[]string{"--from-json", `{"queue":"TARGET","issues":["PROJ-1"],"moveAllFields":true}`})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if len(mover.calls) != 1 {
		t.Fatalf("expected 1 Move call, got %d", len(mover.calls))
	}

	req := mover.calls[0].req
	if req.Queue == nil || *req.Queue != "TARGET" {
		t.Errorf("expected queue=TARGET, got %v", req.Queue)
	}

	if len(req.Issues) != 1 || req.Issues[0] != "PROJ-1" {
		t.Errorf("expected issues=[PROJ-1], got %v", req.Issues)
	}

	if req.MoveAllFields == nil || !*req.MoveAllFields {
		t.Error("expected moveAllFields=true")
	}
}

func TestMoveMutualExclusion(t *testing.T) {
	testutil.ResetOutputFlags(t)

	mover := &mockBulkMover{}
	poll := &mockPollGetter{}

	_, err := setupMoveCmd(t, mover, poll,
		[]string{"PROJ-1", "--queue", "TARGET", "--from-json", "{}"})
	if err == nil {
		t.Fatal("expected mutual exclusion error, got nil")
	}

	if !strings.Contains(err.Error(), "cannot use") {
		t.Errorf("expected 'cannot use' in error, got: %v", err)
	}
}

func TestMoveMissingQueue(t *testing.T) {
	testutil.ResetOutputFlags(t)

	mover := &mockBulkMover{}
	poll := &mockPollGetter{}

	_, err := setupMoveCmd(t, mover, poll, []string{"PROJ-1"})
	if err == nil {
		t.Fatal("expected error for missing --queue, got nil")
	}

	if !strings.Contains(err.Error(), "--queue is required") {
		t.Errorf("expected '--queue is required' in error, got: %v", err)
	}
}

func TestMoveAPIError(t *testing.T) {
	testutil.ResetOutputFlags(t)

	mover := &mockBulkMover{err: errors.New("connection refused")}
	poll := &mockPollGetter{}

	_, err := setupMoveCmd(t, mover, poll,
		[]string{"PROJ-1", "--queue", "TARGET"})
	if err == nil {
		t.Fatal("expected error from API, got nil")
	}

	if !strings.Contains(err.Error(), "API request failed") {
		t.Errorf("expected mapped API error, got: %v", err)
	}
}

func TestMoveNoKeys(t *testing.T) {
	testutil.ResetOutputFlags(t)

	// Override stdin to a TTY-like pipe that appears as non-pipe.
	// Since readIssueKeys uses isatty, we override stdinFile to os.Stdout
	// which is a TTY in test environments. Use a pipe instead to simulate.
	origStdin := stdinFile
	r, w, _ := os.Pipe()
	// Write nothing and close to simulate empty non-TTY stdin.
	w.Close()
	stdinFile = r
	t.Cleanup(func() { stdinFile = origStdin; r.Close() })

	mover := &mockBulkMover{bc: makeCompletedBulkChange("x")}
	poll := &mockPollGetter{bc: makeCompletedBulkChange("x")}

	_, err := setupMoveCmd(t, mover, poll, []string{"--queue", "TARGET"})
	if err == nil {
		t.Fatal("expected error for no keys, got nil")
	}

	if !strings.Contains(err.Error(), "no issue keys provided") {
		t.Errorf("expected 'no issue keys provided' in error, got: %v", err)
	}
}

func TestMoveFailedStatusExitsNonZero(t *testing.T) {
	testutil.ResetOutputFlags(t)

	bc := makeFailedBulkChange("move-fail-1")
	mover := &mockBulkMover{bc: bc}
	poll := &mockPollGetter{bc: bc}

	out, err := setupMoveCmd(t, mover, poll,
		[]string{"PROJ-1", "PROJ-2", "--queue", "TARGET"})
	if err == nil {
		t.Fatal("expected non-nil error for FAILED bulk operation, got nil")
	}

	var exitErr *ytrerrors.ExitError
	if !errors.As(err, &exitErr) {
		t.Fatalf("expected *ExitError, got %T: %v", err, err)
	}
	if exitErr.ExitCode == ytrerrors.ExitSuccess {
		t.Errorf("expected non-zero exit code for FAILED operation, got %d", exitErr.ExitCode)
	}
	if exitErr.Code != "bulk_failed" {
		t.Errorf("expected code=bulk_failed, got %q", exitErr.Code)
	}
	if !strings.Contains(exitErr.Message, "Operation FAILED") {
		t.Errorf("expected statusText in error message, got: %q", exitErr.Message)
	}

	// Details must still be rendered (review: "оставив вывод деталей").
	if !strings.Contains(out, "FAILED") {
		t.Errorf("expected FAILED details rendered, got: %s", out)
	}
}

func TestMoveEmptyOperationID(t *testing.T) {
	testutil.ResetOutputFlags(t)

	// API returns a BulkChange with no ID — must fail loudly, not poll a
	// collection endpoint with an empty operation ID.
	bc := &tracker.BulkChange{Status: testutil.StrPtr("CREATED")}
	mover := &mockBulkMover{bc: bc}
	poll := &mockPollGetter{bc: bc}

	_, err := setupMoveCmd(t, mover, poll,
		[]string{"PROJ-1", "--queue", "TARGET"})
	if err == nil {
		t.Fatal("expected error for empty operation ID, got nil")
	}
	if !strings.Contains(err.Error(), "no operation ID") {
		t.Errorf("expected 'no operation ID' in error, got: %v", err)
	}
}

func TestMove_RegisteredAsSubcommand(t *testing.T) {
	cmd := NewCmd()

	found := false
	for _, sub := range cmd.Commands() {
		if sub.Name() == "move" {
			found = true
			break
		}
	}

	if !found {
		t.Error("expected 'move' subcommand to be registered on bulk command")
	}
}
