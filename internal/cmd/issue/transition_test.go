package issue

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

// mockTransitioner implements issueTransitioner for testing.
type mockTransitioner struct {
	transitions []*tracker.Transition
	getResp     *tracker.Response
	getErr      error
	execResp    *tracker.Response
	execErr     error
	execCalls   []mockExecCall
}

type mockExecCall struct {
	issueKey     string
	transitionID string
}

func (m *mockTransitioner) GetTransitions(
	_ context.Context,
	issueKey string,
) ([]*tracker.Transition, *tracker.Response, error) {
	if m.getErr != nil {
		return nil, nil, m.getErr
	}
	return m.transitions, m.getResp, nil
}

func (m *mockTransitioner) ExecuteTransition(
	_ context.Context,
	issueKey string,
	transitionID string,
	req *tracker.TransitionRequest,
) ([]*tracker.Transition, *tracker.Response, error) {
	m.execCalls = append(m.execCalls, mockExecCall{issueKey: issueKey, transitionID: transitionID})
	if m.execErr != nil {
		return nil, nil, m.execErr
	}
	return m.transitions, m.execResp, nil
}

// makeTransitions creates test transition data with To.Key and To.Display set.
func makeTransitions(specs ...struct{ key, display, id string }) []*tracker.Transition {
	result := make([]*tracker.Transition, len(specs))
	for i, spec := range specs {
		result[i] = &tracker.Transition{
			ID:      testutil.FlexStringPtr(spec.id),
			Display: testutil.StrPtr(spec.display),
			To: &tracker.Status{
				Key:     testutil.StrPtr(spec.key),
				Display: testutil.StrPtr(spec.display),
			},
		}
	}
	return result
}

func setupTransitionCmd(t *testing.T, mock *mockTransitioner, args []string) (string, error) {
	t.Helper()

	origTransitioner := newTransitioner
	newTransitioner = func(_ *config.ResolvedAuth) issueTransitioner {
		return mock
	}
	t.Cleanup(func() { newTransitioner = origTransitioner })

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

// sampleTransitions returns a standard set of transitions for tests.
func sampleTransitions() []*tracker.Transition {
	return makeTransitions(
		struct{ key, display, id string }{"open", "Open", "1"},
		struct{ key, display, id string }{"inProgress", "In Progress", "2"},
		struct{ key, display, id string }{"closed", "Closed", "3"},
	)
}

func TestTransitionByKey(t *testing.T) {
	testutil.ResetOutputFlags(t)

	mock := &mockTransitioner{
		transitions: sampleTransitions(),
		getResp:     &tracker.Response{},
		execResp:    &tracker.Response{},
	}

	_, err := setupTransitionCmd(t, mock, []string{"PROJ-123", "--to", "inProgress"})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if len(mock.execCalls) != 1 {
		t.Fatalf("expected 1 ExecuteTransition call, got %d", len(mock.execCalls))
	}
	if mock.execCalls[0].issueKey != "PROJ-123" {
		t.Errorf("expected issueKey=PROJ-123, got %s", mock.execCalls[0].issueKey)
	}
	if mock.execCalls[0].transitionID != "2" {
		t.Errorf("expected transitionID=2, got %s", mock.execCalls[0].transitionID)
	}
}

func TestTransitionByDisplayName(t *testing.T) {
	testutil.ResetOutputFlags(t)

	mock := &mockTransitioner{
		transitions: sampleTransitions(),
		getResp:     &tracker.Response{},
		execResp:    &tracker.Response{},
	}

	_, err := setupTransitionCmd(t, mock, []string{"PROJ-123", "--to", "In Progress"})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if len(mock.execCalls) != 1 {
		t.Fatalf("expected 1 ExecuteTransition call, got %d", len(mock.execCalls))
	}
	if mock.execCalls[0].transitionID != "2" {
		t.Errorf("expected transitionID=2, got %s", mock.execCalls[0].transitionID)
	}
}

func TestTransitionByDisplayNameCaseInsensitive(t *testing.T) {
	testutil.ResetOutputFlags(t)

	mock := &mockTransitioner{
		transitions: sampleTransitions(),
		getResp:     &tracker.Response{},
		execResp:    &tracker.Response{},
	}

	_, err := setupTransitionCmd(t, mock, []string{"PROJ-123", "--to", "in progress"})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if len(mock.execCalls) != 1 {
		t.Fatalf("expected 1 ExecuteTransition call, got %d", len(mock.execCalls))
	}
	if mock.execCalls[0].transitionID != "2" {
		t.Errorf("expected transitionID=2, got %s", mock.execCalls[0].transitionID)
	}
}

func TestTransitionInvalid(t *testing.T) {
	testutil.ResetOutputFlags(t)

	mock := &mockTransitioner{
		transitions: sampleTransitions(),
		getResp:     &tracker.Response{},
	}

	_, err := setupTransitionCmd(t, mock, []string{"PROJ-123", "--to", "nonexistent"})
	if err == nil {
		t.Fatal("expected error for invalid transition, got nil")
	}

	errMsg := err.Error()
	if !strings.Contains(errMsg, "not available") {
		t.Errorf("error %q should contain 'not available'", errMsg)
	}
	if !strings.Contains(errMsg, "PROJ-123") {
		t.Errorf("error %q should contain issue key", errMsg)
	}

	if len(mock.execCalls) != 0 {
		t.Error("ExecuteTransition should not have been called")
	}
}

func TestTransitionInvalidKey(t *testing.T) {
	testutil.ResetOutputFlags(t)

	mock := &mockTransitioner{
		transitions: sampleTransitions(),
		getResp:     &tracker.Response{},
	}

	_, err := setupTransitionCmd(t, mock, []string{"bad-key", "--to", "open"})
	if err == nil {
		t.Fatal("expected validation error for bad key, got nil")
	}

	if !strings.Contains(err.Error(), "invalid issue key") {
		t.Errorf("error %q should contain 'invalid issue key'", err.Error())
	}

	if len(mock.execCalls) != 0 {
		t.Error("no API calls should have been made")
	}
}

func TestTransitionJSON(t *testing.T) {
	testutil.ResetOutputFlags(t)
	output.JSONFields = IssueTransitionFields

	mock := &mockTransitioner{
		transitions: sampleTransitions(),
		getResp:     &tracker.Response{},
		execResp:    &tracker.Response{},
	}

	out, err := setupTransitionCmd(t, mock, []string{"PROJ-123", "--to", "inProgress"})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	var result map[string]any
	if err := json.Unmarshal([]byte(out), &result); err != nil {
		t.Fatalf("invalid JSON output: %v\nraw: %s", err, out)
	}

	if result["key"] != "PROJ-123" {
		t.Errorf("expected key=PROJ-123, got %v", result["key"])
	}
	if result["transition"] != "In Progress" {
		t.Errorf("expected transition='In Progress', got %v", result["transition"])
	}
}

func TestTransitionQuiet(t *testing.T) {
	testutil.ResetOutputFlags(t)
	output.QuietFlag = true

	mock := &mockTransitioner{
		transitions: sampleTransitions(),
		getResp:     &tracker.Response{},
		execResp:    &tracker.Response{},
	}

	out, err := setupTransitionCmd(t, mock, []string{"PROJ-123", "--to", "inProgress"})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	trimmed := strings.TrimSpace(out)
	if trimmed != "PROJ-123" {
		t.Errorf("expected 'PROJ-123', got %q", trimmed)
	}
}

func TestTransitionTable(t *testing.T) {
	testutil.ResetOutputFlags(t)

	mock := &mockTransitioner{
		transitions: sampleTransitions(),
		getResp:     &tracker.Response{},
		execResp:    &tracker.Response{},
	}

	out, err := setupTransitionCmd(t, mock, []string{"PROJ-123", "--to", "inProgress"})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	expected := "PROJ-123 transitioned to In Progress"
	if !strings.Contains(out, expected) {
		t.Errorf("expected output containing %q, got: %s", expected, out)
	}
}

func TestTransitionGetError(t *testing.T) {
	testutil.ResetOutputFlags(t)

	mock := &mockTransitioner{
		getErr: errors.New("api error"),
	}

	_, err := setupTransitionCmd(t, mock, []string{"PROJ-123", "--to", "open"})
	if err == nil {
		t.Fatal("expected error from GetTransitions, got nil")
	}
}

func TestTransitionExecError(t *testing.T) {
	testutil.ResetOutputFlags(t)

	mock := &mockTransitioner{
		transitions: sampleTransitions(),
		getResp:     &tracker.Response{},
		execErr:     errors.New("exec api error"),
	}

	_, err := setupTransitionCmd(t, mock, []string{"PROJ-123", "--to", "inProgress"})
	if err == nil {
		t.Fatal("expected error from ExecuteTransition, got nil")
	}
}
