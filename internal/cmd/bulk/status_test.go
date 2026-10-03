package bulk

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

// mockStatusGetter implements bulkStatusGetter for status command tests.
type mockStatusGetter struct {
	bc  *tracker.BulkChange
	err error
}

func (m *mockStatusGetter) GetStatus(
	_ context.Context,
	_ string,
) (*tracker.BulkChange, *tracker.Response, error) {
	if m.err != nil {
		return nil, nil, m.err
	}
	return m.bc, &tracker.Response{}, nil
}

// makeBulkChange creates a BulkChange with common test fields.
func makeBulkChange(id, status string, total, done, pct int) *tracker.BulkChange {
	ts := tracker.Timestamp{
		Time: time.Date(2026, 3, 30, 12, 0, 0, 0, time.UTC),
	}
	return &tracker.BulkChange{
		ID:                    testutil.FlexStringPtr(id),
		Status:                testutil.StrPtr(status),
		StatusText:            testutil.StrPtr("Operation " + status),
		TotalIssues:           testutil.IntPtr(total),
		TotalCompletedIssues:  testutil.IntPtr(done),
		ExecutionIssuePercent: testutil.IntPtr(pct),
		ExecutionChunkPercent: testutil.IntPtr(pct),
		CreatedBy:             &tracker.User{Display: testutil.StrPtr("testuser")},
		CreatedAt:             &ts,
	}
}

// setupStatusCmd creates a status command with mocked dependencies.
func setupStatusCmd(
	t *testing.T,
	mock *mockStatusGetter,
	opts output.Options,
	args []string,
) (string, error) {
	t.Helper()

	origGetter := newBulkStatusGetter
	newBulkStatusGetter = func(_ *config.ResolvedAuth) bulkStatusGetter {
		return mock
	}
	t.Cleanup(func() { newBulkStatusGetter = origGetter })

	buf := &bytes.Buffer{}
	cmd := newStatusCmd()
	cmd.SetOut(buf)
	cmd.SetErr(buf)

	// Simulate root persistent flags for auth.
	cmd.PersistentFlags().String("token", "test-token", "")
	cmd.PersistentFlags().String("org-id", "test-org", "")
	cmd.PersistentFlags().String("org-type", "360", "")

	cmd.SetArgs(args)
	err := cmd.ExecuteContext(output.NewContext(t.Context(), &opts))
	return buf.String(), err
}

func TestStatusTable(t *testing.T) {
	mock := &mockStatusGetter{
		bc: makeBulkChange("593cd211ef7e8a0000000001", "COMPLETED", 50, 50, 100),
	}

	out, err := setupStatusCmd(t, mock, output.Options{}, []string{"593cd211ef7e8a0000000001"})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	// Verify table headers.
	for _, header := range []string{"ID", "STATUS", "TOTAL", "DONE", "PERCENT"} {
		if !strings.Contains(out, header) {
			t.Errorf("expected header %q in output, got: %s", header, out)
		}
	}

	// Verify data values.
	if !strings.Contains(out, "593cd211ef7e8a0000000001") {
		t.Errorf("expected operation ID in output, got: %s", out)
	}
	if !strings.Contains(out, "COMPLETED") {
		t.Errorf("expected COMPLETED in output, got: %s", out)
	}
}

func TestStatusJSON(t *testing.T) {
	mock := &mockStatusGetter{
		bc: makeBulkChange("op-json-1", "COMPLETED", 10, 10, 100),
	}

	out, err := setupStatusCmd(t, mock, output.Options{JSONFields: BulkStatusFields}, []string{"op-json-1"})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	var item map[string]any
	if err := json.Unmarshal([]byte(out), &item); err != nil {
		t.Fatalf("invalid JSON: %v\nraw: %s", err, out)
	}

	if item["id"] != "op-json-1" {
		t.Errorf("expected id=op-json-1, got %v", item["id"])
	}
	if item["status"] != "COMPLETED" {
		t.Errorf("expected status=COMPLETED, got %v", item["status"])
	}

	// totalIssues comes as float64 from JSON unmarshal.
	if total, ok := item["totalIssues"].(float64); !ok || total != 10 {
		t.Errorf("expected totalIssues=10, got %v", item["totalIssues"])
	}

	if item["createdBy"] != "testuser" {
		t.Errorf("expected createdBy=testuser, got %v", item["createdBy"])
	}
}

func TestStatusQuiet(t *testing.T) {
	mock := &mockStatusGetter{
		bc: makeBulkChange("op-quiet-1", "COMPLETED", 5, 5, 100),
	}

	out, err := setupStatusCmd(t, mock, output.Options{Quiet: true}, []string{"op-quiet-1"})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	trimmed := strings.TrimSpace(out)
	if trimmed != "op-quiet-1" {
		t.Errorf("expected 'op-quiet-1', got %q", trimmed)
	}
}

func TestStatusAPIError(t *testing.T) {
	mock := &mockStatusGetter{
		err: errors.New("connection refused"),
	}

	_, err := setupStatusCmd(t, mock, output.Options{}, []string{"op-err-1"})
	if err == nil {
		t.Fatal("expected error from API, got nil")
	}

	if !strings.Contains(err.Error(), "API request failed") {
		t.Errorf("expected mapped API error, got: %v", err)
	}
}

func TestStatusInvalidID(t *testing.T) {
	mock := &mockStatusGetter{}

	_, err := setupStatusCmd(t, mock, output.Options{}, []string{""})
	if err == nil {
		t.Fatal("expected error for empty ID, got nil")
	}

	if !strings.Contains(err.Error(), "operation ID") {
		t.Errorf("expected 'operation ID' in error, got: %v", err)
	}
}

func TestStatus_RegisteredAsSubcommand(t *testing.T) {
	cmd := NewCmd()

	found := false
	for _, sub := range cmd.Commands() {
		if sub.Name() == "status" {
			found = true
			break
		}
	}

	if !found {
		t.Error("expected 'status' subcommand to be registered on bulk command")
	}
}

func TestStatusExposesCreatedByID(t *testing.T) {
	// Built inline rather than via makeBulkChange: only the author matters here,
	// and a fourth caller passing the same status would pin that helper's
	// status parameter to a constant.
	mock := &mockStatusGetter{
		bc: &tracker.BulkChange{
			ID:         testutil.FlexStringPtr("op-1"),
			Status:     testutil.StrPtr("COMPLETED"),
			StatusText: testutil.StrPtr("Operation COMPLETED"),
			CreatedBy: &tracker.User{
				Display: testutil.StrPtr("Иван Петров"),
				ID:      testutil.FlexStringPtr("uid-a"),
			},
		},
	}

	out, err := setupStatusCmd(t, mock, output.Options{JSONFields: BulkStatusFields}, []string{"op-1"})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	var item map[string]any
	if err := json.Unmarshal([]byte(out), &item); err != nil {
		t.Fatalf("invalid JSON: %v\nraw: %s", err, out)
	}
	if item["createdBy"] != "Иван Петров" {
		t.Errorf("expected createdBy='Иван Петров', got %v", item["createdBy"])
	}
	if item["createdById"] != "uid-a" {
		t.Errorf("expected createdById=uid-a, got %v", item["createdById"])
	}
}

// TestStatusReportsAFailedOperationAtExitZero pins the query side of the rule:
// bulk status changes nothing, so a FAILED operation is an answer, not a
// failure, and it arrives as one document on stdout.
func TestStatusReportsAFailedOperationAtExitZero(t *testing.T) {
	mock := &mockStatusGetter{
		bc: makeBulkChange("op-failed-1", "FAILED", 10, 3, 30),
	}

	out, err := setupStatusCmd(t, mock, output.Options{JSONFields: BulkStatusFields}, []string{"op-failed-1"})
	if err != nil {
		t.Fatalf("unexpected error for a FAILED operation: %v", err)
	}

	if strings.Count(out, "\n") != 1 {
		t.Errorf("stdout = %q, want exactly one JSON document", out)
	}

	var item map[string]any
	if unmarshalErr := json.Unmarshal([]byte(out), &item); unmarshalErr != nil {
		t.Fatalf("invalid JSON: %v\nraw: %s", unmarshalErr, out)
	}
	if item["status"] != "FAILED" {
		t.Errorf("status = %v, want FAILED", item["status"])
	}
	if item["totalCompletedIssues"] != float64(3) {
		t.Errorf("totalCompletedIssues = %v, want 3", item["totalCompletedIssues"])
	}
}
