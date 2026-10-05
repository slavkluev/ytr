package cmd

import (
	"net/http"
	"net/url"
	"slices"
	"testing"

	ytrerrors "github.com/slavkluev/ytr/internal/errors"
	"github.com/slavkluev/ytr/internal/faketracker"
)

// changelogPage answers the changelog of PROJ-123 asked for with query.
func changelogPage(query url.Values, entries string) faketracker.Exchange {
	return withQuery(trackerGET("/v3/issues/PROJ-123/changelog", entries), query)
}

func TestIssueChangelog(t *testing.T) {
	t.Parallel()

	const changes = `[
		{"id": "cl-001", "updatedAt": "2024-03-15T10:00:00.000+0000", "updatedBy": {"id": "uid-a", "display": "alice"},
			"type": "IssueUpdated", "fields": [
				{"field": {"id": "status"}, "from": {"display": "Open", "key": "open"},
					"to": {"display": "In Progress", "key": "inprogress"}},
				{"field": {"id": "summary"}, "from": "Old title", "to": "New title"}]},
		{"id": "cl-002", "updatedAt": "2024-03-16T14:30:00.000+0000", "updatedBy": {"display": "bob"},
			"type": "IssueWorkflow", "fields": [{"field": {"id": "status"}, "to": {"display": "Done", "key": "done"}}]}]`
	const entries = `[
		{"date": "2024-03-15T10:00:00Z", "author": "alice", "authorId": "uid-a", "type": "IssueUpdated",
			"fields": [
				{"field": "status", "from": {"display": "Open", "key": "open"},
					"to": {"display": "In Progress", "key": "inprogress"}},
				{"field": "summary", "from": "Old title", "to": "New title"}]},
		{"date": "2024-03-16T14:30:00Z", "author": "bob", "authorId": "", "type": "IssueWorkflow",
			"fields": [{"field": "status", "to": {"display": "Done", "key": "done"}}]}]`
	const emptyPage = `{"items": [], "pagination": {"hasMore": false}}`
	firstPage := url.Values{"perPage": {"50"}}
	page := changelogPage(firstPage, changes)
	changelog := func(extra ...string) []string {
		return slices.Concat([]string{"issue", "changelog", "PROJ-123"}, extra)
	}
	entry := func(id, author string) string {
		return `[{"id": "` + id + `", "updatedAt": "2024-03-15T10:00:00.000+0000", "updatedBy": {"display": "` +
			author + `"}, "fields": [{"field": {"id": "status"}, "from": "open", "to": "closed"}]}]`
	}

	runLeafRows(t, []leafRow{
		{
			name: "Every field", args: changelog(), exchanges: []faketracker.Exchange{page},
			json: `{"items": ` + entries + `, "pagination": {"hasMore": false}}`,
		},
		{
			name: "JSON of a full page", args: changelog("--limit", "2", "--json", "date,author,authorId,type,fields"),
			exchanges: []faketracker.Exchange{changelogPage(url.Values{"perPage": {"2"}}, changes)},
			json:      `{"items": ` + entries + `, "pagination": {"cursor": "cl-002", "hasMore": true}}`,
		},
		{
			name: "JSON of a short page", args: changelog("--json", "author"), exchanges: []faketracker.Exchange{page},
			json: `{"items": [{"author": "alice"}, {"author": "bob"}], "pagination": {"hasMore": false}}`,
		},
		{
			name: "jq", args: changelog("--jq", ".items[].author"), exchanges: []faketracker.Exchange{page},
			stdout: "alice\nbob\n",
		},
		{
			name: "Field filter as given", args: changelog("--field", "storyPoints"),
			exchanges: []faketracker.Exchange{changelogPage(
				url.Values{"perPage": {"50"}, "field": {"storyPoints"}}, `[]`)},
			json: emptyPage,
		},
		{
			name: "Type filter as given", args: changelog("--type", "IssueWorkflow"),
			exchanges: []faketracker.Exchange{changelogPage(
				url.Values{"perPage": {"50"}, "type": {"IssueWorkflow"}}, `[]`)},
			json: emptyPage,
		},
		{
			name: "Cursor", args: changelog("--cursor", "my-cursor", "--json", "author"),
			exchanges: []faketracker.Exchange{changelogPage(
				url.Values{"perPage": {"50"}, "id": {"my-cursor"}}, entry("page1-last", "alice"))},
			json: `{"items": [{"author": "alice"}], "pagination": {"hasMore": false}}`,
		},
		{
			name: "Empty", args: changelog(), exchanges: []faketracker.Exchange{changelogPage(firstPage, `[]`)},
			json: emptyPage,
		},
		{
			name: "All pages", args: changelog("--all", "--limit", "1", "--json", "author"),
			exchanges: []faketracker.Exchange{
				changelogPage(url.Values{"perPage": {"1"}}, entry("cursor-1", "alice")),
				changelogPage(url.Values{"perPage": {"1"}, "id": {"cursor-1"}}, entry("cursor-2", "bob")),
				changelogPage(url.Values{"perPage": {"1"}, "id": {"cursor-2"}}, `[]`),
			},
			json:  `{"items": [{"author": "alice"}, {"author": "bob"}], "pagination": {"hasMore": false}}`,
			check: assertRequestOrder("perPage=1", "id=cursor-1&perPage=1", "id=cursor-2&perPage=1"),
		},
		{
			name: "All pages as JSON when they hold --limit entries",
			args: changelog("--all", "--limit", "1", "--json", "author"),
			exchanges: []faketracker.Exchange{
				changelogPage(url.Values{"perPage": {"1"}}, entry("cursor-1", "alice")),
				changelogPage(url.Values{"perPage": {"1"}, "id": {"cursor-1"}}, `[]`),
			},
			json: `{"items": [{"author": "alice"}], "pagination": {"hasMore": false}}`,
		},
		{
			name: "A short page is not the last", args: changelog("--all", "--limit", "2", "--json", "author"),
			exchanges: []faketracker.Exchange{
				changelogPage(url.Values{"perPage": {"2"}}, entry("cursor-1", "alice")),
				changelogPage(url.Values{"perPage": {"2"}, "id": {"cursor-1"}}, entry("cursor-2", "bob")),
				changelogPage(url.Values{"perPage": {"2"}, "id": {"cursor-2"}}, `[]`),
			},
			json:  `{"items": [{"author": "alice"}, {"author": "bob"}], "pagination": {"hasMore": false}}`,
			check: assertRequestOrder("perPage=2", "id=cursor-1&perPage=2", "id=cursor-2&perPage=2"),
		},
		{
			name: "All pages keep the filters",
			args: changelog(
				"--all",
				"--limit",
				"1",
				"--field",
				"status",
				"--type",
				"IssueWorkflow",
				"--json",
				"author",
			),
			exchanges: []faketracker.Exchange{
				changelogPage(url.Values{"perPage": {"1"}, "field": {"status"}, "type": {"IssueWorkflow"}},
					entry("cursor-1", "alice")),
				changelogPage(
					url.Values{"perPage": {"1"}, "id": {"cursor-1"}, "field": {"status"}, "type": {"IssueWorkflow"}},
					`[]`,
				),
			},
			json: `{"items": [{"author": "alice"}], "pagination": {"hasMore": false}}`,
			check: assertRequestOrder(
				"field=status&perPage=1&type=IssueWorkflow", "field=status&id=cursor-1&perPage=1&type=IssueWorkflow",
			),
		},
		{
			name: "A later page fails", args: changelog("--all", "--limit", "1", "--json", "author"),
			exchanges: []faketracker.Exchange{
				changelogPage(url.Values{"perPage": {"1"}}, entry("cursor-1", "alice")),
				withQuery(
					trackerError(http.MethodGet, "/v3/issues/PROJ-123/changelog", http.StatusInternalServerError,
						"Changelog unavailable"),
					url.Values{"perPage": {"1"}, "id": {"cursor-1"}},
				),
			},
			code: ytrerrors.ExitUserError, stderr: []string{`"message":"Changelog unavailable"`},
			check: assertOneErrorDocument("Changelog unavailable"),
		},
		{
			name: "A page ending in null", args: changelog("--all", "--limit", "2", "--json", "author"),
			exchanges: []faketracker.Exchange{
				changelogPage(
					url.Values{"perPage": {"2"}},
					`[{"id": "cl-001", "updatedBy": {"display": "alice"}}, null]`,
				),
			},
			code: ytrerrors.ExitUserError, stderr: []string{"cannot page past cursor"},
			check: assertOneErrorDocument(
				`API request failed: tracker: cannot page past cursor "": the last item of the page has ID ""`,
			),
		},
		{
			name: "A cursor that does not move", args: changelog("--all"),
			exchanges: []faketracker.Exchange{
				changelogPage(firstPage, entry("stuck", "alice")),
				changelogPage(url.Values{"perPage": {"50"}, "id": {"stuck"}}, entry("stuck", "bob")),
			},
			code:   ytrerrors.ExitUserError,
			stderr: []string{`cannot page past cursor "stuck": the last item of the page has ID "stuck"`},
		},
		{
			name: "Not an issue key", args: []string{"issue", "changelog", "123"},
			code: ytrerrors.ExitUserError, stderr: []string{`invalid issue key "123"`},
		},
		{
			name: "Conflict before the hint", args: changelog("--all", "--cursor", "2", "--json="), signedOut: true,
			code: ytrerrors.ExitUserError, stderr: []string{"Error: cannot combine --all with --cursor\n"},
		},
		{
			name: "Conflict before the limit", args: changelog("--all", "--cursor", "2", "--limit", "0"),
			signedOut: true, code: ytrerrors.ExitUserError, stderr: []string{"cannot combine --all with --cursor"},
		},
		{
			name: "Bad arg before the hint", args: []string{"issue", "changelog", "123", "--json="},
			code: ytrerrors.ExitUserError, stderr: []string{`invalid issue key "123"`},
		},
		failureRow(withQuery(trackerNotFound("/v3/issues/PROJ-123/changelog"), firstPage),
			changelog("--json", "author")...),
	})
}
