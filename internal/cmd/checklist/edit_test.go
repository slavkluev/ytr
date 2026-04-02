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

// mockChecklistEditor implements checklistEditor for testing.
type mockChecklistEditor struct {
	issue       *tracker.Issue
	resp        *tracker.Response
	err         error
	gotIssueKey string
	gotItemID   string
	gotReq      *tracker.ChecklistItemRequest
}

func (m *mockChecklistEditor) EditChecklistItem(
	_ context.Context,
	issueKey, itemID string,
	item *tracker.ChecklistItemRequest,
) (*tracker.Issue, *tracker.Response, error) {
	m.gotIssueKey = issueKey
	m.gotItemID = itemID
	m.gotReq = item
	if m.err != nil {
		return nil, nil, m.err
	}
	return m.issue, m.resp, nil
}

func setupEditCmd(t *testing.T, mock *mockChecklistEditor, args []string) (string, error) {
	t.Helper()

	origEditor := newChecklistEditor
	newChecklistEditor = func(_ *config.ResolvedAuth) checklistEditor {
		return mock
	}
	t.Cleanup(func() { newChecklistEditor = origEditor })

	buf := &bytes.Buffer{}
	cmd := newEditCmd()
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

func TestEditText(t *testing.T) {
	testutil.ResetOutputFlags(t)

	mock := &mockChecklistEditor{
		issue: makeIssueWithChecklist(
			&tracker.ChecklistItem{
				ID:      testutil.FlexStringPtr("item-1"),
				Text:    testutil.StrPtr("Updated text"),
				Checked: testutil.BoolPtr(false),
			},
		),
		resp: &tracker.Response{},
	}

	out, err := setupEditCmd(t, mock, []string{"PROJ-123", "item-1", "--text", "Updated text"})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	expected := "Checklist item item-1 updated on PROJ-123"
	if !strings.Contains(out, expected) {
		t.Errorf("expected %q in output, got: %s", expected, out)
	}
}

func TestEditChecked(t *testing.T) {
	testutil.ResetOutputFlags(t)

	mock := &mockChecklistEditor{
		issue: makeIssueWithChecklist(
			&tracker.ChecklistItem{
				ID:      testutil.FlexStringPtr("item-2"),
				Text:    testutil.StrPtr("Task"),
				Checked: testutil.BoolPtr(true),
			},
		),
		resp: &tracker.Response{},
	}

	out, err := setupEditCmd(t, mock, []string{"PROJ-123", "item-2", "--checked"})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if !strings.Contains(out, "Checklist item item-2 updated on PROJ-123") {
		t.Errorf("expected update confirmation, got: %s", out)
	}

	// Verify checked was set to true.
	if mock.gotReq == nil || mock.gotReq.Checked == nil || !*mock.gotReq.Checked {
		t.Error("expected --checked to set Checked=true in request")
	}
}

func TestEditCheckedFalse(t *testing.T) {
	testutil.ResetOutputFlags(t)

	mock := &mockChecklistEditor{
		issue: makeIssueWithChecklist(
			&tracker.ChecklistItem{
				ID:      testutil.FlexStringPtr("item-3"),
				Text:    testutil.StrPtr("Task"),
				Checked: testutil.BoolPtr(false),
			},
		),
		resp: &tracker.Response{},
	}

	out, err := setupEditCmd(t, mock, []string{"PROJ-123", "item-3", "--checked=false"})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if !strings.Contains(out, "Checklist item item-3 updated on PROJ-123") {
		t.Errorf("expected update confirmation, got: %s", out)
	}

	// Verify checked was explicitly set to false.
	if mock.gotReq == nil || mock.gotReq.Checked == nil || *mock.gotReq.Checked {
		t.Error("expected --checked=false to set Checked=false in request")
	}
}

func TestEditAssignee(t *testing.T) {
	testutil.ResetOutputFlags(t)

	mock := &mockChecklistEditor{
		issue: makeIssueWithChecklist(
			&tracker.ChecklistItem{
				ID:       testutil.FlexStringPtr("item-4"),
				Text:     testutil.StrPtr("Task"),
				Checked:  testutil.BoolPtr(false),
				Assignee: &tracker.User{Display: testutil.StrPtr("alice")},
			},
		),
		resp: &tracker.Response{},
	}

	out, err := setupEditCmd(t, mock, []string{"PROJ-123", "item-4", "--assignee", "alice-uid"})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if !strings.Contains(out, "Checklist item item-4 updated on PROJ-123") {
		t.Errorf("expected update confirmation, got: %s", out)
	}

	if mock.gotReq == nil || mock.gotReq.Assignee == nil || *mock.gotReq.Assignee != "alice-uid" {
		t.Error("expected Assignee='alice-uid' in request")
	}
}

func TestEditJSON(t *testing.T) {
	testutil.ResetOutputFlags(t)
	output.JSONFields = ChecklistFields

	mock := &mockChecklistEditor{
		issue: makeIssueWithChecklist(
			&tracker.ChecklistItem{
				ID:      testutil.FlexStringPtr("item-5"),
				Text:    testutil.StrPtr("Edited"),
				Checked: testutil.BoolPtr(true),
			},
		),
		resp: &tracker.Response{},
	}

	out, err := setupEditCmd(t, mock, []string{"PROJ-1", "item-5", "--text", "Edited"})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	var item map[string]any
	if err := json.Unmarshal([]byte(out), &item); err != nil {
		t.Fatalf("invalid JSON: %v\nraw: %s", err, out)
	}

	if item["id"] != "item-5" {
		t.Errorf("expected id=item-5, got %v", item["id"])
	}
	if item["text"] != "Edited" {
		t.Errorf("expected text='Edited', got %v", item["text"])
	}
	if item["checked"] != true {
		t.Errorf("expected checked=true, got %v", item["checked"])
	}
}

func TestEditQuiet(t *testing.T) {
	testutil.ResetOutputFlags(t)
	output.QuietFlag = true

	mock := &mockChecklistEditor{
		issue: makeIssueWithChecklist(
			&tracker.ChecklistItem{
				ID:      testutil.FlexStringPtr("item-6"),
				Text:    testutil.StrPtr("Quiet test"),
				Checked: testutil.BoolPtr(false),
			},
		),
		resp: &tracker.Response{},
	}

	out, err := setupEditCmd(t, mock, []string{"PROJ-1", "item-6", "--text", "Quiet test"})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	trimmed := strings.TrimSpace(out)
	if trimmed != "item-6" {
		t.Errorf("expected 'item-6', got %q", trimmed)
	}
}

func TestEditFromJSON(t *testing.T) {
	testutil.ResetOutputFlags(t)

	mock := &mockChecklistEditor{
		issue: makeIssueWithChecklist(
			&tracker.ChecklistItem{
				ID:      testutil.FlexStringPtr("item-7"),
				Text:    testutil.StrPtr("JSON edit"),
				Checked: testutil.BoolPtr(false),
			},
		),
		resp: &tracker.Response{},
	}

	out, err := setupEditCmd(t, mock, []string{
		"PROJ-1", "item-7", "--from-json", `{"text":"JSON edit"}`,
	})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if !strings.Contains(out, "Checklist item item-7 updated on PROJ-1") {
		t.Errorf("expected update confirmation, got: %s", out)
	}
}

func TestEditMutualExclusion(t *testing.T) {
	testutil.ResetOutputFlags(t)

	mock := &mockChecklistEditor{}

	_, err := setupEditCmd(t, mock, []string{
		"PROJ-1", "item-1", "--text", "Hello", "--from-json", `{"text":"World"}`,
	})
	if err == nil {
		t.Fatal("expected error for mutual exclusion, got nil")
	}

	if !strings.Contains(err.Error(), "cannot use individual flags and --from-json together") {
		t.Errorf("expected mutual exclusion error, got: %v", err)
	}
}

func TestEditNoFlags(t *testing.T) {
	testutil.ResetOutputFlags(t)

	mock := &mockChecklistEditor{}

	_, err := setupEditCmd(t, mock, []string{"PROJ-1", "item-1"})
	if err == nil {
		t.Fatal("expected error for no flags, got nil")
	}

	if !strings.Contains(err.Error(), "at least one of") {
		t.Errorf("expected missing flags error, got: %v", err)
	}
}

func TestEditInvalidKey(t *testing.T) {
	testutil.ResetOutputFlags(t)

	mock := &mockChecklistEditor{}

	_, err := setupEditCmd(t, mock, []string{"bad-key", "item-1", "--text", "test"})
	if err == nil {
		t.Fatal("expected error for invalid key, got nil")
	}

	if !strings.Contains(err.Error(), "invalid issue key") {
		t.Errorf("expected validation error, got: %v", err)
	}
}

func TestEditEmptyItemID(t *testing.T) {
	testutil.ResetOutputFlags(t)

	mock := &mockChecklistEditor{}

	_, err := setupEditCmd(t, mock, []string{"PROJ-1", " ", "--text", "test"})
	if err == nil {
		t.Fatal("expected error for empty item ID, got nil")
	}

	if !strings.Contains(err.Error(), "invalid checklist item ID") {
		t.Errorf("expected validation error, got: %v", err)
	}
}

func TestEditAPIError(t *testing.T) {
	testutil.ResetOutputFlags(t)

	mock := &mockChecklistEditor{
		err: errors.New("connection refused"),
	}

	_, err := setupEditCmd(t, mock, []string{"PROJ-1", "item-1", "--text", "test"})
	if err == nil {
		t.Fatal("expected error from API, got nil")
	}

	if !strings.Contains(err.Error(), "API request failed") {
		t.Errorf("expected mapped API error, got: %v", err)
	}
}

func TestEditItemNotFound(t *testing.T) {
	testutil.ResetOutputFlags(t)

	// Mock returns Issue with different item IDs than requested.
	mock := &mockChecklistEditor{
		issue: makeIssueWithChecklist(
			&tracker.ChecklistItem{
				ID:      testutil.FlexStringPtr("other-item"),
				Text:    testutil.StrPtr("Not the one"),
				Checked: testutil.BoolPtr(false),
			},
		),
		resp: &tracker.Response{},
	}

	_, err := setupEditCmd(t, mock, []string{"PROJ-1", "missing-item", "--text", "test"})
	if err == nil {
		t.Fatal("expected error for item not found, got nil")
	}

	if !strings.Contains(err.Error(), "edited item missing-item not found") {
		t.Errorf("expected item not found error, got: %v", err)
	}
}

func TestEditRequestCapture(t *testing.T) {
	testutil.ResetOutputFlags(t)

	mock := &mockChecklistEditor{
		issue: makeIssueWithChecklist(
			&tracker.ChecklistItem{
				ID:      testutil.FlexStringPtr("item-cap"),
				Text:    testutil.StrPtr("Captured"),
				Checked: testutil.BoolPtr(true),
			},
		),
		resp: &tracker.Response{},
	}

	_, err := setupEditCmd(t, mock, []string{"PROJ-123", "item-cap", "--text", "Captured", "--checked"})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if mock.gotIssueKey != "PROJ-123" {
		t.Errorf("expected issueKey=PROJ-123, got %q", mock.gotIssueKey)
	}
	if mock.gotItemID != "item-cap" {
		t.Errorf("expected itemID=item-cap, got %q", mock.gotItemID)
	}
	if mock.gotReq == nil {
		t.Fatal("expected request to be captured")
	}
	if mock.gotReq.Text == nil || *mock.gotReq.Text != "Captured" {
		t.Errorf("expected Text='Captured', got %v", mock.gotReq.Text)
	}
	if mock.gotReq.Checked == nil || !*mock.gotReq.Checked {
		t.Error("expected Checked=true in captured request")
	}
}
