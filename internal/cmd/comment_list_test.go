package cmd

import (
	"net/http"
	"net/url"
	"slices"
	"strings"
	"testing"
	"time"

	ytrerrors "github.com/slavkluev/ytr/internal/errors"
	"github.com/slavkluev/ytr/internal/faketracker"
	"github.com/slavkluev/ytr/internal/output"
)

// commentPage answers the comment list of PROJ-1 after the comment cursor
// names, or from the start when cursor is empty.
func commentPage(cursor, comments string) faketracker.Exchange {
	query := url.Values{"perPage": {"100"}}
	if cursor != "" {
		query.Set("id", cursor)
	}

	return withQuery(trackerGET("/v3/issues/PROJ-1/comments", comments), query)
}

// commentThread answers the comment list of PROJ-1 with one page of comments,
// the last of which has the ID last, and then with the empty page that ends
// the list.
func commentThread(comments, last string) []faketracker.Exchange {
	return []faketracker.Exchange{commentPage("", comments), commentPage(last, `[]`)}
}

// numberedComments is a JSON array of comments with the given IDs.
func numberedComments(ids []string) string {
	comments := make([]string, len(ids))
	for i, id := range ids {
		comments[i] = `{"id": "` + id + `", "text": "body"}`
	}

	return "[" + strings.Join(comments, ",") + "]"
}

func TestCommentList(t *testing.T) {
	t.Parallel()

	const comments = `[
		{"id": 101, "text": "Fixed in abc123", "createdBy": {"id": "uid-a", "display": "john.doe"},
			"createdAt": "2026-09-19T14:22:31.000+0300", "updatedAt": "2026-09-20T10:00:00.000+0000"},
		{"id": 202, "text": "Thanks", "createdBy": {"id": "uid-b", "display": "jane.doe"},
			"createdAt": "2026-09-20T09:00:00.000+0000"}]`
	thread := commentThread(comments, "202")
	list := func(extra ...string) []string { return slices.Concat([]string{"comment", "list", "PROJ-1"}, extra) }

	longBody := strings.Repeat("a long comment body ", 20)

	runLeafRows(t, []leafRow{
		{
			name: "Table", args: list(), exchanges: thread,
			stdout: "ID\tAUTHOR\tDATE\tBODY\n101\tjohn.doe\t2026-09-19T14:22:31+03:00\tFixed in abc123\n" +
				"202\tjane.doe\t2026-09-20T09:00:00Z\tThanks\n",
		},
		{
			name: "JSON", args: list("--json", "id,author,authorId,body,createdAt,updatedAt"),
			exchanges: thread,
			json: `[{"id": "101", "author": "john.doe", "authorId": "uid-a", "body": "Fixed in abc123",
					"createdAt": "2026-09-19T14:22:31+03:00", "updatedAt": "2026-09-20T10:00:00Z"},
				{"id": "202", "author": "jane.doe", "authorId": "uid-b", "body": "Thanks",
					"createdAt": "2026-09-20T09:00:00Z"}]`,
		},
		{
			name: "JSON of a comment without an author", args: list("--jq", "."),
			exchanges: commentThread(`[{"id": 7}]`, "7"),
			json:      `[{"id": "7", "author": "", "authorId": "", "body": "", "createdAt": ""}]`,
		},
		{
			name: "Table of a bare comment", args: list(), exchanges: commentThread(`[{"id": 7}]`, "7"),
			stdout: "ID\tAUTHOR\tDATE\tBODY\n7\t-\t-\t\n",
		},
		{
			name: "Namesakes keep their author IDs", args: list("--jq", "[.[].authorId]"),
			exchanges: commentThread(`[
				{"id": 1, "createdBy": {"id": "uid-a", "display": "Иван Петров"}},
				{"id": 2, "createdBy": {"id": "uid-b", "display": "Иван Петров"}}]`, "2"),
			json: `["uid-a", "uid-b"]`,
		},
		{
			name: "Author ID alone", args: list("--json", "authorId"), exchanges: thread,
			json: `[{"authorId": "uid-a"}, {"authorId": "uid-b"}]`,
		},
		{
			name: "Quiet", args: list("--quiet"), exchanges: thread, stdout: "101\n202\n",
		},
		{
			name: "Empty", args: list(), exchanges: []faketracker.Exchange{commentPage("", `[]`)},
			stdout: "No comments found\n",
		},
		{
			name: "Body on one escaped line", args: list(),
			exchanges: commentThread(`[{"id": 101, "text": "first\nsecond\tthird",
				"createdBy": {"display": "john.doe"}, "createdAt": "2026-09-19T14:22:31.000+0000"}]`, "101"),
			stdout: "ID\tAUTHOR\tDATE\tBODY\n101\tjohn.doe\t2026-09-19T14:22:31Z\tfirst\\nsecond\\tthird\n",
		},
		{
			name: "A short page is not the last", args: list("--quiet"),
			exchanges: []faketracker.Exchange{
				commentPage("", numberedComments([]string{"1", "2"})),
				commentPage("2", numberedComments([]string{"3"})),
				commentPage("3", `[]`),
			},
			stdout: "1\n2\n3\n",
			check:  assertRequestOrder("perPage=100", "id=2&perPage=100", "id=3&perPage=100"),
		},
		{
			name: "A later page fails", args: list("--json", "id"),
			exchanges: []faketracker.Exchange{
				commentPage("", numberedComments([]string{"1", "2"})),
				withQuery(
					trackerError(http.MethodGet, "/v3/issues/PROJ-1/comments", http.StatusInternalServerError,
						"Comments unavailable"),
					url.Values{"perPage": {"100"}, "id": {"2"}},
				),
			},
			code: ytrerrors.ExitUserError, stderr: []string{`"message":"Comments unavailable"`},
			check: assertOneErrorDocument("Comments unavailable"),
		},
		{
			name: "A cursor that does not move", args: list("--quiet"),
			exchanges: []faketracker.Exchange{
				commentPage("", numberedComments([]string{"1", "stuck"})),
				commentPage("stuck", numberedComments([]string{"2", "stuck"})),
			},
			code:   ytrerrors.ExitUserError,
			stderr: []string{`cannot page past cursor "stuck": the last item of the page has ID "stuck"`},
		},
		{
			name: "A page ending in null", args: list("--quiet"),
			exchanges: []faketracker.Exchange{commentPage("", `[{"id": 1}, null]`)},
			code:      ytrerrors.ExitUserError,
			stderr:    []string{`cannot page past cursor "": the last item of the page has ID ""`},
		},
		{
			name: "TTY", args: list(), term: output.Options{TTY: true, Colors: true},
			exchanges: commentThread(`[{"id": 101, "text": "Fixed in abc123",
				"createdBy": {"display": "john.doe"}, "createdAt": "`+trackerTime(time.Now())+`"}]`, "101"),
			holds: []string{"ID   AUTHOR    DATE      BODY", "101  john.doe  just now  Fixed in abc123"},
			check: assertAlignedTable,
		},
		{
			name: "Long body on a TTY", args: list(), term: output.Options{TTY: true, Colors: true},
			exchanges: commentThread(`[{"id": 101, "text": "`+longBody+`",
				"createdBy": {"display": "john.doe"}, "createdAt": "`+trackerTime(time.Now())+`"}]`, "101"),
			holds: []string{"101  john.doe  just now  a long comment body a long commen..."},
		},
		{
			name: "Not an issue key", args: []string{"comment", "list", "bad-key"}, code: ytrerrors.ExitUserError,
			stderr: []string{`invalid issue key "bad-key": expected format QUEUE-123`},
		},
		failureRow(withQuery(trackerNotFound("/v3/issues/PROJ-1/comments"), url.Values{"perPage": {"100"}}),
			list("--json", "id")...),
	})
}
