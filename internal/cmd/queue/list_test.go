package queue

import (
	"bytes"
	"context"
	"encoding/json"
	"strings"
	"testing"

	"github.com/slavkluev/go-yandex-tracker/tracker"

	"github.com/slavkluev/ytr/internal/config"
	"github.com/slavkluev/ytr/internal/output"
	"github.com/slavkluev/ytr/internal/testutil"
)

// mockQueueLister implements queueLister for testing.
type mockQueueLister struct {
	queues []*tracker.Queue
	resp   *tracker.Response
	err    error
	calls  []mockListCall
	// multiPage holds page-indexed results for --all pagination tests.
	multiPage map[int][]*tracker.Queue
}

type mockListCall struct {
	opts *tracker.QueueListOptions
}

func (m *mockQueueLister) List(
	_ context.Context,
	opts *tracker.QueueListOptions,
) ([]*tracker.Queue, *tracker.Response, error) {
	m.calls = append(m.calls, mockListCall{opts: opts})
	if m.err != nil {
		return nil, nil, m.err
	}
	if m.multiPage != nil {
		page := 1
		if opts != nil && opts.Page > 0 {
			page = opts.Page
		}
		queues := m.multiPage[page]
		resp := &tracker.Response{TotalCount: m.resp.TotalCount}
		return queues, resp, nil
	}
	return m.queues, m.resp, nil
}

func makeQueues(keys ...string) []*tracker.Queue {
	queues := make([]*tracker.Queue, len(keys))
	for i, key := range keys {
		name := "Queue " + key
		leadDisplay := "lead-" + key
		queues[i] = &tracker.Queue{
			Key:  testutil.StrPtr(key),
			Name: testutil.StrPtr(name),
			Lead: &tracker.User{
				Display: testutil.StrPtr(leadDisplay),
			},
		}
	}
	return queues
}

func setupListCmd(t *testing.T, mock *mockQueueLister, args []string) (string, error) {
	t.Helper()

	origLister := newLister
	newLister = func(_ *config.ResolvedAuth) queueLister {
		return mock
	}
	t.Cleanup(func() { newLister = origLister })

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

func TestQueueListTable(t *testing.T) {
	testutil.ResetOutputFlags(t)

	mock := &mockQueueLister{
		queues: makeQueues("PROJ", "TEST"),
		resp:   &tracker.Response{TotalCount: 2},
	}

	out, err := setupListCmd(t, mock, []string{})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	for _, want := range []string{"KEY", "NAME", "LEAD", "PROJ", "TEST", "Queue PROJ", "lead-PROJ"} {
		if !strings.Contains(out, want) {
			t.Errorf("table output missing %q; got:\n%s", want, out)
		}
	}
}

func TestQueueListJSON(t *testing.T) {
	testutil.ResetOutputFlags(t)
	output.JSONFields = QueueListFields

	mock := &mockQueueLister{
		queues: makeQueues("PROJ", "TEST"),
		resp:   &tracker.Response{TotalCount: 2},
	}

	out, err := setupListCmd(t, mock, []string{})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	var result map[string]any
	if err := json.Unmarshal([]byte(out), &result); err != nil {
		t.Fatalf("invalid JSON: %v\nraw: %s", err, out)
	}

	items, ok := result["items"].([]any)
	if !ok {
		t.Fatal("JSON missing 'items' array")
	}
	if len(items) != 2 {
		t.Errorf("expected 2 items, got %d", len(items))
	}

	pagination, ok := result["pagination"].(map[string]any)
	if !ok {
		t.Fatal("JSON missing 'pagination' object")
	}
	if _, ok := pagination["hasMore"]; !ok {
		t.Error("pagination missing 'hasMore' field")
	}
}

func TestQueueListQuiet(t *testing.T) {
	testutil.ResetOutputFlags(t)
	output.QuietFlag = true

	mock := &mockQueueLister{
		queues: makeQueues("PROJ", "TEST"),
		resp:   &tracker.Response{TotalCount: 2},
	}

	out, err := setupListCmd(t, mock, []string{})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	lines := strings.Split(strings.TrimSpace(out), "\n")
	if len(lines) != 2 {
		t.Fatalf("expected 2 lines, got %d: %v", len(lines), lines)
	}
	if lines[0] != "PROJ" || lines[1] != "TEST" {
		t.Errorf("expected PROJ and TEST, got %v", lines)
	}
}

func TestQueueListLimit(t *testing.T) {
	testutil.ResetOutputFlags(t)

	mock := &mockQueueLister{
		queues: []*tracker.Queue{},
		resp:   &tracker.Response{},
	}

	_, _ = setupListCmd(t, mock, []string{"--limit", "10"})

	if len(mock.calls) == 0 {
		t.Fatal("no list calls made")
	}
	if mock.calls[0].opts.PerPage != 10 {
		t.Errorf("expected PerPage=10, got %d", mock.calls[0].opts.PerPage)
	}
}

func TestQueueListInvalidCursor(t *testing.T) {
	testutil.ResetOutputFlags(t)

	mock := &mockQueueLister{
		queues: []*tracker.Queue{},
		resp:   &tracker.Response{},
	}

	_, err := setupListCmd(t, mock, []string{"--cursor", "abc"})
	if err == nil {
		t.Fatal("expected error for invalid cursor, got nil")
	}
	if !strings.Contains(err.Error(), "invalid cursor") {
		t.Errorf("expected invalid cursor error, got: %v", err)
	}
	if len(mock.calls) != 0 {
		t.Errorf("expected no API calls, got %d", len(mock.calls))
	}
}

func TestQueueListAll(t *testing.T) {
	testutil.ResetOutputFlags(t)
	output.QuietFlag = true

	// Page 1 returns 2 queues (full page), page 2 returns 1 queue (partial = done).
	mock := &mockQueueLister{
		multiPage: map[int][]*tracker.Queue{
			1: makeQueues("A", "B"),
			2: makeQueues("C"),
		},
		resp: &tracker.Response{TotalCount: 3},
	}

	out, err := setupListCmd(t, mock, []string{"--all", "--limit", "2"})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	lines := strings.Split(strings.TrimSpace(out), "\n")
	if len(lines) != 3 {
		t.Fatalf("expected 3 keys, got %d: %v", len(lines), lines)
	}
	if len(mock.calls) != 2 {
		t.Errorf("expected 2 list calls for pagination, got %d", len(mock.calls))
	}
}

func TestQueueListEmpty_Table(t *testing.T) {
	testutil.ResetOutputFlags(t)

	mock := &mockQueueLister{
		queues: []*tracker.Queue{},
		resp:   &tracker.Response{},
	}

	out, err := setupListCmd(t, mock, []string{})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if !strings.Contains(out, "No queues found") {
		t.Errorf("expected 'No queues found', got: %s", out)
	}
}

func TestQueueListEmpty_JSON(t *testing.T) {
	testutil.ResetOutputFlags(t)
	output.JSONFields = QueueListFields

	mock := &mockQueueLister{
		queues: []*tracker.Queue{},
		resp:   &tracker.Response{},
	}

	out, err := setupListCmd(t, mock, []string{})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	var result map[string]any
	if err := json.Unmarshal([]byte(out), &result); err != nil {
		t.Fatalf("invalid JSON: %v\nraw: %s", err, out)
	}

	items, ok := result["items"].([]any)
	if !ok {
		t.Fatal("items is not an array")
	}
	if len(items) != 0 {
		t.Errorf("expected empty items, got %d", len(items))
	}

	pagination, ok := result["pagination"].(map[string]any)
	if !ok {
		t.Fatal("missing pagination")
	}
	if pagination["hasMore"] != false {
		t.Error("expected hasMore=false")
	}
}

func TestQueueListNilFields(t *testing.T) {
	testutil.ResetOutputFlags(t)

	// Queue with nil Lead should not panic.
	queues := []*tracker.Queue{
		{
			Key:  testutil.StrPtr("NIL-Q"),
			Name: testutil.StrPtr("Queue with nils"),
			// Lead intentionally nil
		},
	}
	mock := &mockQueueLister{
		queues: queues,
		resp:   &tracker.Response{TotalCount: 1},
	}

	out, err := setupListCmd(t, mock, []string{})
	if err != nil {
		t.Fatalf("unexpected error (panic?): %v", err)
	}

	if !strings.Contains(out, "NIL-Q") {
		t.Errorf("expected NIL-Q in output, got: %s", out)
	}
	// Verify fallback "-" is used for nil Lead.
	if !strings.Contains(out, "-") {
		t.Errorf("expected '-' fallback for nil Lead, got: %s", out)
	}
}

func TestQueueList_RegisteredAsSubcommand(t *testing.T) {
	cmd := NewCmd()
	if cmd.Use != "queue" {
		t.Errorf("expected Use='queue', got %q", cmd.Use)
	}

	found := false
	for _, sub := range cmd.Commands() {
		if sub.Use == "list" {
			found = true
			break
		}
	}
	if !found {
		t.Error("'list' not registered as subcommand of 'queue'")
	}
}
