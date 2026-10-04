package issue

import (
	"bytes"
	"encoding/json"
	"reflect"
	"slices"
	"testing"
	"time"

	"github.com/slavkluev/go-yandex-tracker/tracker"
	"github.com/spf13/cobra"

	"github.com/slavkluev/ytr/internal/cmd/runner"
	"github.com/slavkluev/ytr/internal/output"
)

func fieldRef(id string) *tracker.FieldRef {
	return &tracker.FieldRef{ID: new(tracker.FlexString(id))}
}

// sampleChangelog holds a status and a summary change, then a status set for
// the first time, which has no from.
func sampleChangelog() []*tracker.Changelog {
	return []*tracker.Changelog{
		{
			ID:        new(tracker.FlexString("cl-001")),
			UpdatedAt: &tracker.Timestamp{Time: time.Date(2024, 3, 15, 10, 0, 0, 0, time.UTC)},
			UpdatedBy: &tracker.User{Display: new("alice")},
			Fields: []*tracker.ChangelogEvent{
				{
					Field: fieldRef("status"),
					From:  map[string]any{"display": "Open", "key": "open"},
					To:    map[string]any{"display": "In Progress", "key": "inprogress"},
				},
				{Field: fieldRef("summary"), From: "Old title", To: "New title"},
			},
		},
		{
			ID:        new(tracker.FlexString("cl-002")),
			UpdatedAt: &tracker.Timestamp{Time: time.Date(2024, 3, 16, 14, 30, 0, 0, time.UTC)},
			UpdatedBy: &tracker.User{Display: new("bob")},
			Fields: []*tracker.ChangelogEvent{
				{Field: fieldRef("status"), To: map[string]any{"display": "Done", "key": "done"}},
			},
		},
	}
}

// sampleChangelogAllTypes holds one entry for each kind of change other than a
// plain field change.
func sampleChangelogAllTypes() []*tracker.Changelog {
	ts := &tracker.Timestamp{Time: time.Date(2024, 3, 15, 10, 0, 0, 0, time.UTC)}
	alice := &tracker.User{Display: new("alice")}
	comment := &tracker.CommentRef{ID: new(tracker.FlexString("7"))}
	relates := &tracker.IssueLinkType{
		ID: new(tracker.FlexString("relates")), Inward: new("Related"), Outward: new("Related"),
	}
	depends := &tracker.IssueLinkType{
		ID: new(tracker.FlexString("depends")), Inward: new("Blocker"), Outward: new("Depends on"),
	}
	linked := &tracker.Issue{Key: new("SIG-1"), Display: new("Linked issue")}
	dependency := &tracker.Issue{Key: new("SIG-7"), Display: new("Dep issue")}
	attachment := &tracker.AttachmentRef{ID: new(tracker.FlexString("4")), Display: new("test.txt")}
	hour := &tracker.Duration{Duration: time.Hour}
	halfHour := &tracker.Duration{Duration: 30 * time.Minute}
	start := &tracker.Timestamp{Time: time.Date(2024, 3, 15, 12, 0, 0, 0, time.UTC)}

	return []*tracker.Changelog{
		{
			UpdatedAt: ts, UpdatedBy: alice, Type: new("IssueCommentAdded"), Transport: new("front"),
			Comments: &tracker.ChangelogComments{
				Added: []*tracker.CommentRef{{ID: comment.ID, Display: new("Test comment")}},
			},
		},
		{
			UpdatedAt: ts, UpdatedBy: alice, Type: new("IssueCommentRemoved"),
			Comments: &tracker.ChangelogComments{
				Removed: []*tracker.CommentRef{{ID: comment.ID, Display: new("Old comment")}},
			},
		},
		{
			UpdatedAt: ts, UpdatedBy: alice, Type: new("IssueCommentUpdated"),
			Comments: &tracker.ChangelogComments{
				Updated: []*tracker.CommentUpdate{{Comment: comment, From: "old text", To: "new text"}},
			},
		},
		{
			UpdatedAt: ts, UpdatedBy: alice, Type: new("IssueCommentReactionAdded"),
			Comments: &tracker.ChangelogComments{
				Updated: []*tracker.CommentUpdate{{Comment: comment, AddedReaction: new("like")}},
			},
		},
		{
			UpdatedAt: ts, UpdatedBy: alice, Type: new("IssueCommentReactionRemoved"),
			Comments: &tracker.ChangelogComments{
				Updated: []*tracker.CommentUpdate{{Comment: comment, RemovedReaction: new("heart")}},
			},
		},
		{
			UpdatedAt: ts, UpdatedBy: alice, Type: new("IssueLinked"),
			Links: []*tracker.ChangelogLink{{
				To: &tracker.ChangelogLinkValue{Direction: new("outward"), Object: linked, Type: relates},
			}},
		},
		{
			UpdatedAt: ts, UpdatedBy: alice, Type: new("IssueUnlinked"),
			Links: []*tracker.ChangelogLink{{
				From: &tracker.ChangelogLinkValue{Direction: new("inward"), Object: dependency, Type: depends},
			}},
		},
		{
			UpdatedAt: ts, UpdatedBy: alice, Type: new("IssueAttachmentAdded"),
			Attachments: &tracker.ChangelogAttachments{Added: []*tracker.AttachmentRef{attachment}},
		},
		{
			UpdatedAt: ts, UpdatedBy: alice, Type: new("IssueAttachmentRemoved"),
			Attachments: &tracker.ChangelogAttachments{Removed: []*tracker.AttachmentRef{attachment}},
		},
		{
			UpdatedAt: ts, UpdatedBy: alice, Type: new("IssueUpdated"),
			Fields: []*tracker.ChangelogEvent{{Field: fieldRef("spent"), To: "PT1H"}},
			Worklog: []*tracker.ChangelogWorklog{{
				Record: &tracker.WorklogRef{ID: new(tracker.FlexString("1")), Display: new("Work done")},
				To:     &tracker.ChangelogWorklogValue{Duration: hour, Start: start},
			}},
		},
		{
			UpdatedAt: ts, UpdatedBy: alice, Type: new("IssueUpdated"),
			Worklog: []*tracker.ChangelogWorklog{{
				Record: &tracker.WorklogRef{ID: new(tracker.FlexString("1")), Display: new("Updated")},
				From:   &tracker.ChangelogWorklogValue{Duration: halfHour, Start: start},
				To:     &tracker.ChangelogWorklogValue{Duration: hour, Start: start},
			}},
		},
		{
			UpdatedAt: ts, UpdatedBy: alice, Type: new("RelatedIssueResolutionChanged"),
			RelatedResolutions: []*tracker.RelatedResolution{{
				Direction:     new("outward"),
				Issue:         dependency,
				LinkType:      depends,
				NewResolution: &tracker.Resolution{Key: new("fixed"), Display: new("Resolved")},
			}},
		},
	}
}

// changelogDocument is what --json prints for entries, cut to fields, with
// the pagination page.
func changelogDocument(
	t *testing.T, entries []*tracker.Changelog, fields []string, page output.PaginationMeta,
) []byte {
	t.Helper()
	var doc bytes.Buffer
	cmd := &cobra.Command{}
	cmd.SetOut(&doc)
	opts := &output.Options{JSONFields: fields}
	if err := runner.PrintPage(cmd, opts, normalizeChangelog(entries), page); err != nil {
		t.Fatalf("print document: %v", err)
	}
	return doc.Bytes()
}

func assertDocument(t *testing.T, got []byte, want string) {
	t.Helper()
	var gotValue, wantValue any
	if err := json.Unmarshal(got, &gotValue); err != nil {
		t.Fatalf("document is not JSON: %v\n%s", err, got)
	}
	if err := json.Unmarshal([]byte(want), &wantValue); err != nil {
		t.Fatalf("expected document is not JSON: %v\n%s", err, want)
	}
	if !reflect.DeepEqual(gotValue, wantValue) {
		t.Errorf("document mismatch\ngot:\n%s\nwant:\n%s", got, want)
	}
}

func TestChangelogDocument(t *testing.T) {
	page := output.PaginationMeta{Cursor: "cl-002", HasMore: true}

	assertDocument(t, changelogDocument(t, sampleChangelog(), IssueChangelogFields, page), `{
	  "items": [
	    {"date": "2024-03-15T10:00:00Z", "author": "alice", "authorId": "", "type": "",
	     "fields": [
	       {"field": "status", "from": {"display": "Open", "key": "open"},
	        "to": {"display": "In Progress", "key": "inprogress"}},
	       {"field": "summary", "from": "Old title", "to": "New title"}
	     ]},
	    {"date": "2024-03-16T14:30:00Z", "author": "bob", "authorId": "", "type": "",
	     "fields": [{"field": "status", "to": {"display": "Done", "key": "done"}}]}
	  ],
	  "pagination": {"cursor": "cl-002", "hasMore": true}
	}`)
}

func TestChangelogDocumentAllTypes(t *testing.T) {
	const entry = `"date": "2024-03-15T10:00:00Z", "author": "alice", "authorId": ""`

	assertDocument(t, changelogDocument(t, sampleChangelogAllTypes(), IssueChangelogFields, output.PaginationMeta{}), `{
	  "items": [
	    {`+entry+`, "type": "IssueCommentAdded", "transport": "front",
	     "comments": [{"action": "added", "id": "7", "to": "Test comment"}]},
	    {`+entry+`, "type": "IssueCommentRemoved",
	     "comments": [{"action": "removed", "id": "7", "from": "Old comment"}]},
	    {`+entry+`, "type": "IssueCommentUpdated",
	     "comments": [{"action": "updated", "id": "7", "from": "old text", "to": "new text"}]},
	    {`+entry+`, "type": "IssueCommentReactionAdded",
	     "comments": [{"action": "reactionAdded", "id": "7", "reaction": "like"}]},
	    {`+entry+`, "type": "IssueCommentReactionRemoved",
	     "comments": [{"action": "reactionRemoved", "id": "7", "reaction": "heart"}]},
	    {`+entry+`, "type": "IssueLinked",
	     "links": [{"to": {"direction": "outward", "issue": "SIG-1", "issueDisplay": "Linked issue",
	                       "linkType": "relates", "linkTypeName": "Related"}}]},
	    {`+entry+`, "type": "IssueUnlinked",
	     "links": [{"from": {"direction": "inward", "issue": "SIG-7", "issueDisplay": "Dep issue",
	                         "linkType": "depends", "linkTypeName": "Blocker"}}]},
	    {`+entry+`, "type": "IssueAttachmentAdded",
	     "attachments": [{"action": "added", "id": "4", "name": "test.txt"}]},
	    {`+entry+`, "type": "IssueAttachmentRemoved",
	     "attachments": [{"action": "removed", "id": "4", "name": "test.txt"}]},
	    {`+entry+`, "type": "IssueUpdated",
	     "fields": [{"field": "spent", "to": "PT1H"}],
	     "worklog": [{"record": "1", "recordDisplay": "Work done",
	                  "to": {"duration": "PT1H", "start": "2024-03-15T12:00:00Z"}}]},
	    {`+entry+`, "type": "IssueUpdated",
	     "worklog": [{"record": "1", "recordDisplay": "Updated",
	                  "from": {"duration": "PT30M", "start": "2024-03-15T12:00:00Z"},
	                  "to": {"duration": "PT1H", "start": "2024-03-15T12:00:00Z"}}]},
	    {`+entry+`, "type": "RelatedIssueResolutionChanged",
	     "relatedResolutions": [{"direction": "outward", "issue": "SIG-7", "issueDisplay": "Dep issue",
	                             "linkType": "depends", "linkTypeName": "Depends on",
	                             "resolution": "fixed", "resolutionDisplay": "Resolved"}]}
	  ],
	  "pagination": {"hasMore": false}
	}`)
}

func TestChangelogDocumentEmpty(t *testing.T) {
	assertDocument(t, changelogDocument(t, nil, IssueChangelogFields, output.PaginationMeta{}), `{
	  "items": [],
	  "pagination": {"hasMore": false}
	}`)
}

func TestChangelogNamesakesKeepDistinctAuthorIDs(t *testing.T) {
	entry := func(userID string) *tracker.Changelog {
		return &tracker.Changelog{
			UpdatedBy: &tracker.User{Display: new("Иван Петров"), ID: new(tracker.FlexString(userID))},
		}
	}
	entries := []*tracker.Changelog{entry("uid-a"), entry("uid-b")}

	assertDocument(t, changelogDocument(t, entries, []string{"author", "authorId"}, output.PaginationMeta{}), `{
	  "items": [
	    {"author": "Иван Петров", "authorId": "uid-a"},
	    {"author": "Иван Петров", "authorId": "uid-b"}
	  ],
	  "pagination": {"hasMore": false}
	}`)
}

func TestChangelogFieldSelectionOmitsEmptySections(t *testing.T) {
	ts := &tracker.Timestamp{Time: time.Date(2026, 3, 15, 10, 30, 0, 0, time.UTC)}
	entries := []*tracker.Changelog{
		{
			UpdatedAt: ts,
			Links: []*tracker.ChangelogLink{{
				To: &tracker.ChangelogLinkValue{
					Direction: new("outward"),
					Object:    &tracker.Issue{Key: new("SIG-1")},
					Type:      &tracker.IssueLinkType{ID: new(tracker.FlexString("relates"))},
				},
			}},
		},
		{
			UpdatedAt: ts,
			Fields:    []*tracker.ChangelogEvent{{Field: fieldRef("summary"), From: "Old", To: "New"}},
		},
	}

	assertDocument(t, changelogDocument(t, entries, []string{"date", "links"}, output.PaginationMeta{}), `{
	  "items": [
	    {"date": "2026-03-15T10:30:00Z",
	     "links": [{"to": {"direction": "outward", "issue": "SIG-1", "linkType": "relates"}}]},
	    {"date": "2026-03-15T10:30:00Z"}
	  ],
	  "pagination": {"hasMore": false}
	}`)
}

// A null element anywhere in a changelog array decodes to a nil pointer, which
// must be skipped rather than dereferenced.
func TestChangelogNilElementsDoNotPanic(t *testing.T) {
	entries := []*tracker.Changelog{
		nil,
		{
			Type:   new("IssueWorkflow"),
			Fields: []*tracker.ChangelogEvent{nil, {Field: fieldRef("status"), From: "open", To: "closed"}},
			Comments: &tracker.ChangelogComments{
				Added:   []*tracker.CommentRef{nil},
				Removed: []*tracker.CommentRef{nil},
				Updated: []*tracker.CommentUpdate{nil},
			},
			Links:              []*tracker.ChangelogLink{nil},
			Attachments:        &tracker.ChangelogAttachments{Added: []*tracker.AttachmentRef{nil}},
			Worklog:            []*tracker.ChangelogWorklog{nil},
			RelatedResolutions: []*tracker.RelatedResolution{nil},
		},
	}

	assertDocument(t, changelogDocument(t, entries, IssueChangelogFields, output.PaginationMeta{}), `{
	  "items": [
	    {"date": "", "author": "", "authorId": "", "type": "IssueWorkflow",
	     "fields": [{"field": "status", "from": "open", "to": "closed"}]}
	  ],
	  "pagination": {"hasMore": false}
	}`)

	want := []changelogItem{{Field: "status", From: "open", To: "closed"}}
	if got := flattenChangelog(entries); !slices.Equal(got, want) {
		t.Errorf("flattenChangelog() = %+v, want %+v", got, want)
	}
}

func TestFlattenChangelog(t *testing.T) {
	const (
		first  = "2024-03-15T10:00:00Z"
		second = "2024-03-16T14:30:00Z"
	)

	tests := []struct {
		name    string
		entries []*tracker.Changelog
		want    []changelogItem
	}{
		{
			name:    "field changes",
			entries: sampleChangelog(),
			want: []changelogItem{
				{Date: first, Author: "alice", Field: "status", From: "Open", To: "In Progress"},
				{Date: first, Author: "alice", Field: "summary", From: "Old title", To: "New title"},
				{Date: second, Author: "bob", Field: "status", To: "Done"},
			},
		},
		{
			name:    "every other kind of change",
			entries: sampleChangelogAllTypes(),
			want: []changelogItem{
				{Date: first, Author: "alice", Field: "comment", To: "Test comment"},
				{Date: first, Author: "alice", Field: "comment", From: "Old comment"},
				{Date: first, Author: "alice", Field: "comment", From: "old text", To: "new text"},
				{Date: first, Author: "alice", Field: "reaction", To: "like"},
				{Date: first, Author: "alice", Field: "reaction", From: "heart"},
				{Date: first, Author: "alice", Field: "link", To: "Related → SIG-1"},
				{Date: first, Author: "alice", Field: "link", From: "Blocker → SIG-7"},
				{Date: first, Author: "alice", Field: "attachment", To: "test.txt"},
				{Date: first, Author: "alice", Field: "attachment", From: "test.txt"},
				{Date: first, Author: "alice", Field: "spent", To: "PT1H"},
				{Date: first, Author: "alice", Field: "worklog", To: "PT1H"},
				{Date: first, Author: "alice", Field: "worklog", From: "PT30M", To: "PT1H"},
				{Date: first, Author: "alice", Field: "relatedResolution", To: "SIG-7: Resolved"},
			},
		},
		{name: "no entries", entries: nil, want: nil},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := flattenChangelog(tt.entries); !slices.Equal(got, tt.want) {
				t.Errorf("flattenChangelog() =\n%+v\nwant\n%+v", got, tt.want)
			}
		})
	}
}

func TestChangelogPagination(t *testing.T) {
	first := &tracker.Changelog{ID: new(tracker.FlexString("cl-001"))}
	second := &tracker.Changelog{ID: new(tracker.FlexString("cl-002"))}

	tests := []struct {
		name    string
		entries []*tracker.Changelog
		limit   int
		want    output.PaginationMeta
	}{
		{"short page", []*tracker.Changelog{first}, 2, output.PaginationMeta{}},
		{"empty page", nil, 50, output.PaginationMeta{}},
		{"full page", []*tracker.Changelog{first, second}, 2, output.PaginationMeta{Cursor: "cl-002", HasMore: true}},
		{
			"full page ending in a null entry",
			[]*tracker.Changelog{first, nil}, 2,
			output.PaginationMeta{Cursor: "cl-001", HasMore: true},
		},
		{"full page of null entries", []*tracker.Changelog{nil, nil}, 2, output.PaginationMeta{HasMore: true}},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := changelogPagination(tt.entries, tt.limit); got != tt.want {
				t.Errorf("changelogPagination() = %+v, want %+v", got, tt.want)
			}
		})
	}
}

func TestLastChangelogCursorID(t *testing.T) {
	id1 := new(tracker.FlexString("cl-001"))
	id2 := new(tracker.FlexString("cl-002"))

	tests := []struct {
		name    string
		entries []*tracker.Changelog
		want    string
	}{
		{"nil slice", nil, ""},
		{"empty slice", []*tracker.Changelog{}, ""},
		{"normal last", []*tracker.Changelog{{ID: id1}, {ID: id2}}, "cl-002"},
		{"nil last element skipped", []*tracker.Changelog{{ID: id1}, nil}, "cl-001"},
		{"all nil", []*tracker.Changelog{nil, nil}, ""},
		{"nil last id pointer", []*tracker.Changelog{{ID: id1}, {ID: nil}}, ""},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := lastChangelogCursorID(tt.entries); got != tt.want {
				t.Errorf("lastChangelogCursorID(%+v) = %q, want %q", tt.entries, got, tt.want)
			}
		})
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
			if got := stripSelfURLs(tt.input); !reflect.DeepEqual(got, tt.want) {
				t.Errorf("stripSelfURLs() = %v, want %v", got, tt.want)
			}
		})
	}
}

func TestNormalizeLinkValue(t *testing.T) {
	relates := &tracker.IssueLinkType{
		ID: new(tracker.FlexString("relates")), Inward: new("Related"), Outward: new("Related"),
	}
	depends := &tracker.IssueLinkType{
		ID: new(tracker.FlexString("depends")), Inward: new("Blocker"), Outward: new("Depends on"),
	}
	issue := &tracker.Issue{Key: new("SIG-1"), Display: new("Test issue")}

	tests := []struct {
		name  string
		value *tracker.ChangelogLinkValue
		want  *linkValue
	}{
		{"nil", nil, nil},
		{
			"outward relates",
			&tracker.ChangelogLinkValue{Direction: new("outward"), Object: issue, Type: relates},
			&linkValue{
				Direction: "outward", Issue: "SIG-1", IssueDisplay: "Test issue",
				LinkType: "relates", LinkTypeName: "Related",
			},
		},
		{
			"inward depends takes the inward name",
			&tracker.ChangelogLinkValue{Direction: new("inward"), Object: issue, Type: depends},
			&linkValue{
				Direction: "inward", Issue: "SIG-1", IssueDisplay: "Test issue",
				LinkType: "depends", LinkTypeName: "Blocker",
			},
		},
		{
			"outward depends takes the outward name",
			&tracker.ChangelogLinkValue{Direction: new("outward"), Object: issue, Type: depends},
			&linkValue{
				Direction: "outward", Issue: "SIG-1", IssueDisplay: "Test issue",
				LinkType: "depends", LinkTypeName: "Depends on",
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := normalizeLinkValue(tt.value); !reflect.DeepEqual(got, tt.want) {
				t.Errorf("normalizeLinkValue() = %+v, want %+v", got, tt.want)
			}
		})
	}
}

func TestFormatDurationISO(t *testing.T) {
	tests := []struct {
		name     string
		duration *tracker.Duration
		want     string
	}{
		{"nil", nil, ""},
		{"an hour", &tracker.Duration{Duration: time.Hour}, "PT1H"},
		{"half an hour", &tracker.Duration{Duration: 30 * time.Minute}, "PT30M"},
		{"a day", &tracker.Duration{Duration: 24 * time.Hour}, "P1D"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := formatDurationISO(tt.duration); got != tt.want {
				t.Errorf("formatDurationISO() = %q, want %q", got, tt.want)
			}
		})
	}
}

func TestFormatLinkValueString(t *testing.T) {
	relates := &tracker.IssueLinkType{ID: new(tracker.FlexString("relates")), Outward: new("Related")}

	tests := []struct {
		name  string
		value *tracker.ChangelogLinkValue
		want  string
	}{
		{"nil", nil, ""},
		{
			"link name and issue key",
			&tracker.ChangelogLinkValue{
				Direction: new("outward"), Object: &tracker.Issue{Key: new("SIG-1")}, Type: relates,
			},
			"Related → SIG-1",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := formatLinkValueString(tt.value); got != tt.want {
				t.Errorf("formatLinkValueString() = %q, want %q", got, tt.want)
			}
		})
	}
}

func TestNormalizeChangeValue(t *testing.T) {
	tests := []struct {
		name  string
		input any
		want  string
	}{
		{"nil returns empty string", nil, ""},
		{"string returns as-is", "hello", "hello"},
		{"float64 formats as number", float64(42.5), "42.5"},
		{"float64 integer formats without decimal", float64(100), "100"},
		{"map with display key returns display", map[string]any{"display": "Open", "key": "open"}, "Open"},
		{"map with only key returns key", map[string]any{"key": "open"}, "open"},
		{"map with only id returns id", map[string]any{"id": "42"}, "42"},
		{
			"array of objects joins display values",
			[]any{map[string]any{"display": "Alice"}, map[string]any{"display": "Bob"}},
			"Alice, Bob",
		},
		{"array of strings joins with comma", []any{"foo", "bar"}, "foo, bar"},
		{"empty array returns empty string", []any{}, ""},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := normalizeChangeValue(tt.input); got != tt.want {
				t.Errorf("normalizeChangeValue(%v) = %q, want %q", tt.input, got, tt.want)
			}
		})
	}
}
