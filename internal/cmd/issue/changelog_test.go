package issue

import (
	"bytes"
	"context"
	"encoding/json"
	"slices"
	"strings"
	"testing"
	"time"

	"github.com/slavkluev/go-yandex-tracker/tracker"

	"github.com/slavkluev/ytr/internal/config"
	"github.com/slavkluev/ytr/internal/output"
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

func setupChangelogCmd(t *testing.T, mock *mockChangelogGetter, opts output.Options, args []string) (string, error) {
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
	err := cmd.ExecuteContext(output.NewContext(t.Context(), &opts))
	return buf.String(), err
}

func TestChangelogTable(t *testing.T) {
	mock := &mockChangelogGetter{
		entries: sampleChangelog(),
		resp:    &tracker.Response{},
	}

	out, err := setupChangelogCmd(t, mock, output.Options{}, []string{"PROJ-123"})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	lines := strings.Split(out, "\n")
	want := []string{
		"DATE\tAUTHOR\tFIELD\tFROM\tTO",
		"2024-03-15T10:00:00Z\talice\tstatus\tOpen\tIn Progress",
	}
	if len(lines) < len(want) || !slices.Equal(lines[:len(want)], want) {
		t.Errorf("table does not start with the header and the first change; got:\n%s", out)
	}
}

func TestChangelogQuiet(t *testing.T) {
	mock := &mockChangelogGetter{
		entries: sampleChangelog(),
		resp:    &tracker.Response{},
	}

	out, err := setupChangelogCmd(t, mock, output.Options{Quiet: true}, []string{"PROJ-123"})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	want := "status: Open -> In Progress\n" +
		"summary: Old title -> New title\n" +
		"status:  -> Done\n"
	if out != want {
		t.Errorf("quiet output = %q, want %q", out, want)
	}
}

func TestChangelogJSONIsTheDocument(t *testing.T) {
	entries := sampleChangelog()
	mock := &mockChangelogGetter{entries: entries, resp: &tracker.Response{}}
	fields := []string{"date", "author", "fields"}

	out, err := setupChangelogCmd(t, mock, output.Options{JSONFields: fields}, []string{"PROJ-123", "--limit", "2"})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	page := output.PaginationMeta{Cursor: "cl-002", HasMore: true}
	want, err := json.Marshal(changelogDocument(entries, fields, page))
	if err != nil {
		t.Fatalf("marshal document: %v", err)
	}
	if out != string(want)+"\n" {
		t.Errorf("stdout is not the changelog document\ngot:  %s\nwant: %s", out, want)
	}
}

func TestChangelogJQ(t *testing.T) {
	mock := &mockChangelogGetter{entries: sampleChangelog(), resp: &tracker.Response{}}

	out, err := setupChangelogCmd(t, mock, output.Options{JQFilter: ".items[].author"}, []string{"PROJ-123"})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if want := "alice\nbob\n"; out != want {
		t.Errorf("jq output = %q, want %q", out, want)
	}
}

func TestChangelogFieldFilter(t *testing.T) {
	mock := &mockChangelogGetter{
		entries: sampleChangelog(),
		resp:    &tracker.Response{},
	}

	_, err := setupChangelogCmd(t, mock, output.Options{}, []string{"PROJ-123", "--field", "status"})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	// Verify --field is passed to API as server-side filter.
	if mock.lastOpts == nil || mock.lastOpts.Field != "status" {
		t.Errorf("expected API field filter 'status', got opts: %+v", mock.lastOpts)
	}
}

// pagingChangelogGetter returns a fixed sequence of pages, one per call, so the
// auto-pagination loop in fetchAllChangelog can be exercised across page
// boundaries. It records the options received on each call.
type pagingChangelogGetter struct {
	pages [][]*tracker.Changelog
	call  int
	opts  []*tracker.ChangelogOptions
}

func (m *pagingChangelogGetter) GetChangelog(
	_ context.Context,
	_ string,
	opts *tracker.ChangelogOptions,
) ([]*tracker.Changelog, *tracker.Response, error) {
	m.opts = append(m.opts, opts)
	if m.call >= len(m.pages) {
		return nil, nil, nil
	}
	page := m.pages[m.call]
	m.call++
	return page, nil, nil
}

func TestFetchAllChangelog_NullLastEntryNoPanic(t *testing.T) {
	// A null element as the LAST entry of a full page must not panic when
	// fetchAllChangelog extracts the next cursor. The cursor must advance to
	// the last non-nil ID so the loop makes progress and terminates.
	id1 := tracker.FlexString("cl-001")
	id3 := tracker.FlexString("cl-003")
	mock := &pagingChangelogGetter{
		pages: [][]*tracker.Changelog{
			{{ID: &id1}, nil}, // full page (len==limit), last element nil
			{{ID: &id3}},      // short page → terminates the loop
		},
	}

	all, err := fetchAllChangelog(context.Background(), mock, "PROJ-1", 2, "", "")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(all) != 3 {
		t.Fatalf("expected 3 accumulated entries (incl. the nil), got %d", len(all))
	}
	if len(mock.opts) != 2 {
		t.Fatalf("expected 2 page requests, got %d", len(mock.opts))
	}
	if mock.opts[1].ID != "cl-001" {
		t.Errorf("expected 2nd page cursor %q (last non-nil of page 1), got %q", "cl-001", mock.opts[1].ID)
	}
}

func TestChangelogFieldFilterPreservesCase(t *testing.T) {
	// Tracker field IDs are case-sensitive and commonly camelCase
	// (storyPoints, checklistItems, dueDate). The filter must be passed
	// through verbatim — lowercasing it (the old behavior) made the API
	// match nothing and silently returned zero changes.
	mock := &mockChangelogGetter{
		entries: sampleChangelog(),
		resp:    &tracker.Response{},
	}

	_, err := setupChangelogCmd(t, mock, output.Options{}, []string{"PROJ-123", "--field", "storyPoints"})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if mock.lastOpts == nil || mock.lastOpts.Field != "storyPoints" {
		t.Errorf("expected API field filter 'storyPoints' (verbatim), got opts: %+v", mock.lastOpts)
	}
}

func TestChangelogTypeFilter(t *testing.T) {
	mock := &mockChangelogGetter{
		entries: sampleChangelog(),
		resp:    &tracker.Response{},
	}

	_, err := setupChangelogCmd(t, mock, output.Options{}, []string{"PROJ-123", "--type", "IssueWorkflow"})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	// Verify --type is passed to API as-is (PascalCase, no lowercasing).
	if mock.lastOpts == nil || mock.lastOpts.Type != "IssueWorkflow" {
		t.Errorf("expected API type filter 'IssueWorkflow', got opts: %+v", mock.lastOpts)
	}
}

func TestChangelogEmpty(t *testing.T) {
	mock := &mockChangelogGetter{
		entries: []*tracker.Changelog{},
		resp:    &tracker.Response{},
	}

	out, err := setupChangelogCmd(t, mock, output.Options{}, []string{"PROJ-123"})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if !strings.Contains(out, "No changes found") {
		t.Errorf("expected 'No changes found'; got:\n%s", out)
	}
}

func TestChangelogNoArgs(t *testing.T) {
	mock := &mockChangelogGetter{
		entries: sampleChangelog(),
		resp:    &tracker.Response{},
	}

	_, err := setupChangelogCmd(t, mock, output.Options{}, []string{})
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
	id1 := tracker.FlexString("page1-last")

	mock := &paginatingChangelogMock{
		pages: map[string][]*tracker.Changelog{
			"my-cursor": {
				{
					ID:        &id1,
					UpdatedAt: &tracker.Timestamp{Time: time.Date(2024, 3, 15, 10, 0, 0, 0, time.UTC)},
					UpdatedBy: &tracker.User{Display: new("alice")},
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
					UpdatedBy: &tracker.User{Display: new("alice")},
					Fields:    []*tracker.ChangelogEvent{{Field: fieldRef("status"), From: "open", To: "inProgress"}},
				},
			},
			"cursor-1": {
				{
					ID:        &id2,
					UpdatedAt: &tracker.Timestamp{Time: time.Date(2024, 3, 16, 14, 0, 0, 0, time.UTC)},
					UpdatedBy: &tracker.User{Display: new("bob")},
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
