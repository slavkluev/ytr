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

// mockChecklistCreator implements checklistCreator for testing.
type mockChecklistCreator struct {
	issue       *tracker.Issue
	resp        *tracker.Response
	err         error
	gotIssueKey string
	gotReq      *tracker.ChecklistItemRequest
}

func (m *mockChecklistCreator) CreateChecklistItem(
	_ context.Context,
	issueKey string,
	item *tracker.ChecklistItemRequest,
) (*tracker.Issue, *tracker.Response, error) {
	m.gotIssueKey = issueKey
	m.gotReq = item
	if m.err != nil {
		return nil, nil, m.err
	}
	return m.issue, m.resp, nil
}

func makeIssueWithChecklist(items ...*tracker.ChecklistItem) *tracker.Issue {
	return &tracker.Issue{
		ChecklistItems: items,
	}
}

func setupCreateCmd(t *testing.T, mock *mockChecklistCreator, args []string) (string, error) {
	t.Helper()

	origCreator := newChecklistCreator
	newChecklistCreator = func(_ *config.ResolvedAuth) checklistCreator {
		return mock
	}
	t.Cleanup(func() { newChecklistCreator = origCreator })

	buf := &bytes.Buffer{}
	cmd := newCreateCmd()
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

func TestCreateWithText(t *testing.T) {
	testutil.ResetOutputFlags(t)

	mock := &mockChecklistCreator{
		issue: makeIssueWithChecklist(
			&tracker.ChecklistItem{
				ID:      testutil.FlexStringPtr("item-new"),
				Text:    testutil.StrPtr("Review PR"),
				Checked: testutil.BoolPtr(false),
			},
		),
		resp: &tracker.Response{},
	}

	out, err := setupCreateCmd(t, mock, []string{"PROJ-123", "--text", "Review PR"})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	expected := "Checklist item item-new created on PROJ-123"
	if !strings.Contains(out, expected) {
		t.Errorf("expected %q in output, got: %s", expected, out)
	}

	if mock.gotIssueKey != "PROJ-123" {
		t.Errorf("expected issueKey=PROJ-123, got %q", mock.gotIssueKey)
	}
}

func TestCreateWithTextAndAssignee(t *testing.T) {
	testutil.ResetOutputFlags(t)

	mock := &mockChecklistCreator{
		issue: makeIssueWithChecklist(
			&tracker.ChecklistItem{
				ID:       testutil.FlexStringPtr("item-a"),
				Text:     testutil.StrPtr("Deploy"),
				Checked:  testutil.BoolPtr(false),
				Assignee: &tracker.User{Display: testutil.StrPtr("alice")},
			},
		),
		resp: &tracker.Response{},
	}

	out, err := setupCreateCmd(t, mock, []string{"PROJ-1", "--text", "Deploy", "--assignee", "12345"})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if !strings.Contains(out, "Checklist item item-a created on PROJ-1") {
		t.Errorf("expected creation confirmation, got: %s", out)
	}
}

func TestCreateJSON(t *testing.T) {
	testutil.ResetOutputFlags(t)
	output.JSONFields = ChecklistFields

	mock := &mockChecklistCreator{
		issue: makeIssueWithChecklist(
			&tracker.ChecklistItem{
				ID:      testutil.FlexStringPtr("item-42"),
				Text:    testutil.StrPtr("Review PR"),
				Checked: testutil.BoolPtr(false),
			},
		),
		resp: &tracker.Response{},
	}

	out, err := setupCreateCmd(t, mock, []string{"PROJ-1", "--text", "Review PR"})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	var item map[string]any
	if err := json.Unmarshal([]byte(out), &item); err != nil {
		t.Fatalf("invalid JSON: %v\nraw: %s", err, out)
	}

	if item["id"] != "item-42" {
		t.Errorf("expected id=item-42, got %v", item["id"])
	}
	if item["text"] != "Review PR" {
		t.Errorf("expected text='Review PR', got %v", item["text"])
	}
}

func TestCreateQuiet(t *testing.T) {
	testutil.ResetOutputFlags(t)
	output.QuietFlag = true

	mock := &mockChecklistCreator{
		issue: makeIssueWithChecklist(
			&tracker.ChecklistItem{
				ID:      testutil.FlexStringPtr("item-99"),
				Text:    testutil.StrPtr("Test"),
				Checked: testutil.BoolPtr(false),
			},
		),
		resp: &tracker.Response{},
	}

	out, err := setupCreateCmd(t, mock, []string{"PROJ-1", "--text", "Test"})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	trimmed := strings.TrimSpace(out)
	if trimmed != "item-99" {
		t.Errorf("expected 'item-99', got %q", trimmed)
	}
}

func TestCreateFromJSON(t *testing.T) {
	testutil.ResetOutputFlags(t)

	mock := &mockChecklistCreator{
		issue: makeIssueWithChecklist(
			&tracker.ChecklistItem{
				ID:      testutil.FlexStringPtr("item-json"),
				Text:    testutil.StrPtr("From JSON"),
				Checked: testutil.BoolPtr(false),
			},
		),
		resp: &tracker.Response{},
	}

	out, err := setupCreateCmd(t, mock, []string{
		"PROJ-1", "--from-json", `{"text":"From JSON"}`,
	})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if !strings.Contains(out, "Checklist item item-json created on PROJ-1") {
		t.Errorf("expected creation confirmation, got: %s", out)
	}
}

func TestCreateMutualExclusion(t *testing.T) {
	testutil.ResetOutputFlags(t)

	mock := &mockChecklistCreator{}

	_, err := setupCreateCmd(t, mock, []string{
		"PROJ-1", "--text", "Hello", "--from-json", `{"text":"World"}`,
	})
	if err == nil {
		t.Fatal("expected error for mutual exclusion, got nil")
	}

	if !strings.Contains(err.Error(), "cannot use individual flags and --from-json together") {
		t.Errorf("expected mutual exclusion error, got: %v", err)
	}
}

func TestCreateMissingText(t *testing.T) {
	testutil.ResetOutputFlags(t)

	mock := &mockChecklistCreator{}

	_, err := setupCreateCmd(t, mock, []string{"PROJ-1"})
	if err == nil {
		t.Fatal("expected error for missing --text, got nil")
	}

	if !strings.Contains(err.Error(), "--text or --from-json is required") {
		t.Errorf("expected missing text error, got: %v", err)
	}
}

func TestCreateAPIError(t *testing.T) {
	testutil.ResetOutputFlags(t)

	mock := &mockChecklistCreator{
		err: errors.New("connection refused"),
	}

	_, err := setupCreateCmd(t, mock, []string{"PROJ-1", "--text", "test"})
	if err == nil {
		t.Fatal("expected error from API, got nil")
	}

	if !strings.Contains(err.Error(), "API request failed") {
		t.Errorf("expected mapped API error, got: %v", err)
	}
}

// TestCreateMatchesByText verifies the created item is identified by its text,
// not by assuming it is the last element. The API does not guarantee item order.
func TestCreateMatchesByText(t *testing.T) {
	testutil.ResetOutputFlags(t)

	// "Review PR" is NOT the last item; matching by text must still pick it.
	mock := &mockChecklistCreator{
		issue: makeIssueWithChecklist(
			&tracker.ChecklistItem{ID: testutil.FlexStringPtr("item-old"), Text: testutil.StrPtr("Old task")},
			&tracker.ChecklistItem{ID: testutil.FlexStringPtr("item-new"), Text: testutil.StrPtr("Review PR")},
			&tracker.ChecklistItem{ID: testutil.FlexStringPtr("item-other"), Text: testutil.StrPtr("Another task")},
		),
		resp: &tracker.Response{},
	}

	out, err := setupCreateCmd(t, mock, []string{"PROJ-1", "--text", "Review PR"})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if !strings.Contains(out, "Checklist item item-new created on PROJ-1") {
		t.Errorf("expected item-new (matched by text), got: %s", out)
	}
}

// TestCreateEmptyChecklistItemsBestEffort verifies that a successful mutation
// whose response carries no checklist items does NOT fail. A non-zero exit would
// make agents retry and create duplicates (create isn't idempotent); instead the
// command reports a best-effort confirmation built from the request.
func TestCreateEmptyChecklistItemsBestEffort(t *testing.T) {
	testutil.ResetOutputFlags(t)
	output.JSONFields = ChecklistFields

	mock := &mockChecklistCreator{
		issue: &tracker.Issue{
			ChecklistItems: []*tracker.ChecklistItem{},
		},
		resp: &tracker.Response{},
	}

	out, err := setupCreateCmd(t, mock, []string{"PROJ-1", "--text", "test"})
	if err != nil {
		t.Fatalf("expected no error (mutation succeeded), got: %v", err)
	}

	var item map[string]any
	if err := json.Unmarshal([]byte(out), &item); err != nil {
		t.Fatalf("invalid JSON: %v\nraw: %s", err, out)
	}
	if item["text"] != "test" {
		t.Errorf("expected best-effort text='test' from request, got %v", item["text"])
	}
}

func TestCreateRequestCapture(t *testing.T) {
	testutil.ResetOutputFlags(t)

	mock := &mockChecklistCreator{
		issue: makeIssueWithChecklist(
			&tracker.ChecklistItem{
				ID:      testutil.FlexStringPtr("item-cap"),
				Text:    testutil.StrPtr("Capture test"),
				Checked: testutil.BoolPtr(false),
			},
		),
		resp: &tracker.Response{},
	}

	_, err := setupCreateCmd(t, mock, []string{"PROJ-123", "--text", "Capture test", "--assignee", "user1"})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if mock.gotIssueKey != "PROJ-123" {
		t.Errorf("expected issueKey=PROJ-123, got %q", mock.gotIssueKey)
	}
	if mock.gotReq == nil {
		t.Fatal("expected request to be captured")
	}
	if mock.gotReq.Text == nil || *mock.gotReq.Text != "Capture test" {
		t.Errorf("expected Text='Capture test', got %v", mock.gotReq.Text)
	}
	if mock.gotReq.Assignee == nil || *mock.gotReq.Assignee != "user1" {
		t.Errorf("expected Assignee='user1', got %v", mock.gotReq.Assignee)
	}
}
