package queue

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"strings"
	"testing"

	"github.com/slavkluev/go-yandex-tracker/tracker"

	"github.com/slavkluev/ytr/internal/config"
	ytrerrors "github.com/slavkluev/ytr/internal/errors"
	"github.com/slavkluev/ytr/internal/output"
	"github.com/slavkluev/ytr/internal/testutil"
)

// mockQueueGetter implements queueGetter for testing.
type mockQueueGetter struct {
	queue *tracker.Queue
	resp  *tracker.Response
	err   error
}

func (m *mockQueueGetter) Get(
	_ context.Context,
	_ string,
	_ *tracker.QueueGetOptions,
) (*tracker.Queue, *tracker.Response, error) {
	if m.err != nil {
		return nil, nil, m.err
	}
	return m.queue, m.resp, nil
}

func fullQueue() *tracker.Queue {
	return &tracker.Queue{
		Key:         testutil.StrPtr("MYQUEUE"),
		Name:        testutil.StrPtr("My Queue"),
		Description: testutil.StrPtr("Queue for tracking tasks"),
		Lead: &tracker.User{
			Display: testutil.StrPtr("john.doe"),
		},
		DefaultType: &tracker.IssueType{
			Display: testutil.StrPtr("Task"),
		},
		DefaultPriority: &tracker.Priority{
			Display: testutil.StrPtr("Normal"),
		},
		AssignAuto:     testutil.BoolPtr(true),
		AllowExternals: testutil.BoolPtr(false),
	}
}

func setupViewCmd(t *testing.T, mock *mockQueueGetter, args []string) (string, error) {
	t.Helper()

	origGetter := newGetter
	newGetter = func(_ *config.ResolvedAuth) queueGetter {
		return mock
	}
	t.Cleanup(func() { newGetter = origGetter })

	buf := &bytes.Buffer{}
	cmd := newViewCmd()
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

func TestQueueViewTable(t *testing.T) {
	testutil.ResetOutputFlags(t)

	mock := &mockQueueGetter{
		queue: fullQueue(),
		resp:  &tracker.Response{},
	}

	out, err := setupViewCmd(t, mock, []string{"MYQUEUE"})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	for _, want := range []string{
		"MYQUEUE",
		"My Queue",
		"john.doe",
		"Task",
		"Normal",
		"Queue for tracking tasks",
	} {
		if !strings.Contains(out, want) {
			t.Errorf("view output missing %q; got:\n%s", want, out)
		}
	}
}

func TestQueueViewJSON(t *testing.T) {
	testutil.ResetOutputFlags(t)
	output.JSONFields = QueueDetailFields

	mock := &mockQueueGetter{
		queue: fullQueue(),
		resp:  &tracker.Response{},
	}

	out, err := setupViewCmd(t, mock, []string{"MYQUEUE"})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	var result map[string]any
	if err := json.Unmarshal([]byte(out), &result); err != nil {
		t.Fatalf("invalid JSON: %v\nraw: %s", err, out)
	}

	// Verify essential fields are present.
	for _, field := range []string{"key", "name", "description", "lead", "defaultType", "defaultPriority"} {
		if _, ok := result[field]; !ok {
			t.Errorf("JSON missing field %q", field)
		}
	}

	if result["key"] != "MYQUEUE" {
		t.Errorf("expected key=MYQUEUE, got %v", result["key"])
	}
	if result["assignAuto"] != true {
		t.Errorf("expected assignAuto=true, got %v", result["assignAuto"])
	}
}

func TestQueueViewQuiet(t *testing.T) {
	testutil.ResetOutputFlags(t)
	output.QuietFlag = true

	mock := &mockQueueGetter{
		queue: fullQueue(),
		resp:  &tracker.Response{},
	}

	out, err := setupViewCmd(t, mock, []string{"MYQUEUE"})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	trimmed := strings.TrimSpace(out)
	if trimmed != "MYQUEUE" {
		t.Errorf("expected 'MYQUEUE', got %q", trimmed)
	}
}

func TestQueueViewNotFound(t *testing.T) {
	testutil.ResetOutputFlags(t)

	// Simulate a not-found error from the API.
	mock := &mockQueueGetter{
		err: errors.New("not found"),
	}

	_, err := setupViewCmd(t, mock, []string{"NOEXIST"})
	if err == nil {
		t.Fatal("expected error, got nil")
	}

	// The error should be mapped through api.MapAPIError.
	var exitErr *ytrerrors.ExitError
	if errors.As(err, &exitErr) {
		// Good -- it's an ExitError.
	} else if !strings.Contains(err.Error(), "not found") && !strings.Contains(err.Error(), "API request failed") {
		t.Errorf("unexpected error type: %v", err)
	}
}

func TestQueueViewNilFields(t *testing.T) {
	testutil.ResetOutputFlags(t)

	// Queue with nil Description, Lead, DefaultType, DefaultPriority should not panic.
	queue := &tracker.Queue{
		Key:  testutil.StrPtr("NIL-Q"),
		Name: testutil.StrPtr("Minimal queue"),
		// Description, Lead, DefaultType, DefaultPriority all nil
	}

	mock := &mockQueueGetter{
		queue: queue,
		resp:  &tracker.Response{},
	}

	out, err := setupViewCmd(t, mock, []string{"NIL-Q"})
	if err != nil {
		t.Fatalf("unexpected error (panic?): %v", err)
	}

	if !strings.Contains(out, "NIL-Q") {
		t.Errorf("expected NIL-Q in output, got: %s", out)
	}
	if !strings.Contains(out, "Minimal queue") {
		t.Errorf("expected 'Minimal queue' in output, got: %s", out)
	}
}

func TestQueueViewNoArgs(t *testing.T) {
	testutil.ResetOutputFlags(t)

	mock := &mockQueueGetter{
		queue: fullQueue(),
		resp:  &tracker.Response{},
	}

	_, err := setupViewCmd(t, mock, []string{})
	if err == nil {
		t.Fatal("expected error for no args, got nil")
	}
}
