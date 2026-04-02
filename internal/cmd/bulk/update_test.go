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

// mockBulkUpdater implements bulkUpdater for testing.
type mockBulkUpdater struct {
	bc    *tracker.BulkChange
	err   error
	calls []mockUpdateCall
}

// mockUpdateCall records a single Update invocation for assertion.
type mockUpdateCall struct {
	req *tracker.BulkUpdateRequest
}

func (m *mockBulkUpdater) Update(
	_ context.Context,
	req *tracker.BulkUpdateRequest,
) (*tracker.BulkChange, *tracker.Response, error) {
	m.calls = append(m.calls, mockUpdateCall{req: req})
	if m.err != nil {
		return nil, nil, m.err
	}
	return m.bc, &tracker.Response{}, nil
}

// setupUpdateCmd creates an update command with mocked dependencies.
func setupUpdateCmd(
	t *testing.T,
	updaterMock *mockBulkUpdater,
	pollMock *mockPollGetter,
	args []string,
) (string, error) {
	t.Helper()

	origUpdater := newBulkUpdater
	newBulkUpdater = func(_ *config.ResolvedAuth) bulkUpdater {
		return updaterMock
	}
	t.Cleanup(func() { newBulkUpdater = origUpdater })

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

func TestUpdateTable(t *testing.T) {
	testutil.ResetOutputFlags(t)

	bc := makeCompletedBulkChange("update-op-1")
	updater := &mockBulkUpdater{bc: bc}
	poll := &mockPollGetter{bc: bc}

	out, err := setupUpdateCmd(t, updater, poll,
		[]string{"PROJ-1", "--field", "priority=critical"})
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
	if len(updater.calls) != 1 {
		t.Fatalf("expected 1 Update call, got %d", len(updater.calls))
	}

	req := updater.calls[0].req
	if len(req.Issues) != 1 || req.Issues[0] != "PROJ-1" {
		t.Errorf("expected issues=[PROJ-1], got %v", req.Issues)
	}

	if req.Values["priority"] != "critical" {
		t.Errorf("expected priority=critical, got %v", req.Values["priority"])
	}
}

func TestUpdateJSON(t *testing.T) {
	testutil.ResetOutputFlags(t)
	output.JSONFields = BulkStatusFields

	bc := makeCompletedBulkChange("update-json-1")
	updater := &mockBulkUpdater{bc: bc}
	poll := &mockPollGetter{bc: bc}

	out, err := setupUpdateCmd(t, updater, poll,
		[]string{"PROJ-1", "--field", "priority=critical"})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	var item map[string]any
	if err := json.Unmarshal([]byte(out), &item); err != nil {
		t.Fatalf("invalid JSON: %v\nraw: %s", err, out)
	}

	if item["id"] != "update-json-1" {
		t.Errorf("expected id=update-json-1, got %v", item["id"])
	}
	if item["status"] != "COMPLETED" {
		t.Errorf("expected status=COMPLETED, got %v", item["status"])
	}
}

func TestUpdateQuiet(t *testing.T) {
	testutil.ResetOutputFlags(t)
	output.QuietFlag = true

	bc := makeCompletedBulkChange("update-quiet-1")
	updater := &mockBulkUpdater{bc: bc}
	poll := &mockPollGetter{bc: bc}

	out, err := setupUpdateCmd(t, updater, poll,
		[]string{"PROJ-1", "--field", "priority=critical"})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	trimmed := strings.TrimSpace(out)
	if trimmed != "update-quiet-1" {
		t.Errorf("expected 'update-quiet-1', got %q", trimmed)
	}
}

func TestUpdateMultipleFields(t *testing.T) {
	testutil.ResetOutputFlags(t)

	bc := makeCompletedBulkChange("update-multi-1")
	updater := &mockBulkUpdater{bc: bc}
	poll := &mockPollGetter{bc: bc}

	_, err := setupUpdateCmd(t, updater, poll,
		[]string{"PROJ-1", "--field", "priority=critical", "--field", "assignee=user1"})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if len(updater.calls) != 1 {
		t.Fatalf("expected 1 Update call, got %d", len(updater.calls))
	}

	req := updater.calls[0].req
	if req.Values["priority"] != "critical" {
		t.Errorf("expected priority=critical, got %v", req.Values["priority"])
	}

	if req.Values["assignee"] != "user1" {
		t.Errorf("expected assignee=user1, got %v", req.Values["assignee"])
	}
}

func TestUpdateFromJSON(t *testing.T) {
	testutil.ResetOutputFlags(t)

	bc := makeCompletedBulkChange("update-fj-1")
	updater := &mockBulkUpdater{bc: bc}
	poll := &mockPollGetter{bc: bc}

	_, err := setupUpdateCmd(t, updater, poll,
		[]string{"--from-json", `{"issues":["PROJ-1"],"values":{"priority":"critical"}}`})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if len(updater.calls) != 1 {
		t.Fatalf("expected 1 Update call, got %d", len(updater.calls))
	}

	req := updater.calls[0].req
	if len(req.Issues) != 1 || req.Issues[0] != "PROJ-1" {
		t.Errorf("expected issues=[PROJ-1], got %v", req.Issues)
	}

	if req.Values["priority"] != "critical" {
		t.Errorf("expected priority=critical in Values, got %v", req.Values["priority"])
	}
}

func TestUpdateMutualExclusion(t *testing.T) {
	testutil.ResetOutputFlags(t)

	updater := &mockBulkUpdater{}
	poll := &mockPollGetter{}

	_, err := setupUpdateCmd(t, updater, poll,
		[]string{"PROJ-1", "--field", "x=y", "--from-json", "{}"})
	if err == nil {
		t.Fatal("expected mutual exclusion error, got nil")
	}

	if !strings.Contains(err.Error(), "cannot use") {
		t.Errorf("expected 'cannot use' in error, got: %v", err)
	}
}

func TestUpdateMissingField(t *testing.T) {
	testutil.ResetOutputFlags(t)

	updater := &mockBulkUpdater{}
	poll := &mockPollGetter{}

	_, err := setupUpdateCmd(t, updater, poll, []string{"PROJ-1"})
	if err == nil {
		t.Fatal("expected error for missing --field, got nil")
	}

	if !strings.Contains(err.Error(), "--field is required") {
		t.Errorf("expected '--field is required' in error, got: %v", err)
	}
}

func TestUpdateAPIError(t *testing.T) {
	testutil.ResetOutputFlags(t)

	updater := &mockBulkUpdater{err: errors.New("connection refused")}
	poll := &mockPollGetter{}

	_, err := setupUpdateCmd(t, updater, poll,
		[]string{"PROJ-1", "--field", "priority=critical"})
	if err == nil {
		t.Fatal("expected error from API, got nil")
	}

	if !strings.Contains(err.Error(), "API request failed") {
		t.Errorf("expected mapped API error, got: %v", err)
	}
}

func TestUpdate_RegisteredAsSubcommand(t *testing.T) {
	cmd := NewCmd()

	found := false
	for _, sub := range cmd.Commands() {
		if sub.Name() == "update" {
			found = true
			break
		}
	}

	if !found {
		t.Error("expected 'update' subcommand to be registered on bulk command")
	}
}
