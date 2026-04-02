package bulk

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"os"
	"strings"
	"testing"

	"github.com/slavkluev/go-yandex-tracker/tracker"

	"github.com/slavkluev/ytr/internal/config"
	"github.com/slavkluev/ytr/internal/output"
	"github.com/slavkluev/ytr/internal/testutil"
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

	buf := &bytes.Buffer{}
	cmd := newTransitionCmd()
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

func TestTransitionTable(t *testing.T) {
	testutil.ResetOutputFlags(t)

	bc := makeCompletedBulkChange("transition-op-1")
	transitioner := &mockBulkTransitioner{bc: bc}
	poll := &mockPollGetter{bc: bc}

	out, err := setupTransitionCmd(t, transitioner, poll,
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
	testutil.ResetOutputFlags(t)
	output.JSONFields = BulkStatusFields

	bc := makeCompletedBulkChange("transition-json-1")
	transitioner := &mockBulkTransitioner{bc: bc}
	poll := &mockPollGetter{bc: bc}

	out, err := setupTransitionCmd(t, transitioner, poll,
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
	testutil.ResetOutputFlags(t)
	output.QuietFlag = true

	bc := makeCompletedBulkChange("transition-quiet-1")
	transitioner := &mockBulkTransitioner{bc: bc}
	poll := &mockPollGetter{bc: bc}

	out, err := setupTransitionCmd(t, transitioner, poll,
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
	testutil.ResetOutputFlags(t)

	bc := makeCompletedBulkChange("transition-fields-1")
	transitioner := &mockBulkTransitioner{bc: bc}
	poll := &mockPollGetter{bc: bc}

	_, err := setupTransitionCmd(t, transitioner, poll,
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
	testutil.ResetOutputFlags(t)

	bc := makeCompletedBulkChange("transition-fj-1")
	transitioner := &mockBulkTransitioner{bc: bc}
	poll := &mockPollGetter{bc: bc}

	_, err := setupTransitionCmd(t, transitioner, poll,
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
	testutil.ResetOutputFlags(t)

	transitioner := &mockBulkTransitioner{}
	poll := &mockPollGetter{}

	_, err := setupTransitionCmd(t, transitioner, poll,
		[]string{"PROJ-1", "--transition", "close", "--from-json", "{}"})
	if err == nil {
		t.Fatal("expected mutual exclusion error, got nil")
	}

	if !strings.Contains(err.Error(), "cannot use") {
		t.Errorf("expected 'cannot use' in error, got: %v", err)
	}
}

func TestTransitionMissingTransition(t *testing.T) {
	testutil.ResetOutputFlags(t)

	transitioner := &mockBulkTransitioner{}
	poll := &mockPollGetter{}

	_, err := setupTransitionCmd(t, transitioner, poll, []string{"PROJ-1"})
	if err == nil {
		t.Fatal("expected error for missing --transition, got nil")
	}

	if !strings.Contains(err.Error(), "--transition is required") {
		t.Errorf("expected '--transition is required' in error, got: %v", err)
	}
}

func TestTransitionAPIError(t *testing.T) {
	testutil.ResetOutputFlags(t)

	transitioner := &mockBulkTransitioner{err: errors.New("connection refused")}
	poll := &mockPollGetter{}

	_, err := setupTransitionCmd(t, transitioner, poll,
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
