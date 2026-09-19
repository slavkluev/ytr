package issue

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
	ytrerrors "github.com/slavkluev/ytr/internal/errors"
	"github.com/slavkluev/ytr/internal/output"
	"github.com/slavkluev/ytr/internal/testutil"
)

// mockGetter implements issueGetter for testing.
type mockGetter struct {
	issue *tracker.Issue
	resp  *tracker.Response
	err   error
}

func (m *mockGetter) Get(
	_ context.Context,
	_ string,
	_ *tracker.IssueGetOptions,
) (*tracker.Issue, *tracker.Response, error) {
	if m.err != nil {
		return nil, nil, m.err
	}
	return m.issue, m.resp, nil
}

func makeTimestamp(t time.Time) *tracker.Timestamp {
	return &tracker.Timestamp{Time: t}
}

func fullIssue() *tracker.Issue {
	now := time.Now()
	created := now.Add(-3 * 24 * time.Hour)
	updated := now.Add(-2 * time.Hour)

	return &tracker.Issue{
		Key:     testutil.StrPtr("PROJ-123"),
		Summary: testutil.StrPtr("Fix login bug"),
		Status: &tracker.Status{
			Key:     testutil.StrPtr("inProgress"),
			Display: testutil.StrPtr("In Progress"),
		},
		Priority: &tracker.Priority{
			Display: testutil.StrPtr("Critical"),
		},
		Type: &tracker.IssueType{
			Display: testutil.StrPtr("Bug"),
		},
		CreatedBy: &tracker.User{
			Display: testutil.StrPtr("john.doe"),
		},
		Assignee: &tracker.User{
			Display: testutil.StrPtr("jane.smith"),
		},
		CreatedAt:   makeTimestamp(created),
		UpdatedAt:   makeTimestamp(updated),
		Description: testutil.StrPtr("The login page returns 500 error when submitting the form."),
	}
}

func setupViewCmd(t *testing.T, mock *mockGetter, args []string) (string, error) {
	t.Helper()

	origGetter := newGetter
	newGetter = func(_ *config.ResolvedAuth) issueGetter {
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

func TestViewTable(t *testing.T) {
	testutil.ResetOutputFlags(t)

	mock := &mockGetter{
		issue: fullIssue(),
		resp:  &tracker.Response{},
	}

	out, err := setupViewCmd(t, mock, []string{"PROJ-123"})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	for _, want := range []string{
		"Title\tFix login bug",
		"Status\tIn Progress",
		"Priority\tCritical",
		"Type\tBug",
		"Author\tjohn.doe",
		"Assignee\tjane.smith",
		"login page returns 500",
	} {
		if !strings.Contains(out, want) {
			t.Errorf("view output missing %q; got:\n%s", want, out)
		}
	}
}

func TestViewJSON(t *testing.T) {
	testutil.ResetOutputFlags(t)
	output.JSONFields = IssueDetailFields

	mock := &mockGetter{
		issue: fullIssue(),
		resp:  &tracker.Response{},
	}

	out, err := setupViewCmd(t, mock, []string{"PROJ-123"})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	var result map[string]any
	if err := json.Unmarshal([]byte(out), &result); err != nil {
		t.Fatalf("invalid JSON: %v\nraw: %s", err, out)
	}

	// Verify essential fields are present.
	for _, field := range []string{"key", "summary", "status", "priority", "type", "author", "assignee", "createdAt", "updatedAt", "description"} {
		if _, ok := result[field]; !ok {
			t.Errorf("JSON missing field %q", field)
		}
	}

	// Verify dates are ISO 8601 format.
	createdAt, ok := result["createdAt"].(string)
	if !ok || createdAt == "" {
		t.Error("createdAt is empty or not a string")
	}
	if _, err := time.Parse(time.RFC3339, createdAt); err != nil {
		t.Errorf("createdAt is not RFC3339: %q", createdAt)
	}
}

func TestViewQuiet(t *testing.T) {
	testutil.ResetOutputFlags(t)
	output.QuietFlag = true

	mock := &mockGetter{
		issue: fullIssue(),
		resp:  &tracker.Response{},
	}

	out, err := setupViewCmd(t, mock, []string{"PROJ-123"})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	trimmed := strings.TrimSpace(out)
	if trimmed != "PROJ-123" {
		t.Errorf("expected 'PROJ-123', got %q", trimmed)
	}
}

func TestViewNotFound(t *testing.T) {
	testutil.ResetOutputFlags(t)

	// Simulate a not-found error from the API.
	mock := &mockGetter{
		err: errors.New("not found"),
	}

	_, err := setupViewCmd(t, mock, []string{"NOEXIST-1"})
	if err == nil {
		t.Fatal("expected error, got nil")
	}

	// The error should be mapped through api.MapAPIError.
	// Since our simple error won't match tracker.IsNotFound,
	// it will be wrapped. Just verify we get an error back.
	var exitErr *ytrerrors.ExitError
	if errors.As(err, &exitErr) {
		// Good -- it's an ExitError.
	} else if !strings.Contains(err.Error(), "not found") && !strings.Contains(err.Error(), "API request failed") {
		t.Errorf("unexpected error type: %v", err)
	}
}

func TestViewNilFields(t *testing.T) {
	testutil.ResetOutputFlags(t)

	// Issue with many nil fields should render without panic.
	issue := &tracker.Issue{
		Key:     testutil.StrPtr("NIL-1"),
		Summary: testutil.StrPtr("Minimal issue"),
		// Priority, Type, Assignee, CreatedBy, Description all nil
	}

	mock := &mockGetter{
		issue: issue,
		resp:  &tracker.Response{},
	}

	out, err := setupViewCmd(t, mock, []string{"NIL-1"})
	if err != nil {
		t.Fatalf("unexpected error (panic?): %v", err)
	}

	if !strings.Contains(out, "NIL-1") {
		t.Errorf("expected NIL-1 in output, got: %s", out)
	}
	if !strings.Contains(out, "Minimal issue") {
		t.Errorf("expected 'Minimal issue' in output, got: %s", out)
	}
}

func TestViewNoArgs(t *testing.T) {
	testutil.ResetOutputFlags(t)

	mock := &mockGetter{
		issue: fullIssue(),
		resp:  &tracker.Response{},
	}

	_, err := setupViewCmd(t, mock, []string{})
	if err == nil {
		t.Fatal("expected error for no args, got nil")
	}
}

func TestViewTooManyArgs(t *testing.T) {
	testutil.ResetOutputFlags(t)

	mock := &mockGetter{
		issue: fullIssue(),
		resp:  &tracker.Response{},
	}

	_, err := setupViewCmd(t, mock, []string{"PROJ-1", "PROJ-2"})
	if err == nil {
		t.Fatal("expected error for too many args, got nil")
	}
}

func TestViewNamesakesKeepDistinctUserIDs(t *testing.T) {
	testutil.ResetOutputFlags(t)
	output.JSONFields = IssueDetailFields

	// Author and assignee are two different people sharing one display name.
	mock := &mockGetter{
		issue: &tracker.Issue{
			Key:     testutil.StrPtr("PROJ-1"),
			Summary: testutil.StrPtr("Summary"),
			CreatedBy: &tracker.User{
				Display: testutil.StrPtr("Иван Петров"),
				ID:      testutil.FlexStringPtr("uid-author"),
			},
			Assignee: &tracker.User{
				Display: testutil.StrPtr("Иван Петров"),
				ID:      testutil.FlexStringPtr("uid-assignee"),
			},
		},
		resp: &tracker.Response{},
	}

	out, err := setupViewCmd(t, mock, []string{"PROJ-1"})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	var result map[string]any
	if err := json.Unmarshal([]byte(out), &result); err != nil {
		t.Fatalf("invalid JSON: %v\nraw: %s", err, out)
	}

	if result["author"] != result["assignee"] {
		t.Fatalf("test premise broken: display names should collide, got %v and %v",
			result["author"], result["assignee"])
	}
	if result["authorId"] != "uid-author" {
		t.Errorf("expected authorId=uid-author, got %v", result["authorId"])
	}
	if result["assigneeId"] != "uid-assignee" {
		t.Errorf("expected assigneeId=uid-assignee, got %v", result["assigneeId"])
	}
}

func TestViewTableOnTTYUsesLabeledRowsAndRelativeTimes(t *testing.T) {
	testutil.ResetOutputFlags(t)
	output.SetTTY(true)
	t.Setenv("NO_COLOR", "1")

	mock := &mockGetter{
		issue: fullIssue(),
		resp:  &tracker.Response{},
	}

	out, err := setupViewCmd(t, mock, []string{"PROJ-123"})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if !strings.Contains(out, "Key:  PROJ-123") {
		t.Errorf("TTY view lost its labeled rows: %q", out)
	}
	if strings.Contains(out, "\t") {
		t.Errorf("TTY view uses tabs: %q", out)
	}
	if !strings.Contains(out, "ago") {
		t.Errorf("TTY view lost the relative times: %q", out)
	}
}

func TestViewTableOffTTYUsesISOTimes(t *testing.T) {
	testutil.ResetOutputFlags(t)
	output.SetTTY(false)

	created := tracker.Timestamp{
		Time: time.Date(2026, 9, 17, 9, 5, 0, 0, time.FixedZone("MSK", 3*60*60)),
	}
	issue := fullIssue()
	issue.CreatedAt = &created
	issue.UpdatedAt = &created

	out, err := setupViewCmd(t, &mockGetter{issue: issue, resp: &tracker.Response{}}, []string{"PROJ-123"})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if !strings.Contains(out, "Key\tPROJ-123") {
		t.Errorf("off-TTY view is not tab separated: %q", out)
	}
	if !strings.Contains(out, "Created\t2026-09-17T09:05:00+03:00") {
		t.Errorf("off-TTY view lost the ISO created time: %q", out)
	}
	if strings.Contains(out, "ago") {
		t.Errorf("off-TTY view kept a relative time: %q", out)
	}
}
