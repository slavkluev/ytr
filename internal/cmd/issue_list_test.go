package cmd

import (
	"net/http"
	"net/url"
	"slices"
	"strconv"
	"strings"
	"testing"

	ytrerrors "github.com/slavkluev/ytr/internal/errors"
	"github.com/slavkluev/ytr/internal/faketracker"
)

// trackerPage answers a paged list request, page of perPage, with items, a
// JSON array, and the X-Total-Count Tracker sends with it.
func trackerPage(method, path string, page, perPage, total int, items string) faketracker.Exchange {
	ex := withQuery(trackerWrite(method, path, http.StatusOK, items), pageQuery(page, perPage))
	ex.Header.Set("X-Total-Count", strconv.Itoa(total))

	return ex
}

func pageQuery(page, perPage int) url.Values {
	return url.Values{"page": {strconv.Itoa(page)}, "perPage": {strconv.Itoa(perPage)}}
}

func issueSearch(page, perPage, total int, issues string) faketracker.Exchange {
	return countedPage(http.MethodPost, "/v3/issues/_search", page, perPage, total, issues)
}

// listedIssue is an issue as the search answers it, with an open status, an
// assignee and only the fields ytr reads.
func listedIssue(key string) string {
	return `{"key": "` + key + `", "summary": "Summary for ` + key + `",
		"status": {"key": "open", "display": "Open"}, "assignee": {"id": "uid-` + key + `", "display": "user` + key + `"}}`
}

func TestIssueList(t *testing.T) {
	t.Parallel()

	const fullIssue = `{"key": "PROJ-1", "summary": "Fix login bug", "status": {"key": "inProgress", "display": "In Progress"},
		"priority": {"display": "Critical"}, "type": {"display": "Bug"},
		"assignee": {"id": "uid-a", "display": "Иван Петров"},
		"createdAt": "2026-09-17T09:05:00.000+0300", "updatedAt": "2026-09-18T10:00:00.000+0000"}`
	const fullItem = `{"key": "PROJ-1", "summary": "Fix login bug", "status": "In Progress", "priority": "Critical",
		"type": "Bug", "assignee": "Иван Петров", "assigneeId": "uid-a", "createdAt": "2026-09-17T09:05:00+03:00",
		"updatedAt": "2026-09-18T10:00:00Z"}`
	two := issueSearch(1, 50, 2, "["+listedIssue("PROJ-1")+","+listedIssue("PROJ-2")+"]")
	longSummary := strings.Repeat("a long summary ", 20)
	long := issueSearch(1, 50, 1, `[{"key": "PROJ-1", "summary": "`+longSummary+`", "status": {"key": "open"}}]`)
	empty := issueSearch(1, 50, 0, `[]`)
	list := func(extra ...string) []string { return slices.Concat([]string{"issue", "list"}, extra) }

	runLeafRows(t, []leafRow{
		{
			name: "Filtered", args: list("--filter", "queue=PROJ"), exchanges: []faketracker.Exchange{two},
			body: `{"filter": {"queue": "PROJ"}}`,
			json: `{"items": [
				{"key": "PROJ-1", "summary": "Summary for PROJ-1", "status": "Open", "assignee": "userPROJ-1",
					"assigneeId": "uid-PROJ-1"},
				{"key": "PROJ-2", "summary": "Summary for PROJ-2", "status": "Open", "assignee": "userPROJ-2",
					"assigneeId": "uid-PROJ-2"}],
				"pagination": {"hasMore": false, "total": 2}}`,
		},
		{
			name: "Every field", args: list(),
			exchanges: []faketracker.Exchange{issueSearch(1, 50, 1, "["+fullIssue+"]")},
			json:      `{"items": [` + fullItem + `], "pagination": {"hasMore": false, "total": 1}}`,
		},
		{
			name: "A bare issue", args: list(),
			exchanges: []faketracker.Exchange{
				issueSearch(1, 50, 2, `[{"key": "NIL-1", "summary": "Bare"}, {"key": "NIL-2", "status": {}}]`),
			},
			json: `{"items": [{"key": "NIL-1", "summary": "Bare", "status": "", "assigneeId": ""},
				{"key": "NIL-2", "summary": "", "status": "", "assigneeId": ""}],
				"pagination": {"hasMore": false, "total": 2}}`,
		},
		{
			name: "Namesakes keep their assignee IDs", args: list("--jq", "[.items[].assigneeId]"),
			exchanges: []faketracker.Exchange{issueSearch(1, 50, 2, `[
				{"key": "PROJ-1", "assignee": {"id": "uid-a", "display": "Иван Петров"}},
				{"key": "PROJ-2", "assignee": {"id": "uid-b", "display": "Иван Петров"}}]`)},
			json: `["uid-a", "uid-b"]`,
		},
		{
			name: "A full page has more",
			args: list("--limit", "2", "--json", "key"),
			exchanges: []faketracker.Exchange{
				issueSearch(1, 2, 5, "["+listedIssue("PROJ-1")+","+listedIssue("PROJ-2")+"]"),
			},
			json: `{"items": [{"key": "PROJ-1"}, {"key": "PROJ-2"}],
				"pagination": {"cursor": "2", "hasMore": true, "total": 5}}`,
		},
		{
			name: "jq", args: list("--jq", ".items[].key"), exchanges: []faketracker.Exchange{two},
			stdout: "PROJ-1\nPROJ-2\n",
		},
		{
			name: "Limit", args: list("--limit", "10", "--jq", ".items[].key"),
			exchanges: []faketracker.Exchange{issueSearch(1, 10, 0, `[]`)}, body: `{}`,
		},
		{
			name: "Limit at the maximum", args: list("--limit", "1000", "--jq", ".items[].key"),
			exchanges: []faketracker.Exchange{issueSearch(1, 1000, 0, `[]`)},
		},
		{
			name: "Cursor", args: list("--cursor", "3", "--json", "key"),
			exchanges: []faketracker.Exchange{issueSearch(3, 50, 0, `[]`)},
			json:      `{"items": [], "pagination": {"hasMore": false}}`,
		},
		{
			name: "Not a page cursor", args: list("--cursor", "abc"), signedOut: true, code: ytrerrors.ExitUserError,
			stderr: []string{"invalid cursor"},
		},
		{
			name: "All pages", args: list("--all", "--limit", "2", "--jq", ".items[].key"),
			exchanges: []faketracker.Exchange{
				issueSearch(1, 2, 3, "["+listedIssue("A-1")+","+listedIssue("A-2")+"]"),
				issueSearch(2, 2, 3, "["+listedIssue("A-3")+"]"),
			},
			stdout: "A-1\nA-2\nA-3\n",
			check:  assertRequestOrder(`page=1&perPage=2`, `page=2&perPage=2`),
		},
		{
			name: "All pages end on a full page", args: list("--all", "--limit", "2", "--jq", ".items[].key"),
			exchanges: []faketracker.Exchange{
				issueSearch(1, 2, 4, "["+listedIssue("A-1")+","+listedIssue("A-2")+"]"),
				issueSearch(2, 2, 4, "["+listedIssue("A-3")+","+listedIssue("A-4")+"]"),
			},
			stdout: "A-1\nA-2\nA-3\nA-4\n",
			check:  assertRequestOrder("page=1&perPage=2", "page=2&perPage=2"),
		},
		{
			name: "All pages as JSON", args: list("--all", "--limit", "2", "--json", "key"),
			exchanges: []faketracker.Exchange{
				issueSearch(1, 2, 3, "["+listedIssue("A-1")+","+listedIssue("A-2")+"]"),
				issueSearch(2, 2, 3, "["+listedIssue("A-3")+"]"),
			},
			json: `{"items": [{"key": "A-1"}, {"key": "A-2"}, {"key": "A-3"}],
				"pagination": {"hasMore": false, "total": 3}}`,
		},
		{
			name: "A later page fails", args: list("--all", "--limit", "2"),
			exchanges: []faketracker.Exchange{
				issueSearch(1, 2, 3, "["+listedIssue("A-1")+","+listedIssue("A-2")+"]"),
				withQuery(
					trackerError(http.MethodPost, "/v3/issues/_search", http.StatusInternalServerError,
						"Search unavailable"),
					pageQuery(2, 2),
				),
			},
			code: ytrerrors.ExitUserError, stderr: []string{`"message":"Search unavailable"`},
			check: assertOneErrorDocument("Search unavailable"),
		},
		{
			name: "All pages keep the filter and order",
			args: list(
				"--all",
				"--limit",
				"2",
				"--filter",
				"queue=PROJ",
				"--order-by",
				"updated",
				"--jq",
				".items[].key",
			),
			exchanges: []faketracker.Exchange{
				issueSearch(1, 2, 3, "["+listedIssue("A-1")+","+listedIssue("A-2")+"]"),
				issueSearch(2, 2, 3, "["+listedIssue("A-3")+"]"),
			},
			body:   `{"filter": {"queue": "PROJ"}, "order": "-updated"}`,
			stdout: "A-1\nA-2\nA-3\n",
			check:  assertRequestOrder("page=1&perPage=2", "page=2&perPage=2"),
		},
		{
			name: "All pages keep the query",
			args: list("--all", "--limit", "2", "--query", "Queue: PROJ AND Status: open", "--jq", ".items[].key"),
			exchanges: []faketracker.Exchange{
				issueSearch(1, 2, 3, "["+listedIssue("A-1")+","+listedIssue("A-2")+"]"),
				issueSearch(2, 2, 3, "["+listedIssue("A-3")+"]"),
			},
			body:   `{"query": "Queue: PROJ AND Status: open"}`,
			stdout: "A-1\nA-2\nA-3\n",
			check:  assertRequestOrder("page=1&perPage=2", "page=2&perPage=2"),
		},
		{
			name: "Empty", args: list("--filter", "queue=PROJ"), exchanges: []faketracker.Exchange{empty},
			json: `{"items": [], "pagination": {"hasMore": false}}`,
		},
		{
			name: "Empty as JSON", args: list("--json", "key"), exchanges: []faketracker.Exchange{empty},
			json: `{"items": [], "pagination": {"hasMore": false}}`,
		},
		{
			name: "Query", args: list("--query", "Queue: PROJ AND Status: open", "--jq", ".items[].key"),
			exchanges: []faketracker.Exchange{empty}, body: `{"query": "Queue: PROJ AND Status: open"}`,
		},
		{
			name: "Filter", args: list("--filter", "priority=critical", "--jq", ".items[].key"),
			exchanges: []faketracker.Exchange{empty}, body: `{"filter": {"priority": "critical"}}`,
		},
		{
			name: "Filter value holding =", args: list("--filter", "summary=a=b", "--jq", ".items[].key"),
			exchanges: []faketracker.Exchange{empty}, body: `{"filter": {"summary": "a=b"}}`,
		},
		{
			name:      "Repeated filter key",
			args:      list("--filter", "status=open", "--filter", "status=closed", "--jq", ".items[].key"),
			exchanges: []faketracker.Exchange{empty},
			body:      `{"filter": {"status": ["open", "closed"]}}`,
		},
		{
			name: "Order descending", args: list("--order-by", "updated", "--jq", ".items[].key"),
			exchanges: []faketracker.Exchange{empty}, body: `{"order": "-updated"}`,
		},
		{
			name: "Order ascending", args: list("--order-by", "created", "--order-asc", "--jq", ".items[].key"),
			exchanges: []faketracker.Exchange{empty}, body: `{"order": "+created"}`,
		},
		{
			name: "Not a filter", args: list("--filter", "noequalssign"), signedOut: true,
			code: ytrerrors.ExitUserError,
			stderr: []string{
				`"message":"invalid filter format \"noequalssign\": expected key=value",` +
					`"suggestion":"Use --filter key=value (e.g., --filter priority=critical)"`,
			},
		},
		{
			name: "Query with filter", args: list("--query", "Queue: PROJ", "--filter", "priority=critical"),
			code: ytrerrors.ExitUserError,
			stderr: []string{
				`"message":"cannot combine --query with --filter",` +
					`"suggestion":"Use --query for Tracker query language, or --filter for structured search, but not both"`,
			},
		},
		{
			name: "Order with query", args: list("--query", "Queue: PROJ", "--order-by", "updated"),
			code: ytrerrors.ExitUserError,
			stderr: []string{
				`"message":"--order-by cannot be used with --query",` +
					`"suggestion":"Include sorting in the query string: '\"Sort By\": fieldName ASC'"`,
			},
		},
		{
			name: "Ascending without an order", args: list("--order-asc"), code: ytrerrors.ExitUserError,
			stderr: []string{
				`"message":"--order-asc requires --order-by",` +
					`"suggestion":"Use --order-by to specify the sort field (e.g., --order-by updated --order-asc)"`,
			},
		},
		{
			name: "Conflict before the hint", args: list("--order-asc", "--json="), code: ytrerrors.ExitUserError,
			stderr: []string{`"message":"--order-asc requires --order-by"`},
		},
		{
			name: "Long summary whole", args: list("--json", "summary,status"), exchanges: []faketracker.Exchange{long},
			json: `{"items": [{"summary": "` + longSummary + `", "status": "open"}],
				"pagination": {"hasMore": false, "total": 1}}`,
		},
		{
			name: "Signed out", args: list("--filter", "queue=Q"), signedOut: true, code: ytrerrors.ExitAuthError,
			stderr: []string{`{"code":"auth_error","message":"not authenticated",`},
		},
	})
}

// assertRequestOrder wants the run's requests to carry these queries, in order.
func assertRequestOrder(queries ...string) func(*testing.T, cliResult) {
	return func(t *testing.T, res cliResult) {
		t.Helper()

		var got []string
		for _, req := range res.Requests {
			got = append(got, req.Query.Encode())
		}
		if !slices.Equal(got, queries) {
			t.Errorf("request queries = %q, want %q", got, queries)
		}
	}
}
