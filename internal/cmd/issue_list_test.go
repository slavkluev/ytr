package cmd

import (
	"net/http"
	"net/url"
	"slices"
	"strconv"
	"strings"
	"testing"

	"github.com/jedib0t/go-pretty/v6/text"

	ytrerrors "github.com/slavkluev/ytr/internal/errors"
	"github.com/slavkluev/ytr/internal/faketracker"
	"github.com/slavkluev/ytr/internal/output"
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
	return trackerPage(http.MethodPost, "/v3/issues/_search", page, perPage, total, issues)
}

// listedIssue is an issue as the search answers it, with an open status, an
// assignee and only the fields ytr reads.
func listedIssue(key string) string {
	return `{"key": "` + key + `", "summary": "Summary for ` + key + `",
		"status": {"key": "open", "display": "Open"}, "assignee": {"id": "uid-` + key + `", "display": "user` + key + `"}}`
}

func TestIssueList(t *testing.T) {
	// go-pretty decides once, at init, whether to emit ANSI codes, from
	// NO_COLOR and TERM; a developer's NO_COLOR=1 would strip the codes the
	// colors row looks for.
	if text.Bold.Sprint("x") == "x" {
		text.EnableColors()
		t.Cleanup(text.DisableColors)
	}

	const fullIssue = `{"key": "PROJ-1", "summary": "Fix login bug", "status": {"key": "inProgress", "display": "In Progress"},
		"priority": {"display": "Critical"}, "type": {"display": "Bug"},
		"assignee": {"id": "uid-a", "display": "Иван Петров"},
		"createdAt": "2026-09-17T09:05:00.000+0300", "updatedAt": "2026-09-18T10:00:00.000+0000"}`
	const fullItem = `{"key": "PROJ-1", "summary": "Fix login bug", "status": "In Progress", "priority": "Critical",
		"type": "Bug", "assignee": "Иван Петров", "assigneeId": "uid-a", "createdAt": "2026-09-17T09:05:00+03:00",
		"updatedAt": "2026-09-18T10:00:00Z"}`
	all := "key,summary,status,priority,type,assignee,assigneeId,createdAt,updatedAt"
	two := issueSearch(1, 50, 2, "["+listedIssue("PROJ-1")+","+listedIssue("PROJ-2")+"]")
	longSummary := strings.Repeat("a long summary ", 20)
	long := issueSearch(1, 50, 1, `[{"key": "PROJ-1", "summary": "`+longSummary+`", "status": {"key": "open"}}]`)
	empty := issueSearch(1, 50, 0, `[]`)
	list := func(extra ...string) []string { return slices.Concat([]string{"issue", "list"}, extra) }

	runLeafRows(t, []leafRow{
		{
			name: "Table", args: list("--filter", "queue=PROJ"), exchanges: []faketracker.Exchange{two},
			body: `{"filter": {"queue": "PROJ"}}`,
			stdout: "KEY\tSTATUS\tASSIGNEE\tSUMMARY\n" +
				"PROJ-1\tOpen\tuserPROJ-1\tSummary for PROJ-1\n" +
				"PROJ-2\tOpen\tuserPROJ-2\tSummary for PROJ-2\n",
		},
		{
			name: "JSON", args: list("--json", all),
			exchanges: []faketracker.Exchange{issueSearch(1, 50, 1, "["+fullIssue+"]")},
			json:      `{"items": [` + fullItem + `], "pagination": {"hasMore": false, "total": 1}}`,
		},
		{
			name: "JSON of a bare issue", args: list("--json", all),
			exchanges: []faketracker.Exchange{issueSearch(1, 50, 1, `[{"key": "NIL-1", "summary": "Bare"}]`)},
			json: `{"items": [{"key": "NIL-1", "summary": "Bare", "status": "-", "assigneeId": ""}],
				"pagination": {"hasMore": false, "total": 1}}`,
		},
		{
			name: "Table of a bare issue", args: list(),
			exchanges: []faketracker.Exchange{issueSearch(1, 50, 1, `[{"key": "NIL-1", "summary": "Bare"}]`)},
			stdout:    "KEY\tSTATUS\tASSIGNEE\tSUMMARY\nNIL-1\t-\t-\tBare\n",
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
			name: "Quiet", args: list("--filter", "queue=PROJ", "--quiet"), exchanges: []faketracker.Exchange{two},
			stdout: "PROJ-1\nPROJ-2\n",
		},
		{
			name: "jq", args: list("--jq", ".items[].key"), exchanges: []faketracker.Exchange{two},
			stdout: "PROJ-1\nPROJ-2\n",
		},
		{
			name: "Limit", args: list("--limit", "10", "--quiet"),
			exchanges: []faketracker.Exchange{issueSearch(1, 10, 0, `[]`)}, body: `{}`,
		},
		{
			name: "Limit over the maximum", args: list("--limit", "2000", "--quiet"),
			exchanges: []faketracker.Exchange{issueSearch(1, 1000, 0, `[]`)},
		},
		{
			name: "Limit under one", args: list("--limit", "0", "--quiet"),
			exchanges: []faketracker.Exchange{issueSearch(1, 50, 0, `[]`)},
		},
		{
			name: "Cursor", args: list("--cursor", "3", "--json", "key"),
			exchanges: []faketracker.Exchange{issueSearch(3, 50, 0, `[]`)},
			json:      `{"items": [], "pagination": {"hasMore": false}}`,
		},
		{
			name: "Not a page cursor", args: list("--cursor", "abc"), code: ytrerrors.ExitUserError,
			stderr: []string{"invalid cursor"},
		},
		{
			name: "All pages", args: list("--all", "--limit", "2", "--quiet"),
			exchanges: []faketracker.Exchange{
				issueSearch(1, 2, 3, "["+listedIssue("A-1")+","+listedIssue("A-2")+"]"),
				issueSearch(2, 2, 3, "["+listedIssue("A-3")+"]"),
			},
			stdout: "A-1\nA-2\nA-3\n",
			check:  assertRequestOrder(`page=1&perPage=2`, `page=2&perPage=2`),
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
			name: "Empty", args: list("--filter", "queue=PROJ"), exchanges: []faketracker.Exchange{empty},
			stdout: "No issues found\n",
		},
		{
			name: "Empty on a TTY", args: list("--filter", "queue=PROJ"), term: output.Options{TTY: true, Colors: true},
			exchanges: []faketracker.Exchange{empty}, stdout: "No issues found\n",
		},
		{
			name: "Empty as JSON", args: list("--json", "key"), exchanges: []faketracker.Exchange{empty},
			json: `{"items": [], "pagination": {"hasMore": false}}`,
		},
		{
			name: "Query", args: list("--query", "Queue: PROJ AND Status: open", "--quiet"),
			exchanges: []faketracker.Exchange{empty}, body: `{"query": "Queue: PROJ AND Status: open"}`,
		},
		{
			name: "Filter", args: list("--filter", "priority=critical", "--quiet"),
			exchanges: []faketracker.Exchange{empty}, body: `{"filter": {"priority": "critical"}}`,
		},
		{
			name: "Filter value holding =", args: list("--filter", "summary=a=b", "--quiet"),
			exchanges: []faketracker.Exchange{empty}, body: `{"filter": {"summary": "a=b"}}`,
		},
		{
			name: "Repeated filter key", args: list("--filter", "status=open", "--filter", "status=closed", "--quiet"),
			exchanges: []faketracker.Exchange{empty}, body: `{"filter": {"status": ["open", "closed"]}}`,
		},
		{
			name: "Order descending", args: list("--order-by", "updated", "--quiet"),
			exchanges: []faketracker.Exchange{empty}, body: `{"order": "-updated"}`,
		},
		{
			name: "Order ascending", args: list("--order-by", "created", "--order-asc", "--quiet"),
			exchanges: []faketracker.Exchange{empty}, body: `{"order": "+created"}`,
		},
		{
			name: "Not a filter", args: list("--filter", "noequalssign"), code: ytrerrors.ExitUserError,
			stderr: []string{
				"Error: invalid filter format \"noequalssign\": expected key=value\n" +
					"Use --filter key=value (e.g., --filter priority=critical)\n",
			},
		},
		{
			name: "Query with filter", args: list("--query", "Queue: PROJ", "--filter", "priority=critical"),
			code: ytrerrors.ExitUserError,
			stderr: []string{
				"Error: cannot combine --query with --filter\n" +
					"Use --query for Tracker query language, or --filter for structured search, but not both\n",
			},
		},
		{
			name: "Order with query", args: list("--query", "Queue: PROJ", "--order-by", "updated"),
			code: ytrerrors.ExitUserError,
			stderr: []string{
				"Error: --order-by cannot be used with --query\n" +
					"Include sorting in the query string: '\"Sort By\": fieldName ASC'\n",
			},
		},
		{
			name: "Ascending without an order", args: list("--order-asc"), code: ytrerrors.ExitUserError,
			stderr: []string{
				"Error: --order-asc requires --order-by\n" +
					"Use --order-by to specify the sort field (e.g., --order-by updated --order-asc)\n",
			},
		},
		{
			name: "Conflict before the hint", args: list("--order-asc", "--json="), code: ytrerrors.ExitUserError,
			stderr: []string{"Error: --order-asc requires --order-by\n"},
		},
		{
			name:      "TTY",
			args:      list("--filter", "queue=PROJ"),
			term:      output.Options{TTY: true},
			exchanges: []faketracker.Exchange{two},
			holds: []string{
				"KEY     STATUS  ASSIGNEE    SUMMARY",
				"PROJ-1  Open    userPROJ-1  Summary for PROJ-1",
			},
			check: assertAlignedTable,
		},
		{
			name: "TTY with colors",
			args: list("--filter", "queue=PROJ"),
			term: output.Options{TTY: true, Colors: true},
			exchanges: []faketracker.Exchange{issueSearch(1, 50, 3, `[
				{"key": "PROJ-1", "status": {"key": "closed", "display": "Closed"}},
				{"key": "PROJ-2", "status": {"key": "inProgress", "display": "In Progress"}},
				{"key": "PROJ-3", "status": {"key": "blocked", "display": "Blocked"}}]`)},
			check: assertColored(
				text.FgGreen.Sprint("Closed"), text.FgYellow.Sprint("In Progress"), text.FgRed.Sprint("Blocked")),
			holds: []string{"PROJ-1", "Closed"},
		},
		{
			name: "TTY without colors", args: list("--filter", "queue=PROJ"), term: output.Options{TTY: true},
			exchanges: []faketracker.Exchange{issueSearch(1, 50, 1,
				`[{"key": "PROJ-1", "status": {"key": "closed", "display": "Closed"}}]`)},
			holds: []string{"PROJ-1  Closed"},
			check: assertNoANSI,
		},
		{
			name: "Long summary off a TTY", args: list(), exchanges: []faketracker.Exchange{long},
			stdout: "KEY\tSTATUS\tASSIGNEE\tSUMMARY\nPROJ-1\topen\t-\t" + longSummary + "\n",
		},
		{
			name: "Long summary on a TTY", args: list(), term: output.Options{TTY: true},
			exchanges: []faketracker.Exchange{long},
			holds:     []string{"PROJ-1  open    -         a long summary a long summ..."},
		},
		{
			name: "Signed out", args: list(), signedOut: true, code: ytrerrors.ExitAuthError,
			stderr: []string{"Error: not authenticated\n"},
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

// assertColored wants stdout to hold each of these, ANSI codes included.
func assertColored(colored ...string) func(*testing.T, cliResult) {
	return func(t *testing.T, res cliResult) {
		t.Helper()

		for _, want := range colored {
			if !strings.Contains(res.Stdout, want) {
				t.Errorf("stdout = %q, want it to hold %q", res.Stdout, want)
			}
		}
	}
}

func assertNoANSI(t *testing.T, res cliResult) {
	t.Helper()

	if strings.Contains(res.Stdout, "\x1b") {
		t.Errorf("stdout = %q, want no ANSI codes", res.Stdout)
	}
}
