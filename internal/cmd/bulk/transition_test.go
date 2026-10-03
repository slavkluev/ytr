package bulk

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"io"
	"os"
	"strings"
	"testing"

	"github.com/slavkluev/go-yandex-tracker/tracker"

	"github.com/slavkluev/ytr/internal/config"
	"github.com/slavkluev/ytr/internal/output"
)

// mockBulkTransitioner implements bulkTransitioner for testing.
type mockBulkTransitioner struct {
	bc    *tracker.BulkChange
	err   error
	calls []mockTransitionCall
}

// mockTransitionCall records a single Transition invocation for assertion.
type mockTransitionCall struct {
	req *tracker.BulkTransitionRequest
}

func (m *mockBulkTransitioner) Transition(
	_ context.Context,
	req *tracker.BulkTransitionRequest,
) (*tracker.BulkChange, *tracker.Response, error) {
	m.calls = append(m.calls, mockTransitionCall{req: req})
	if m.err != nil {
		return nil, nil, m.err
	}
	return m.bc, &tracker.Response{}, nil
}

// setupTransitionCmd creates a transition command with mocked dependencies.
func setupTransitionCmd(
	t *testing.T,
	transitionerMock *mockBulkTransitioner,
	pollMock *mockPollGetter,
	opts output.Options,
	args []string,
) (string, error) {
	t.Helper()

	origTransitioner := newBulkTransitioner
	newBulkTransitioner = func(_ *config.ResolvedAuth) bulkTransitioner {
		return transitionerMock
	}
	t.Cleanup(func() { newBulkTransitioner = origTransitioner })

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

	// Separate buffers: only what reaches stdout is returned, so a test can
	// tell the command's document apart from cobra's error text.
	buf := &bytes.Buffer{}
	cmd := newTransitionCmd()
	cmd.SetOut(buf)
	cmd.SetErr(io.Discard)
	// The binary silences both on the root command, so nothing cobra writes
	// about a failure reaches the command's own output stream.
	cmd.SilenceErrors = true
	cmd.SilenceUsage = true

	// Simulate root persistent flags for auth.
	cmd.PersistentFlags().String("token", "test-token", "")
	cmd.PersistentFlags().String("org-id", "test-org", "")
	cmd.PersistentFlags().String("org-type", "360", "")

	cmd.SetArgs(args)
	err := cmd.ExecuteContext(output.NewContext(t.Context(), &opts))
	return buf.String(), err
}

func TestTransitionTable(t *testing.T) {
	bc := makeCompletedBulkChange("transition-op-1")
	transitioner := &mockBulkTransitioner{bc: bc}
	poll := &mockPollGetter{bc: bc}

	out, err := setupTransitionCmd(t, transitioner, poll,
		output.Options{},
		[]string{"PROJ-1", "PROJ-2", "--transition", "close"})
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
	if len(transitioner.calls) != 1 {
		t.Fatalf("expected 1 Transition call, got %d", len(transitioner.calls))
	}

	req := transitioner.calls[0].req
	if req.Transition == nil || *req.Transition != "close" {
		t.Errorf("expected transition=close, got %v", req.Transition)
	}

	if len(req.Issues) != 2 || req.Issues[0] != "PROJ-1" || req.Issues[1] != "PROJ-2" {
		t.Errorf("expected issues=[PROJ-1,PROJ-2], got %v", req.Issues)
	}
}

func TestTransitionJSON(t *testing.T) {
	bc := makeCompletedBulkChange("transition-json-1")
	transitioner := &mockBulkTransitioner{bc: bc}
	poll := &mockPollGetter{bc: bc}

	out, err := setupTransitionCmd(t, transitioner, poll,
		output.Options{JSONFields: BulkStatusFields},
		[]string{"PROJ-1", "--transition", "close"})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	var item map[string]any
	if err := json.Unmarshal([]byte(out), &item); err != nil {
		t.Fatalf("invalid JSON: %v\nraw: %s", err, out)
	}

	if item["id"] != "transition-json-1" {
		t.Errorf("expected id=transition-json-1, got %v", item["id"])
	}
	if item["status"] != "COMPLETED" {
		t.Errorf("expected status=COMPLETED, got %v", item["status"])
	}
}

func TestTransitionQuiet(t *testing.T) {
	bc := makeCompletedBulkChange("transition-quiet-1")
	transitioner := &mockBulkTransitioner{bc: bc}
	poll := &mockPollGetter{bc: bc}

	out, err := setupTransitionCmd(t, transitioner, poll,
		output.Options{Quiet: true},
		[]string{"PROJ-1", "--transition", "close"})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	trimmed := strings.TrimSpace(out)
	if trimmed != "transition-quiet-1" {
		t.Errorf("expected 'transition-quiet-1', got %q", trimmed)
	}
}

func TestTransitionWithFields(t *testing.T) {
	bc := makeCompletedBulkChange("transition-fields-1")
	transitioner := &mockBulkTransitioner{bc: bc}
	poll := &mockPollGetter{bc: bc}

	_, err := setupTransitionCmd(t, transitioner, poll,
		output.Options{},
		[]string{"PROJ-1", "--transition", "close", "--field", "resolution=fixed"})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if len(transitioner.calls) != 1 {
		t.Fatalf("expected 1 Transition call, got %d", len(transitioner.calls))
	}

	req := transitioner.calls[0].req
	if req.Values == nil {
		t.Fatal("expected Values in request, got nil")
	}

	if req.Values["resolution"] != "fixed" {
		t.Errorf("expected resolution=fixed, got %v", req.Values["resolution"])
	}
}

func TestTransitionFromJSON(t *testing.T) {
	bc := makeCompletedBulkChange("transition-fj-1")
	transitioner := &mockBulkTransitioner{bc: bc}
	poll := &mockPollGetter{bc: bc}

	_, err := setupTransitionCmd(t, transitioner, poll,
		output.Options{},
		[]string{"--from-json", `{"transition":"close","issues":["PROJ-1"]}`})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if len(transitioner.calls) != 1 {
		t.Fatalf("expected 1 Transition call, got %d", len(transitioner.calls))
	}

	req := transitioner.calls[0].req
	if req.Transition == nil || *req.Transition != "close" {
		t.Errorf("expected transition=close, got %v", req.Transition)
	}

	if len(req.Issues) != 1 || req.Issues[0] != "PROJ-1" {
		t.Errorf("expected issues=[PROJ-1], got %v", req.Issues)
	}
}

func TestTransitionMutualExclusion(t *testing.T) {
	transitioner := &mockBulkTransitioner{}
	poll := &mockPollGetter{}

	_, err := setupTransitionCmd(t, transitioner, poll,
		output.Options{},
		[]string{"PROJ-1", "--transition", "close", "--from-json", "{}"})
	if err == nil {
		t.Fatal("expected mutual exclusion error, got nil")
	}

	if !strings.Contains(err.Error(), "cannot combine --from-json with --transition") {
		t.Errorf("expected 'cannot combine --from-json with --transition' in error, got: %v", err)
	}
}

func TestTransitionMissingTransition(t *testing.T) {
	transitioner := &mockBulkTransitioner{}
	poll := &mockPollGetter{}

	_, err := setupTransitionCmd(t, transitioner, poll, output.Options{}, []string{"PROJ-1"})
	if err == nil {
		t.Fatal("expected error for missing --transition, got nil")
	}

	if !strings.Contains(err.Error(), "missing --transition") {
		t.Errorf("expected 'missing --transition' in error, got: %v", err)
	}
}

func TestTransitionAPIError(t *testing.T) {
	transitioner := &mockBulkTransitioner{err: errors.New("connection refused")}
	poll := &mockPollGetter{}

	_, err := setupTransitionCmd(t, transitioner, poll,
		output.Options{},
		[]string{"PROJ-1", "--transition", "close"})
	if err == nil {
		t.Fatal("expected error from API, got nil")
	}

	if !strings.Contains(err.Error(), "API request failed") {
		t.Errorf("expected mapped API error, got: %v", err)
	}
}

func TestTransition_RegisteredAsSubcommand(t *testing.T) {
	cmd := NewCmd()

	found := false
	for _, sub := range cmd.Commands() {
		if sub.Name() == "transition" {
			found = true
			break
		}
	}

	if !found {
		t.Error("expected 'transition' subcommand to be registered on bulk command")
	}
}

func TestTransitionFromJSONRejectsUnknownFields(t *testing.T) {
	bc := makeCompletedBulkChange("transition-unknown-1")
	transitioner := &mockBulkTransitioner{bc: bc}
	poll := &mockPollGetter{bc: bc}

	_, err := setupTransitionCmd(t, transitioner, poll,
		output.Options{},
		[]string{"--from-json", `{"transition":"close","issues":["PROJ-1"],"bogus":1}`})
	if err == nil {
		t.Fatal("expected an error for an unknown field, got nil")
	}

	if !strings.Contains(err.Error(), "bogus") {
		t.Errorf("error %q should name the unknown field", err.Error())
	}
	if len(transitioner.calls) != 0 {
		t.Error("Transition should not have been called")
	}
}

func TestTransitionFailedStatusWritesNothingToStdout(t *testing.T) {
	bc := makeFailedBulkChange("transition-fail-1")
	transitioner := &mockBulkTransitioner{bc: bc}
	poll := &mockPollGetter{bc: bc}

	out, err := setupTransitionCmd(t, transitioner, poll,
		output.Options{JSONFields: BulkStatusFields},
		[]string{"PROJ-1", "PROJ-2", "--transition", "close"})
	if err == nil {
		t.Fatal("expected non-nil error for FAILED bulk operation, got nil")
	}
	if out != "" {
		t.Errorf("stdout = %q, want empty for a FAILED operation", out)
	}

	assertBulkFailedDetail(t, err, "transition-fail-1", 2, 0)
}
