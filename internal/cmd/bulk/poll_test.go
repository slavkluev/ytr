package bulk

import (
	"context"
	"errors"
	"os"
	"testing"
	"time"

	"github.com/slavkluev/go-yandex-tracker/tracker"

	"github.com/slavkluev/ytr/internal/testutil"
)

// --- readIssueKeys tests ---

func TestReadIssueKeys_FromArgs(t *testing.T) {
	keys, err := readIssueKeys([]string{"PROJ-1", "PROJ-2"})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if len(keys) != 2 || keys[0] != "PROJ-1" || keys[1] != "PROJ-2" {
		t.Errorf("expected [PROJ-1, PROJ-2], got %v", keys)
	}
}

func TestReadIssueKeys_FromStdin(t *testing.T) {
	r, w, err := os.Pipe()
	if err != nil {
		t.Fatal(err)
	}

	_, _ = w.WriteString("PROJ-1\nPROJ-2\n")
	w.Close()

	origStdin := stdinFile
	stdinFile = r
	t.Cleanup(func() { stdinFile = origStdin })

	keys, err := readIssueKeys(nil)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if len(keys) != 2 || keys[0] != "PROJ-1" || keys[1] != "PROJ-2" {
		t.Errorf("expected [PROJ-1, PROJ-2], got %v", keys)
	}
}

func TestReadIssueKeys_ArgsOverStdin(t *testing.T) {
	r, w, err := os.Pipe()
	if err != nil {
		t.Fatal(err)
	}

	_, _ = w.WriteString("STDIN-1\n")
	w.Close()

	origStdin := stdinFile
	stdinFile = r
	t.Cleanup(func() { stdinFile = origStdin })

	keys, err := readIssueKeys([]string{"ARG-1"})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if len(keys) != 1 || keys[0] != "ARG-1" {
		t.Errorf("expected [ARG-1], got %v", keys)
	}
}

func TestReadIssueKeys_EmptyStdin(t *testing.T) {
	r, w, err := os.Pipe()
	if err != nil {
		t.Fatal(err)
	}

	_, _ = w.WriteString("")
	w.Close()

	origStdin := stdinFile
	stdinFile = r
	t.Cleanup(func() { stdinFile = origStdin })

	_, err = readIssueKeys(nil)
	if err == nil {
		t.Fatal("expected error for empty stdin, got nil")
	}

	if got := err.Error(); !contains(got, "no issue keys provided") {
		t.Errorf("expected 'no issue keys provided' in error, got: %v", err)
	}
}

func TestReadIssueKeys_InvalidKey(t *testing.T) {
	r, w, err := os.Pipe()
	if err != nil {
		t.Fatal(err)
	}

	_, _ = w.WriteString("invalid-key\n")
	w.Close()

	origStdin := stdinFile
	stdinFile = r
	t.Cleanup(func() { stdinFile = origStdin })

	_, err = readIssueKeys(nil)
	if err == nil {
		t.Fatal("expected error for invalid key, got nil")
	}

	if got := err.Error(); !contains(got, "invalid issue key") {
		t.Errorf("expected 'invalid issue key' in error, got: %v", err)
	}
}

func TestReadIssueKeys_SkipsBlankLines(t *testing.T) {
	r, w, err := os.Pipe()
	if err != nil {
		t.Fatal(err)
	}

	_, _ = w.WriteString("\nPROJ-1\n\n  \nPROJ-2\n")
	w.Close()

	origStdin := stdinFile
	stdinFile = r
	t.Cleanup(func() { stdinFile = origStdin })

	keys, err := readIssueKeys(nil)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if len(keys) != 2 || keys[0] != "PROJ-1" || keys[1] != "PROJ-2" {
		t.Errorf("expected [PROJ-1, PROJ-2], got %v", keys)
	}
}

func TestReadIssueKeys_InvalidArg(t *testing.T) {
	_, err := readIssueKeys([]string{"bad-key"})
	if err == nil {
		t.Fatal("expected error for invalid arg key, got nil")
	}

	if got := err.Error(); !contains(got, "invalid issue key") {
		t.Errorf("expected 'invalid issue key' in error, got: %v", err)
	}
}

// --- parseFieldFlags tests ---

func TestParseFieldFlags_SingleField(t *testing.T) {
	vals, err := parseFieldFlags([]string{"priority=critical"})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if vals["priority"] != "critical" {
		t.Errorf("expected priority=critical, got %v", vals["priority"])
	}
}

func TestParseFieldFlags_MultipleFields(t *testing.T) {
	vals, err := parseFieldFlags([]string{"priority=critical", "status=open"})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if vals["priority"] != "critical" {
		t.Errorf("expected priority=critical, got %v", vals["priority"])
	}
	if vals["status"] != "open" {
		t.Errorf("expected status=open, got %v", vals["status"])
	}
}

func TestParseFieldFlags_ValueWithEquals(t *testing.T) {
	vals, err := parseFieldFlags([]string{"summary=a=b=c"})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if vals["summary"] != "a=b=c" {
		t.Errorf("expected summary=a=b=c, got %v", vals["summary"])
	}
}

func TestParseFieldFlags_EmptyValue(t *testing.T) {
	vals, err := parseFieldFlags([]string{"key="})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if vals["key"] != "" {
		t.Errorf("expected key='', got %v", vals["key"])
	}
}

func TestParseFieldFlags_NoEquals(t *testing.T) {
	_, err := parseFieldFlags([]string{"invalid"})
	if err == nil {
		t.Fatal("expected error for missing =, got nil")
	}

	if got := err.Error(); !contains(got, "invalid field format") {
		t.Errorf("expected 'invalid field format' in error, got: %v", err)
	}
}

func TestParseFieldFlags_EmptyKey(t *testing.T) {
	_, err := parseFieldFlags([]string{"=value"})
	if err == nil {
		t.Fatal("expected error for empty key, got nil")
	}

	if got := err.Error(); !contains(got, "invalid field format") {
		t.Errorf("expected 'invalid field format' in error, got: %v", err)
	}
}

// --- pollUntilDone tests ---

// mockBulkStatusGetter implements bulkStatusGetter for testing.
type mockBulkStatusGetter struct {
	responses []*tracker.BulkChange
	err       error
	callCount int
}

func (m *mockBulkStatusGetter) GetStatus(
	_ context.Context,
	_ string,
) (*tracker.BulkChange, *tracker.Response, error) {
	if m.err != nil {
		return nil, nil, m.err
	}

	idx := m.callCount
	if idx >= len(m.responses) {
		idx = len(m.responses) - 1
	}
	m.callCount++

	return m.responses[idx], &tracker.Response{}, nil
}

func TestPollUntilDone_ImmediateComplete(t *testing.T) {
	_ = testutil.StrPtr // ensure testutil is referenced

	mock := &mockBulkStatusGetter{
		responses: []*tracker.BulkChange{
			{
				ID:     testutil.FlexStringPtr("op-1"),
				Status: testutil.StrPtr("COMPLETED"),
			},
		},
	}

	// Suppress progress output in tests.
	origStderr := stderrFile
	r, w, _ := os.Pipe()
	stderrFile = r
	t.Cleanup(func() {
		stderrFile = origStderr
		w.Close()
		r.Close()
	})

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	bc, err := pollUntilDone(ctx, mock, "op-1", w)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if got := *bc.Status; got != "COMPLETED" {
		t.Errorf("expected COMPLETED, got %s", got)
	}
}

func TestPollUntilDone_CompletesAfterPolling(t *testing.T) {
	mock := &mockBulkStatusGetter{
		responses: []*tracker.BulkChange{
			{
				ID:     testutil.FlexStringPtr("op-2"),
				Status: testutil.StrPtr("RUNNING"),
			},
			{
				ID:     testutil.FlexStringPtr("op-2"),
				Status: testutil.StrPtr("RUNNING"),
			},
			{
				ID:     testutil.FlexStringPtr("op-2"),
				Status: testutil.StrPtr("COMPLETED"),
			},
		},
	}

	origStderr := stderrFile
	r, w, _ := os.Pipe()
	stderrFile = r
	t.Cleanup(func() {
		stderrFile = origStderr
		w.Close()
		r.Close()
	})

	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()

	bc, err := pollUntilDone(ctx, mock, "op-2", w)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if got := *bc.Status; got != "COMPLETED" {
		t.Errorf("expected COMPLETED, got %s", got)
	}

	if mock.callCount != 3 {
		t.Errorf("expected 3 poll calls, got %d", mock.callCount)
	}
}

func TestPollUntilDone_FailedStatus(t *testing.T) {
	mock := &mockBulkStatusGetter{
		responses: []*tracker.BulkChange{
			{
				ID:     testutil.FlexStringPtr("op-fail"),
				Status: testutil.StrPtr("FAILED"),
			},
		},
	}

	origStderr := stderrFile
	r, w, _ := os.Pipe()
	stderrFile = r
	t.Cleanup(func() {
		stderrFile = origStderr
		w.Close()
		r.Close()
	})

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	bc, err := pollUntilDone(ctx, mock, "op-fail", w)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if got := *bc.Status; got != "FAILED" {
		t.Errorf("expected FAILED, got %s", got)
	}
}

func TestPollUntilDone_APIError(t *testing.T) {
	mock := &mockBulkStatusGetter{
		err: errors.New("connection refused"),
	}

	origStderr := stderrFile
	r, w, _ := os.Pipe()
	stderrFile = r
	t.Cleanup(func() {
		stderrFile = origStderr
		w.Close()
		r.Close()
	})

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	_, err := pollUntilDone(ctx, mock, "op-err", w)
	if err == nil {
		t.Fatal("expected error from API, got nil")
	}

	if got := err.Error(); !contains(got, "API request failed") {
		t.Errorf("expected mapped API error, got: %v", err)
	}
}

func TestPollUntilDone_ContextTimeout(t *testing.T) {
	mock := &mockBulkStatusGetter{
		responses: []*tracker.BulkChange{
			{
				ID:     testutil.FlexStringPtr("op-timeout"),
				Status: testutil.StrPtr("RUNNING"),
			},
		},
	}

	origStderr := stderrFile
	r, w, _ := os.Pipe()
	stderrFile = r
	t.Cleanup(func() {
		stderrFile = origStderr
		w.Close()
		r.Close()
	})

	ctx, cancel := context.WithTimeout(context.Background(), 1*time.Millisecond)
	defer cancel()

	// Give the context time to expire before the poll starts.
	time.Sleep(5 * time.Millisecond)

	_, err := pollUntilDone(ctx, mock, "op-timeout", w)
	if err == nil {
		t.Fatal("expected context deadline exceeded, got nil")
	}

	if !errors.Is(err, context.DeadlineExceeded) {
		t.Errorf("expected DeadlineExceeded, got: %v", err)
	}
}

// contains checks if substr is in s.
func contains(s, substr string) bool {
	return len(s) >= len(substr) && containsSubstring(s, substr)
}

func containsSubstring(s, substr string) bool {
	for i := 0; i <= len(s)-len(substr); i++ {
		if s[i:i+len(substr)] == substr {
			return true
		}
	}
	return false
}
