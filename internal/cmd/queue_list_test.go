package cmd

import (
	"net/http"
	"slices"
	"strconv"
	"testing"

	ytrerrors "github.com/slavkluev/ytr/internal/errors"
	"github.com/slavkluev/ytr/internal/faketracker"
	"github.com/slavkluev/ytr/internal/output"
)

func queuePage(page, perPage, total int, queues string) faketracker.Exchange {
	return countedPage(http.MethodGet, "/v3/queues", page, perPage, total, queues)
}

// countedPage answers method on a page-numbered list the way Tracker answers
// one: trackerPage plus the X-Total-Pages it sends beside X-Total-Count.
func countedPage(method, path string, page, perPage, total int, items string) faketracker.Exchange {
	ex := trackerPage(method, path, page, perPage, total, items)
	ex.Header.Set("X-Total-Pages", strconv.Itoa((total+perPage-1)/perPage))

	return ex
}

func listedQueue(key string) string {
	return `{"key": "` + key + `", "name": "Queue ` + key + `", "lead": {"id": "uid-` + key + `", "display": "lead-` +
		key + `"}}`
}

func TestQueueList(t *testing.T) {
	t.Parallel()

	two := queuePage(1, 50, 2, "["+listedQueue("PROJ")+","+listedQueue("TEST")+"]")
	empty := queuePage(1, 50, 0, `[]`)
	list := func(extra ...string) []string { return slices.Concat([]string{"queue", "list"}, extra) }

	runLeafRows(t, []leafRow{
		{
			name: "Table", args: list(), exchanges: []faketracker.Exchange{two},
			stdout: "KEY\tNAME\tLEAD\nPROJ\tQueue PROJ\tlead-PROJ\nTEST\tQueue TEST\tlead-TEST\n",
		},
		{
			name: "TTY", args: list(), term: output.Options{TTY: true, Colors: true},
			exchanges: []faketracker.Exchange{two},
			holds:     []string{"KEY   NAME        LEAD", "PROJ  Queue PROJ  lead-PROJ", "TEST  Queue TEST  lead-TEST"},
			check:     assertAlignedTable,
		},
		{
			name: "JSON", args: list("--json", "key,name,lead,leadId"), exchanges: []faketracker.Exchange{two},
			json: `{"items": [
				{"key": "PROJ", "name": "Queue PROJ", "lead": "lead-PROJ", "leadId": "uid-PROJ"},
				{"key": "TEST", "name": "Queue TEST", "lead": "lead-TEST", "leadId": "uid-TEST"}],
				"pagination": {"hasMore": false, "total": 2}}`,
		},
		{
			name: "JSON of a bare queue", args: list("--json", "key,name,lead,leadId"),
			exchanges: []faketracker.Exchange{queuePage(1, 50, 1, `[{"key": "NIL-Q", "name": "Queue with nils"}]`)},
			json: `{"items": [{"key": "NIL-Q", "name": "Queue with nils", "leadId": ""}],
				"pagination": {"hasMore": false, "total": 1}}`,
		},
		{
			name: "Table of a bare queue", args: list(),
			exchanges: []faketracker.Exchange{queuePage(1, 50, 1, `[{"key": "NIL-Q", "name": "Queue with nils"}]`)},
			stdout:    "KEY\tNAME\tLEAD\nNIL-Q\tQueue with nils\t-\n",
		},
		{
			name: "Namesakes keep their lead IDs", args: list("--jq", "[.items[].leadId]"),
			exchanges: []faketracker.Exchange{queuePage(1, 50, 2, `[
				{"key": "PROJ", "lead": {"id": "uid-a", "display": "Иван Петров"}},
				{"key": "TEST", "lead": {"id": "uid-b", "display": "Иван Петров"}}]`)},
			json: `["uid-a", "uid-b"]`,
		},
		{
			name: "A full page has more", args: list("--limit", "2", "--json", "key"),
			exchanges: []faketracker.Exchange{queuePage(1, 2, 5, "["+listedQueue("PROJ")+","+listedQueue("TEST")+"]")},
			json: `{"items": [{"key": "PROJ"}, {"key": "TEST"}],
				"pagination": {"cursor": "2", "hasMore": true, "total": 5}}`,
		},
		{
			name: "Quiet", args: list("--quiet"), exchanges: []faketracker.Exchange{two}, stdout: "PROJ\nTEST\n",
		},
		{
			name: "jq", args: list("--jq", ".items[].name"), exchanges: []faketracker.Exchange{two},
			stdout: "Queue PROJ\nQueue TEST\n",
		},
		{
			name: "Limit", args: list("--limit", "10", "--quiet"),
			exchanges: []faketracker.Exchange{queuePage(1, 10, 0, `[]`)},
		},
		{
			name: "Limit over the maximum", args: list("--limit", "2000", "--quiet"),
			exchanges: []faketracker.Exchange{queuePage(1, 1000, 0, `[]`)},
		},
		{
			name: "Limit under one", args: list("--limit", "0", "--quiet"),
			exchanges: []faketracker.Exchange{empty},
		},
		{
			name: "Cursor", args: list("--cursor", "3", "--quiet"),
			exchanges: []faketracker.Exchange{queuePage(3, 50, 0, `[]`)},
		},
		{
			name: "Not a page cursor", args: list("--cursor", "abc"), code: ytrerrors.ExitUserError,
			stderr: []string{"invalid cursor"},
		},
		{
			name: "All pages", args: list("--all", "--limit", "2", "--quiet"),
			exchanges: []faketracker.Exchange{
				queuePage(1, 2, 3, "["+listedQueue("A")+","+listedQueue("B")+"]"),
				queuePage(2, 2, 3, "["+listedQueue("C")+"]"),
			},
			stdout: "A\nB\nC\n",
			check:  assertRequestOrder("page=1&perPage=2", "page=2&perPage=2"),
		},
		{
			name: "All pages end on a full page", args: list("--all", "--limit", "2", "--quiet"),
			exchanges: []faketracker.Exchange{
				queuePage(1, 2, 4, "["+listedQueue("A")+","+listedQueue("B")+"]"),
				queuePage(2, 2, 4, "["+listedQueue("C")+","+listedQueue("D")+"]"),
			},
			stdout: "A\nB\nC\nD\n",
			check:  assertRequestOrder("page=1&perPage=2", "page=2&perPage=2"),
		},
		{
			name: "All pages as JSON", args: list("--all", "--limit", "2", "--json", "key"),
			exchanges: []faketracker.Exchange{
				queuePage(1, 2, 3, "["+listedQueue("A")+","+listedQueue("B")+"]"),
				queuePage(2, 2, 3, "["+listedQueue("C")+"]"),
			},
			json: `{"items": [{"key": "A"}, {"key": "B"}, {"key": "C"}], "pagination": {"hasMore": false, "total": 3}}`,
		},
		{
			name: "A later page fails", args: list("--all", "--limit", "2", "--json", "key"),
			exchanges: []faketracker.Exchange{
				queuePage(1, 2, 3, "["+listedQueue("A")+","+listedQueue("B")+"]"),
				withQuery(
					trackerError(http.MethodGet, "/v3/queues", http.StatusInternalServerError, "Queues unavailable"),
					pageQuery(2, 2),
				),
			},
			code: ytrerrors.ExitUserError, stderr: []string{`"message":"Queues unavailable"`},
			check: assertOneErrorDocument("Queues unavailable"),
		},
		{
			name: "Empty", args: list(), exchanges: []faketracker.Exchange{empty}, stdout: "No queues found\n",
		},
		{
			name: "Empty as JSON", args: list("--json", "key"), exchanges: []faketracker.Exchange{empty},
			json: `{"items": [], "pagination": {"hasMore": false}}`,
		},
		failureRow(withQuery(trackerNotFound("/v3/queues"), pageQuery(1, 50)),
			list("--json", "key")...),
	})
}
