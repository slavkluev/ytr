package checklist

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"strings"
	"testing"

	"github.com/slavkluev/go-yandex-tracker/tracker"

	"github.com/slavkluev/ytr/internal/config"
	"github.com/slavkluev/ytr/internal/output"
	"github.com/slavkluev/ytr/internal/testutil"
)

// mockChecklistLister implements checklistLister for testing.
type mockChecklistLister struct {
	items       []*tracker.ChecklistItem
	resp        *tracker.Response
	err         error
	gotIssueKey string
}

func (m *mockChecklistLister) ListChecklistItems(
	_ context.Context,
	issueKey string,
) ([]*tracker.ChecklistItem, *tracker.Response, error) {
	m.gotIssueKey = issueKey
	if m.err != nil {
		return nil, nil, m.err
	}
	return m.items, m.resp, nil
}

func makeChecklistItems(ids ...string) []*tracker.ChecklistItem {
	items := make([]*tracker.ChecklistItem, len(ids))
	for i, id := range ids {
		checked := i%2 == 0
		items[i] = &tracker.ChecklistItem{
			ID:      testutil.FlexStringPtr(id),
			Text:    testutil.StrPtr("Item " + id),
			Checked: testutil.BoolPtr(checked),
		}
	}
	return items
}

func setupListCmd(t *testing.T, mock *mockChecklistLister, args []string) (string, error) {
	t.Helper()

	origLister := newChecklistLister
	newChecklistLister = func(_ *config.ResolvedAuth) checklistLister {
		return mock
	}
	t.Cleanup(func() { newChecklistLister = origLister })

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

func TestListTable(t *testing.T) {
	testutil.ResetOutputFlags(t)

	mock := &mockChecklistLister{
		items: []*tracker.ChecklistItem{
			{
				ID:       testutil.FlexStringPtr("item-1"),
				Text:     testutil.StrPtr("Review code"),
				Checked:  testutil.BoolPtr(true),
				Assignee: &tracker.User{Display: testutil.StrPtr("alice")},
			},
			{
				ID:      testutil.FlexStringPtr("item-2"),
				Text:    testutil.StrPtr("Write tests"),
				Checked: testutil.BoolPtr(false),
			},
		},
		resp: &tracker.Response{},
	}

	out, err := setupListCmd(t, mock, []string{"PROJ-123"})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	for _, want := range []string{"ID", "TEXT", "CHECKED", "ASSIGNEE", "item-1", "item-2", "Review code", "Write tests", "yes", "no", "alice"} {
		if !strings.Contains(out, want) {
			t.Errorf("table output missing %q; got:\n%s", want, out)
		}
	}

	if mock.gotIssueKey != "PROJ-123" {
		t.Errorf("expected issueKey=PROJ-123, got %q", mock.gotIssueKey)
	}
}

func TestListEmpty(t *testing.T) {
	testutil.ResetOutputFlags(t)

	mock := &mockChecklistLister{
		items: []*tracker.ChecklistItem{},
		resp:  &tracker.Response{},
	}

	out, err := setupListCmd(t, mock, []string{"PROJ-1"})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if !strings.Contains(out, "No checklist items found") {
		t.Errorf("expected 'No checklist items found', got: %s", out)
	}
}

func TestListJSON(t *testing.T) {
	testutil.ResetOutputFlags(t)
	output.JSONFields = ChecklistFields

	mock := &mockChecklistLister{
		items: []*tracker.ChecklistItem{
			{
				ID:       testutil.FlexStringPtr("item-42"),
				Text:     testutil.StrPtr("Deploy service"),
				Checked:  testutil.BoolPtr(true),
				Assignee: &tracker.User{Display: testutil.StrPtr("bob")},
			},
		},
		resp: &tracker.Response{},
	}

	out, err := setupListCmd(t, mock, []string{"PROJ-1"})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	var items []map[string]any
	if err := json.Unmarshal([]byte(out), &items); err != nil {
		t.Fatalf("invalid JSON array: %v\nraw: %s", err, out)
	}

	if len(items) != 1 {
		t.Fatalf("expected 1 item, got %d", len(items))
	}

	item := items[0]
	if item["id"] != "item-42" {
		t.Errorf("expected id=item-42, got %v", item["id"])
	}
	if item["text"] != "Deploy service" {
		t.Errorf("expected text='Deploy service', got %v", item["text"])
	}
	if item["checked"] != true {
		t.Errorf("expected checked=true, got %v", item["checked"])
	}
	if item["assignee"] != "bob" {
		t.Errorf("expected assignee=bob, got %v", item["assignee"])
	}
}

func TestListQuiet(t *testing.T) {
	testutil.ResetOutputFlags(t)
	output.QuietFlag = true

	mock := &mockChecklistLister{
		items: makeChecklistItems("a1", "b2", "c3"),
		resp:  &tracker.Response{},
	}

	out, err := setupListCmd(t, mock, []string{"PROJ-1"})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	lines := strings.Split(strings.TrimSpace(out), "\n")
	if len(lines) != 3 {
		t.Fatalf("expected 3 lines, got %d: %v", len(lines), lines)
	}
	if lines[0] != "a1" || lines[1] != "b2" || lines[2] != "c3" {
		t.Errorf("expected a1, b2, c3 got %v", lines)
	}
}

func TestListJQFilter(t *testing.T) {
	testutil.ResetOutputFlags(t)
	output.JQFilter = ".[].id"

	mock := &mockChecklistLister{
		items: makeChecklistItems("x1", "x2"),
		resp:  &tracker.Response{},
	}

	out, err := setupListCmd(t, mock, []string{"PROJ-1"})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	trimmed := strings.TrimSpace(out)
	if !strings.Contains(trimmed, "x1") || !strings.Contains(trimmed, "x2") {
		t.Errorf("expected jq output with x1 and x2, got: %s", trimmed)
	}
}

func TestListInvalidKey(t *testing.T) {
	testutil.ResetOutputFlags(t)

	mock := &mockChecklistLister{}

	_, err := setupListCmd(t, mock, []string{"bad-key"})
	if err == nil {
		t.Fatal("expected error for invalid key, got nil")
	}

	if !strings.Contains(err.Error(), "invalid issue key") {
		t.Errorf("expected validation error, got: %v", err)
	}
}

func TestListAPIError(t *testing.T) {
	testutil.ResetOutputFlags(t)

	mock := &mockChecklistLister{
		err: errors.New("connection refused"),
	}

	_, err := setupListCmd(t, mock, []string{"PROJ-1"})
	if err == nil {
		t.Fatal("expected error from API, got nil")
	}

	if !strings.Contains(err.Error(), "API request failed") {
		t.Errorf("expected mapped API error, got: %v", err)
	}
}
