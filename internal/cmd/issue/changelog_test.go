package issue

import (
	"bytes"
	"context"
	"encoding/json"
	"strings"
	"testing"
	"time"

	"github.com/slavkluev/go-yandex-tracker/tracker"

	"github.com/slavkluev/ytr/internal/config"
	"github.com/slavkluev/ytr/internal/output"
	"github.com/slavkluev/ytr/internal/testutil"
)

// mockChangelogGetter implements changelogGetter for testing.
type mockChangelogGetter struct {
	entries  []*tracker.Changelog
	resp     *tracker.Response
	err      error
	lastOpts *tracker.ChangelogOptions // captures last options received
}

func (m *mockChangelogGetter) GetChangelog(
	_ context.Context,
	_ string,
	opts *tracker.ChangelogOptions,
) ([]*tracker.Changelog, *tracker.Response, error) {
	m.lastOpts = opts
	if m.err != nil {
		return nil, nil, m.err
	}
	return m.entries, m.resp, nil
}

// fieldRef creates a *tracker.FieldRef from a field ID string.
func fieldRef(id string) *tracker.FieldRef {
	fid := tracker.FlexString(id)
	return &tracker.FieldRef{ID: &fid}
}

// sampleChangelog returns a slice of Changelog entries with a status change,
// a summary change, and an entry where From is nil (field set for first time).
func sampleChangelog() []*tracker.Changelog {
	ts1 := tracker.Timestamp{Time: time.Date(2024, 3, 15, 10, 0, 0, 0, time.UTC)}
	ts2 := tracker.Timestamp{Time: time.Date(2024, 3, 16, 14, 30, 0, 0, time.UTC)}

	id1 := tracker.FlexString("cl-001")
	id2 := tracker.FlexString("cl-002")

	fieldStatus := fieldRef("status")
	fieldSummary := fieldRef("summary")

	return []*tracker.Changelog{
		{
			ID:        &id1,
			UpdatedAt: &ts1,
			UpdatedBy: &tracker.User{Display: testutil.StrPtr("alice")},
			Fields: []*tracker.ChangelogEvent{
				{
					Field: fieldStatus,
					From:  map[string]any{"display": "Open", "key": "open"},
					To:    map[string]any{"display": "In Progress", "key": "inprogress"},
				},
				{
					Field: fieldSummary,
					From:  "Old title",
					To:    "New title",
				},
			},
		},
		{
			ID:        &id2,
			UpdatedAt: &ts2,
			UpdatedBy: &tracker.User{Display: testutil.StrPtr("bob")},
			Fields: []*tracker.ChangelogEvent{
				{
					Field: fieldStatus,
					From:  nil,
					To:    map[string]any{"display": "Done", "key": "done"},
				},
			},
		},
	}
}

func setupChangelogCmd(t *testing.T, mock *mockChangelogGetter, args []string) (string, error) {
	t.Helper()

	origGetter := newChangelogGetter
	newChangelogGetter = func(_ *config.ResolvedAuth) changelogGetter {
		return mock
	}
	t.Cleanup(func() { newChangelogGetter = origGetter })

	buf := &bytes.Buffer{}
	cmd := newChangelogCmd()
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

func TestChangelogTable(t *testing.T) {
	testutil.ResetOutputFlags(t)

	mock := &mockChangelogGetter{
		entries: sampleChangelog(),
		resp:    &tracker.Response{},
	}

	out, err := setupChangelogCmd(t, mock, []string{"PROJ-123"})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	// Should contain column headers.
	for _, want := range []string{"DATE", "AUTHOR", "FIELD", "FROM", "TO"} {
		if !strings.Contains(out, want) {
			t.Errorf("table missing column %q; got:\n%s", want, out)
		}
	}

	// Should contain data from the changelog entries.
	for _, want := range []string{"alice", "bob", "status", "summary", "Open", "In Progress", "Old title", "New title"} {
		if !strings.Contains(out, want) {
			t.Errorf("table missing value %q; got:\n%s", want, out)
		}
	}
}

func TestChangelogJSON(t *testing.T) {
	testutil.ResetOutputFlags(t)
	output.JSONFields = IssueChangelogFields

	mock := &mockChangelogGetter{
		entries: sampleChangelog(),
		resp:    &tracker.Response{},
	}

	out, err := setupChangelogCmd(t, mock, []string{"PROJ-123"})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	var result map[string]any
	if err := json.Unmarshal([]byte(out), &result); err != nil {
		t.Fatalf("invalid JSON: %v\nraw: %s", err, out)
	}

	// Should have items array and pagination envelope.
	items, ok := result["items"]
	if !ok {
		t.Fatal("JSON missing 'items' key")
	}
	if _, hasPagination := result["pagination"]; !hasPagination {
		t.Fatal("JSON missing 'pagination' key")
	}

	itemSlice, ok := items.([]any)
	if !ok {
		t.Fatalf("items is not an array: %T", items)
	}

	// 2 entries (per-entry, not per-field).
	if len(itemSlice) != 2 {
		t.Errorf("expected 2 items (entries), got %d", len(itemSlice))
	}

	// First entry should have type and fields array.
	firstItem, ok := itemSlice[0].(map[string]any)
	if !ok {
		t.Fatal("first item is not an object")
	}
	for _, field := range []string{"date", "author", "type", "fields"} {
		if _, exists := firstItem[field]; !exists {
			t.Errorf("item missing field %q", field)
		}
	}

	// Fields should be an array with 2 changes (status + summary).
	fields, ok := firstItem["fields"].([]any)
	if !ok {
		t.Fatalf("fields is not an array: %T", firstItem["fields"])
	}
	if len(fields) != 2 {
		t.Errorf("expected 2 fields, got %d", len(fields))
	}

	// Verify raw values: status from/to should be objects with display/key.
	firstField, ok := fields[0].(map[string]any)
	if !ok {
		t.Fatal("first field is not an object")
	}
	fromObj, ok := firstField["from"].(map[string]any)
	if !ok {
		t.Fatalf("from is not an object: %T", firstField["from"])
	}
	if fromObj["display"] != "Open" {
		t.Errorf("expected from.display='Open', got %v", fromObj["display"])
	}
	toObj, ok := firstField["to"].(map[string]any)
	if !ok {
		t.Fatalf("to is not an object: %T", firstField["to"])
	}
	if toObj["display"] != "In Progress" {
		t.Errorf("expected to.display='In Progress', got %v", toObj["display"])
	}
}

func TestChangelogQuiet(t *testing.T) {
	testutil.ResetOutputFlags(t)
	output.QuietFlag = true

	mock := &mockChangelogGetter{
		entries: sampleChangelog(),
		resp:    &tracker.Response{},
	}

	out, err := setupChangelogCmd(t, mock, []string{"PROJ-123"})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	lines := strings.Split(strings.TrimSpace(out), "\n")
	// 3 total events.
	if len(lines) != 3 {
		t.Errorf("expected 3 quiet lines, got %d: %v", len(lines), lines)
	}

	// Each line should be "field: from -> to" format.
	for _, line := range lines {
		if !strings.Contains(line, "->") {
			t.Errorf("quiet line missing '->': %q", line)
		}
		if !strings.Contains(line, ":") {
			t.Errorf("quiet line missing ':': %q", line)
		}
	}
}

func TestChangelogFieldFilter(t *testing.T) {
	testutil.ResetOutputFlags(t)

	mock := &mockChangelogGetter{
		entries: sampleChangelog(),
		resp:    &tracker.Response{},
	}

	_, err := setupChangelogCmd(t, mock, []string{"PROJ-123", "--field", "status"})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	// Verify --field is passed to API as server-side filter.
	if mock.lastOpts == nil || mock.lastOpts.Field != "status" {
		t.Errorf("expected API field filter 'status', got opts: %+v", mock.lastOpts)
	}
}

func TestChangelogFieldFilterCaseInsensitive(t *testing.T) {
	testutil.ResetOutputFlags(t)

	mock := &mockChangelogGetter{
		entries: sampleChangelog(),
		resp:    &tracker.Response{},
	}

	// Use uppercase field name — should be lowercased for API.
	_, err := setupChangelogCmd(t, mock, []string{"PROJ-123", "--field", "STATUS"})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if mock.lastOpts == nil || mock.lastOpts.Field != "status" {
		t.Errorf("expected API field filter 'status' (lowercased), got opts: %+v", mock.lastOpts)
	}
}

func TestChangelogTypeFilter(t *testing.T) {
	testutil.ResetOutputFlags(t)

	mock := &mockChangelogGetter{
		entries: sampleChangelog(),
		resp:    &tracker.Response{},
	}

	_, err := setupChangelogCmd(t, mock, []string{"PROJ-123", "--type", "IssueWorkflow"})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	// Verify --type is passed to API as-is (PascalCase, no lowercasing).
	if mock.lastOpts == nil || mock.lastOpts.Type != "IssueWorkflow" {
		t.Errorf("expected API type filter 'IssueWorkflow', got opts: %+v", mock.lastOpts)
	}
}

func TestChangelogEmpty(t *testing.T) {
	testutil.ResetOutputFlags(t)

	mock := &mockChangelogGetter{
		entries: []*tracker.Changelog{},
		resp:    &tracker.Response{},
	}

	out, err := setupChangelogCmd(t, mock, []string{"PROJ-123"})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if !strings.Contains(out, "No changes found") {
		t.Errorf("expected 'No changes found'; got:\n%s", out)
	}
}

func TestChangelogEmptyJSON(t *testing.T) {
	testutil.ResetOutputFlags(t)
	output.JSONFields = IssueChangelogFields

	mock := &mockChangelogGetter{
		entries: []*tracker.Changelog{},
		resp:    &tracker.Response{},
	}

	out, err := setupChangelogCmd(t, mock, []string{"PROJ-123"})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	var result map[string]any
	if err := json.Unmarshal([]byte(out), &result); err != nil {
		t.Fatalf("invalid JSON: %v\nraw: %s", err, out)
	}

	items, ok := result["items"]
	if !ok {
		t.Fatal("JSON missing 'items' key")
	}

	itemSlice, ok := items.([]any)
	if !ok {
		t.Fatalf("items is not an array: %T", items)
	}

	if len(itemSlice) != 0 {
		t.Errorf("expected empty items array, got %d items", len(itemSlice))
	}
}

func TestChangelogNoArgs(t *testing.T) {
	testutil.ResetOutputFlags(t)

	mock := &mockChangelogGetter{
		entries: sampleChangelog(),
		resp:    &tracker.Response{},
	}

	_, err := setupChangelogCmd(t, mock, []string{})
	if err == nil {
		t.Fatal("expected error for no args, got nil")
	}
}

// paginatingChangelogMock returns different pages based on the cursor option.
// Pages are keyed by cursor ID ("" for first page).
type paginatingChangelogMock struct {
	pages map[string][]*tracker.Changelog
	calls []string // records cursor values received
}

func (m *paginatingChangelogMock) GetChangelog(
	_ context.Context,
	_ string,
	opts *tracker.ChangelogOptions,
) ([]*tracker.Changelog, *tracker.Response, error) {
	cursor := ""
	if opts != nil {
		cursor = opts.ID
	}
	m.calls = append(m.calls, cursor)
	entries := m.pages[cursor]
	return entries, &tracker.Response{}, nil
}

func TestChangelogCursor(t *testing.T) {
	testutil.ResetOutputFlags(t)

	id1 := tracker.FlexString("page1-last")

	mock := &paginatingChangelogMock{
		pages: map[string][]*tracker.Changelog{
			"my-cursor": {
				{
					ID:        &id1,
					UpdatedAt: &tracker.Timestamp{Time: time.Date(2024, 3, 15, 10, 0, 0, 0, time.UTC)},
					UpdatedBy: &tracker.User{Display: testutil.StrPtr("alice")},
					Fields: []*tracker.ChangelogEvent{
						{Field: fieldRef("status"), From: "open", To: "closed"},
					},
				},
			},
		},
	}

	origGetter := newChangelogGetter
	newChangelogGetter = func(_ *config.ResolvedAuth) changelogGetter {
		return mock
	}
	t.Cleanup(func() { newChangelogGetter = origGetter })

	buf := &bytes.Buffer{}
	cmd := newChangelogCmd()
	cmd.SetOut(buf)
	cmd.SetErr(buf)
	cmd.PersistentFlags().String("token", "test-token", "")
	cmd.PersistentFlags().String("org-id", "test-org", "")
	cmd.PersistentFlags().String("org-type", "360", "")
	cmd.SetArgs([]string{"PROJ-123", "--cursor", "my-cursor"})

	if err := cmd.Execute(); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	// Verify the mock received the cursor value.
	if len(mock.calls) != 1 || mock.calls[0] != "my-cursor" {
		t.Errorf("expected cursor 'my-cursor', got calls: %v", mock.calls)
	}

	if !strings.Contains(buf.String(), "alice") {
		t.Errorf("expected output to contain page data; got:\n%s", buf.String())
	}
}

func TestChangelogAll(t *testing.T) {
	testutil.ResetOutputFlags(t)

	id1 := tracker.FlexString("cursor-1")
	id2 := tracker.FlexString("cursor-2")

	// Two pages: page 1 has 1 entry (limit=1 → hasMore), page 2 has 1 entry.
	// We use limit=1 to trigger pagination.
	mock := &paginatingChangelogMock{
		pages: map[string][]*tracker.Changelog{
			"": {
				{
					ID:        &id1,
					UpdatedAt: &tracker.Timestamp{Time: time.Date(2024, 3, 15, 10, 0, 0, 0, time.UTC)},
					UpdatedBy: &tracker.User{Display: testutil.StrPtr("alice")},
					Fields:    []*tracker.ChangelogEvent{{Field: fieldRef("status"), From: "open", To: "inProgress"}},
				},
			},
			"cursor-1": {
				{
					ID:        &id2,
					UpdatedAt: &tracker.Timestamp{Time: time.Date(2024, 3, 16, 14, 0, 0, 0, time.UTC)},
					UpdatedBy: &tracker.User{Display: testutil.StrPtr("bob")},
					Fields:    []*tracker.ChangelogEvent{{Field: fieldRef("status"), From: "inProgress", To: "done"}},
				},
			},
			"cursor-2": {}, // empty → stop
		},
	}

	origGetter := newChangelogGetter
	newChangelogGetter = func(_ *config.ResolvedAuth) changelogGetter {
		return mock
	}
	t.Cleanup(func() { newChangelogGetter = origGetter })

	buf := &bytes.Buffer{}
	cmd := newChangelogCmd()
	cmd.SetOut(buf)
	cmd.SetErr(buf)
	cmd.PersistentFlags().String("token", "test-token", "")
	cmd.PersistentFlags().String("org-id", "test-org", "")
	cmd.PersistentFlags().String("org-type", "360", "")
	cmd.SetArgs([]string{"PROJ-123", "--all", "--limit", "1"})

	if err := cmd.Execute(); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	// Should have paginated through 3 calls: "" → "cursor-1" → "cursor-2" (empty, stop).
	if len(mock.calls) != 3 {
		t.Errorf("expected 3 pagination calls, got %d: %v", len(mock.calls), mock.calls)
	}
	if mock.calls[0] != "" || mock.calls[1] != "cursor-1" || mock.calls[2] != "cursor-2" {
		t.Errorf("unexpected cursor sequence: %v", mock.calls)
	}

	// Output should contain data from both pages.
	out := buf.String()
	if !strings.Contains(out, "alice") || !strings.Contains(out, "bob") {
		t.Errorf("expected data from both pages; got:\n%s", out)
	}
}

// sampleChangelogAllTypes returns entries covering all new event types.
func sampleChangelogAllTypes() []*tracker.Changelog {
	ts := tracker.Timestamp{Time: time.Date(2024, 3, 15, 10, 0, 0, 0, time.UTC)}
	id := tracker.FlexString("cl-100")
	typUpdated := "IssueUpdated"
	typLinked := "IssueLinked"
	typUnlinked := "IssueUnlinked"
	typCommentAdded := "IssueCommentAdded"
	typCommentRemoved := "IssueCommentRemoved"
	typCommentUpdated := "IssueCommentUpdated"
	typReactionAdded := "IssueCommentReactionAdded"
	typReactionRemoved := "IssueCommentReactionRemoved"
	typAttachAdded := "IssueAttachmentAdded"
	typAttachRemoved := "IssueAttachmentRemoved"
	typResolution := "RelatedIssueResolutionChanged"

	commentID := tracker.FlexString("7")
	attachID := tracker.FlexString("4")
	worklogID := tracker.FlexString("1")
	linkTypeRelates := tracker.IssueLinkType{
		ID:      testutil.FlexStringPtr("relates"),
		Inward:  testutil.StrPtr("Related"),
		Outward: testutil.StrPtr("Related"),
	}
	linkTypeDepends := tracker.IssueLinkType{
		ID:      testutil.FlexStringPtr("depends"),
		Inward:  testutil.StrPtr("Blocker"),
		Outward: testutil.StrPtr("Depends on"),
	}
	dirOut := "outward"
	dirIn := "inward"
	linkedIssue := tracker.Issue{Key: testutil.StrPtr("SIG-1"), Display: testutil.StrPtr("Linked issue")}
	depIssue := tracker.Issue{Key: testutil.StrPtr("SIG-7"), Display: testutil.StrPtr("Dep issue")}

	dur1h := tracker.Duration{}
	_ = dur1h.UnmarshalJSON([]byte(`"PT1H"`))
	dur30m := tracker.Duration{}
	_ = dur30m.UnmarshalJSON([]byte(`"PT30M"`))
	wlStart := tracker.Timestamp{Time: time.Date(2024, 3, 15, 12, 0, 0, 0, time.UTC)}

	reaction := "like"
	reactionHeart := "heart"

	return []*tracker.Changelog{
		// Comment added
		{
			ID: &id, UpdatedAt: &ts, UpdatedBy: &tracker.User{Display: testutil.StrPtr("alice")},
			Type: &typCommentAdded,
			Comments: &tracker.ChangelogComments{
				Added: []*tracker.CommentRef{{ID: &commentID, Display: testutil.StrPtr("Test comment")}},
			},
		},
		// Comment removed
		{
			ID: &id, UpdatedAt: &ts, UpdatedBy: &tracker.User{Display: testutil.StrPtr("alice")},
			Type: &typCommentRemoved,
			Comments: &tracker.ChangelogComments{
				Removed: []*tracker.CommentRef{{ID: &commentID, Display: testutil.StrPtr("Old comment")}},
			},
		},
		// Comment updated
		{
			ID: &id, UpdatedAt: &ts, UpdatedBy: &tracker.User{Display: testutil.StrPtr("alice")},
			Type: &typCommentUpdated,
			Comments: &tracker.ChangelogComments{
				Updated: []*tracker.CommentUpdate{{
					Comment: &tracker.CommentRef{ID: &commentID},
					From:    "old text",
					To:      "new text",
				}},
			},
		},
		// Reaction added
		{
			ID: &id, UpdatedAt: &ts, UpdatedBy: &tracker.User{Display: testutil.StrPtr("alice")},
			Type: &typReactionAdded,
			Comments: &tracker.ChangelogComments{
				Updated: []*tracker.CommentUpdate{{
					Comment:       &tracker.CommentRef{ID: &commentID},
					AddedReaction: &reaction,
				}},
			},
		},
		// Reaction removed
		{
			ID: &id, UpdatedAt: &ts, UpdatedBy: &tracker.User{Display: testutil.StrPtr("alice")},
			Type: &typReactionRemoved,
			Comments: &tracker.ChangelogComments{
				Updated: []*tracker.CommentUpdate{{
					Comment:         &tracker.CommentRef{ID: &commentID},
					RemovedReaction: &reactionHeart,
				}},
			},
		},
		// Link added (outward relates)
		{
			ID: &id, UpdatedAt: &ts, UpdatedBy: &tracker.User{Display: testutil.StrPtr("alice")},
			Type: &typLinked,
			Links: []*tracker.ChangelogLink{{
				To: &tracker.ChangelogLinkValue{Direction: &dirOut, Object: &linkedIssue, Type: &linkTypeRelates},
			}},
		},
		// Link removed (inward depends)
		{
			ID: &id, UpdatedAt: &ts, UpdatedBy: &tracker.User{Display: testutil.StrPtr("alice")},
			Type: &typUnlinked,
			Links: []*tracker.ChangelogLink{{
				From: &tracker.ChangelogLinkValue{Direction: &dirIn, Object: &depIssue, Type: &linkTypeDepends},
			}},
		},
		// Attachment added
		{
			ID: &id, UpdatedAt: &ts, UpdatedBy: &tracker.User{Display: testutil.StrPtr("alice")},
			Type: &typAttachAdded,
			Attachments: &tracker.ChangelogAttachments{
				Added: []*tracker.AttachmentRef{{ID: &attachID, Display: testutil.StrPtr("test.txt")}},
			},
		},
		// Attachment removed
		{
			ID: &id, UpdatedAt: &ts, UpdatedBy: &tracker.User{Display: testutil.StrPtr("alice")},
			Type: &typAttachRemoved,
			Attachments: &tracker.ChangelogAttachments{
				Removed: []*tracker.AttachmentRef{{ID: &attachID, Display: testutil.StrPtr("test.txt")}},
			},
		},
		// Worklog added (fields + worklog hybrid)
		{
			ID: &id, UpdatedAt: &ts, UpdatedBy: &tracker.User{Display: testutil.StrPtr("alice")},
			Type: &typUpdated,
			Fields: []*tracker.ChangelogEvent{
				{Field: fieldRef("spent"), From: nil, To: "PT1H"},
			},
			Worklog: []*tracker.ChangelogWorklog{{
				Record: &tracker.WorklogRef{ID: &worklogID, Display: testutil.StrPtr("Work done")},
				To:     &tracker.ChangelogWorklogValue{Duration: &dur1h, Start: &wlStart},
			}},
		},
		// Worklog updated (from + to)
		{
			ID: &id, UpdatedAt: &ts, UpdatedBy: &tracker.User{Display: testutil.StrPtr("alice")},
			Type: &typUpdated,
			Worklog: []*tracker.ChangelogWorklog{{
				Record: &tracker.WorklogRef{ID: &worklogID, Display: testutil.StrPtr("Updated")},
				From:   &tracker.ChangelogWorklogValue{Duration: &dur30m, Start: &wlStart},
				To:     &tracker.ChangelogWorklogValue{Duration: &dur1h, Start: &wlStart},
			}},
		},
		// Related resolution changed
		{
			ID: &id, UpdatedAt: &ts, UpdatedBy: &tracker.User{Display: testutil.StrPtr("alice")},
			Type: &typResolution,
			RelatedResolutions: []*tracker.RelatedResolution{{
				Direction: &dirOut,
				Issue:     &depIssue,
				LinkType:  &linkTypeDepends,
				NewResolution: &tracker.Resolution{
					Key:     testutil.StrPtr("fixed"),
					Display: testutil.StrPtr("Resolved"),
				},
			}},
		},
	}
}

func TestChangelogTableAllTypes(t *testing.T) {
	testutil.ResetOutputFlags(t)

	mock := &mockChangelogGetter{
		entries: sampleChangelogAllTypes(),
		resp:    &tracker.Response{},
	}

	out, err := setupChangelogCmd(t, mock, []string{"PROJ-123"})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	// Verify all event types produce rows with expected field names and values.
	// Use short substrings to survive table column truncation.
	for _, want := range []string{
		"comment", "reaction", "link", "attachment", "worklog", "relatedResolution",
		"Test comment", "Old comment", "like", "heart",
		"Related", "Blocker", "SIG-7",
		"test.txt", "PT1H", "PT30M",
		"Resolve",
	} {
		if !strings.Contains(out, want) {
			t.Errorf("table output missing %q; got:\n%s", want, out)
		}
	}
}

func TestChangelogJSONAllTypes(t *testing.T) {
	testutil.ResetOutputFlags(t)
	output.JSONFields = IssueChangelogFields

	mock := &mockChangelogGetter{
		entries: sampleChangelogAllTypes(),
		resp:    &tracker.Response{},
	}

	out, err := setupChangelogCmd(t, mock, []string{"PROJ-123"})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	var result map[string]any
	if err := json.Unmarshal([]byte(out), &result); err != nil {
		t.Fatalf("invalid JSON: %v\nraw: %s", err, out)
	}

	itemSlice, ok := result["items"].([]any)
	if !ok {
		t.Fatal("items is not an array")
	}

	// 12 entries total.
	if len(itemSlice) != 12 {
		t.Fatalf("expected 12 items, got %d", len(itemSlice))
	}

	// Check comment added entry has comments array.
	entry0 := itemSlice[0].(map[string]any)
	if entry0["type"] != "IssueCommentAdded" {
		t.Errorf("entry 0: expected type IssueCommentAdded, got %v", entry0["type"])
	}
	comments, ok := entry0["comments"].([]any)
	if !ok || len(comments) == 0 {
		t.Fatal("entry 0: missing comments array")
	}
	c := comments[0].(map[string]any)
	if c["action"] != "added" || c["to"] != "Test comment" {
		t.Errorf("entry 0: unexpected comment: %v", c)
	}

	// Check link entry has links array with nested to object.
	entry5 := itemSlice[5].(map[string]any)
	if entry5["type"] != "IssueLinked" {
		t.Errorf("entry 5: expected type IssueLinked, got %v", entry5["type"])
	}
	links, ok := entry5["links"].([]any)
	if !ok || len(links) == 0 {
		t.Fatal("entry 5: missing links array")
	}
	link := links[0].(map[string]any)
	linkTo, ok := link["to"].(map[string]any)
	if !ok {
		t.Fatal("entry 5: link missing 'to' object")
	}
	if linkTo["issue"] != "SIG-1" || linkTo["linkType"] != "relates" || linkTo["linkTypeName"] != "Related" {
		t.Errorf("entry 5: unexpected link to: %v", linkTo)
	}

	// Check worklog entry has both fields and worklog.
	entry9 := itemSlice[9].(map[string]any)
	if _, hasFields := entry9["fields"]; !hasFields {
		t.Error("entry 9 (worklog+fields): missing fields")
	}
	wl, hasWL := entry9["worklog"].([]any)
	if !hasWL || len(wl) == 0 {
		t.Fatal("entry 9: missing worklog array")
	}
	wlItem := wl[0].(map[string]any)
	wlTo, hasTo := wlItem["to"].(map[string]any)
	if !hasTo {
		t.Fatal("entry 9: worklog missing 'to' object")
	}
	if wlTo["duration"] != "PT1H" {
		t.Errorf("entry 9: expected worklog duration PT1H, got %v", wlTo["duration"])
	}

	// Check related resolution entry.
	entry11 := itemSlice[11].(map[string]any)
	if entry11["type"] != "RelatedIssueResolutionChanged" {
		t.Errorf("entry 11: expected type RelatedIssueResolutionChanged, got %v", entry11["type"])
	}
	rr, ok := entry11["relatedResolutions"].([]any)
	if !ok || len(rr) == 0 {
		t.Fatal("entry 11: missing relatedResolutions array")
	}
	rrItem := rr[0].(map[string]any)
	if rrItem["issue"] != "SIG-7" || rrItem["resolution"] != "fixed" || rrItem["linkTypeName"] != "Depends on" {
		t.Errorf("entry 11: unexpected relatedResolution: %v", rrItem)
	}
}

func TestStripSelfURLs(t *testing.T) {
	tests := []struct {
		name  string
		input any
		want  any
	}{
		{"nil", nil, nil},
		{"string passthrough", "hello", "hello"},
		{"number passthrough", float64(42), float64(42)},
		{
			"map removes self",
			map[string]any{"display": "Open", "self": "https://example.com", "key": "open"},
			map[string]any{"display": "Open", "key": "open"},
		},
		{
			"nested map removes self",
			map[string]any{"inner": map[string]any{"self": "url", "id": "1"}},
			map[string]any{"inner": map[string]any{"id": "1"}},
		},
		{
			"array recurses",
			[]any{map[string]any{"self": "url", "display": "A"}, "plain"},
			[]any{map[string]any{"display": "A"}, "plain"},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := stripSelfURLs(tt.input)
			gotJSON, _ := json.Marshal(got)
			wantJSON, _ := json.Marshal(tt.want)
			if string(gotJSON) != string(wantJSON) {
				t.Errorf("stripSelfURLs() = %s, want %s", gotJSON, wantJSON)
			}
		})
	}
}

func TestNormalizeLinkValue(t *testing.T) {
	outward := "outward"
	inward := "inward"

	relatesType := tracker.IssueLinkType{
		ID:      testutil.FlexStringPtr("relates"),
		Inward:  testutil.StrPtr("Related"),
		Outward: testutil.StrPtr("Related"),
	}
	dependsType := tracker.IssueLinkType{
		ID:      testutil.FlexStringPtr("depends"),
		Inward:  testutil.StrPtr("Blocker"),
		Outward: testutil.StrPtr("Depends on"),
	}
	issue := tracker.Issue{Key: testutil.StrPtr("SIG-1"), Display: testutil.StrPtr("Test issue")}

	t.Run("nil returns nil", func(t *testing.T) {
		if got := normalizeLinkValue(nil); got != nil {
			t.Errorf("expected nil, got %+v", got)
		}
	})

	t.Run("outward relates", func(t *testing.T) {
		got := normalizeLinkValue(&tracker.ChangelogLinkValue{
			Direction: &outward, Object: &issue, Type: &relatesType,
		})
		if got.Direction != "outward" || got.Issue != "SIG-1" ||
			got.LinkType != "relates" || got.LinkTypeName != "Related" {
			t.Errorf("unexpected: %+v", got)
		}
	})

	t.Run("inward depends uses Inward name", func(t *testing.T) {
		got := normalizeLinkValue(&tracker.ChangelogLinkValue{
			Direction: &inward, Object: &issue, Type: &dependsType,
		})
		if got.LinkTypeName != "Blocker" {
			t.Errorf("expected linkTypeName='Blocker', got %q", got.LinkTypeName)
		}
	})

	t.Run("outward depends uses Outward name", func(t *testing.T) {
		got := normalizeLinkValue(&tracker.ChangelogLinkValue{
			Direction: &outward, Object: &issue, Type: &dependsType,
		})
		if got.LinkTypeName != "Depends on" {
			t.Errorf("expected linkTypeName='Depends on', got %q", got.LinkTypeName)
		}
	})
}

func TestFormatDurationISO(t *testing.T) {
	t.Run("nil returns empty", func(t *testing.T) {
		if got := formatDurationISO(nil); got != "" {
			t.Errorf("expected empty, got %q", got)
		}
	})

	t.Run("PT1H", func(t *testing.T) {
		d := &tracker.Duration{}
		_ = d.UnmarshalJSON([]byte(`"PT1H"`))
		if got := formatDurationISO(d); got != "PT1H" {
			t.Errorf("expected PT1H, got %q", got)
		}
	})

	t.Run("PT30M", func(t *testing.T) {
		d := &tracker.Duration{}
		_ = d.UnmarshalJSON([]byte(`"PT30M"`))
		if got := formatDurationISO(d); got != "PT30M" {
			t.Errorf("expected PT30M, got %q", got)
		}
	})

	t.Run("P1D", func(t *testing.T) {
		d := &tracker.Duration{}
		_ = d.UnmarshalJSON([]byte(`"P1D"`))
		if got := formatDurationISO(d); got != "P1D" {
			t.Errorf("expected P1D, got %q", got)
		}
	})
}

func TestFormatLinkValueString(t *testing.T) {
	outward := "outward"
	lt := tracker.IssueLinkType{
		ID:      testutil.FlexStringPtr("relates"),
		Outward: testutil.StrPtr("Related"),
	}
	issue := tracker.Issue{Key: testutil.StrPtr("SIG-1")}

	t.Run("nil returns empty", func(t *testing.T) {
		if got := formatLinkValueString(nil); got != "" {
			t.Errorf("expected empty, got %q", got)
		}
	})

	t.Run("formats as LinkTypeName → IssueKey", func(t *testing.T) {
		got := formatLinkValueString(&tracker.ChangelogLinkValue{
			Direction: &outward, Object: &issue, Type: &lt,
		})
		if got != "Related → SIG-1" {
			t.Errorf("expected 'Related → SIG-1', got %q", got)
		}
	})
}

func TestNormalizeChangeValue(t *testing.T) {
	tests := []struct {
		name  string
		input any
		want  string
	}{
		{
			name:  "nil returns empty string",
			input: nil,
			want:  "",
		},
		{
			name:  "string returns as-is",
			input: "hello",
			want:  "hello",
		},
		{
			name:  "float64 formats as number",
			input: float64(42.5),
			want:  "42.5",
		},
		{
			name:  "float64 integer formats without decimal",
			input: float64(100),
			want:  "100",
		},
		{
			name:  "map with display key returns display",
			input: map[string]any{"display": "Open", "key": "open"},
			want:  "Open",
		},
		{
			name:  "map with only key returns key",
			input: map[string]any{"key": "open"},
			want:  "open",
		},
		{
			name:  "map with only id returns id",
			input: map[string]any{"id": "42"},
			want:  "42",
		},
		{
			name:  "array of objects joins display values",
			input: []any{map[string]any{"display": "Alice"}, map[string]any{"display": "Bob"}},
			want:  "Alice, Bob",
		},
		{
			name:  "array of strings joins with comma",
			input: []any{"foo", "bar"},
			want:  "foo, bar",
		},
		{
			name:  "empty array returns empty string",
			input: []any{},
			want:  "",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := normalizeChangeValue(tt.input)
			if got != tt.want {
				t.Errorf("normalizeChangeValue(%v) = %q, want %q", tt.input, got, tt.want)
			}
		})
	}
}
