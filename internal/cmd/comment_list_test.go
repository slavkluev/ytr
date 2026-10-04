package cmd

import (
	"net/url"
	"slices"
	"strconv"
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

// numberedComments is a JSON array of comments with the given IDs.
func numberedComments(ids []string) string {
	comments := make([]string, len(ids))
	for i, id := range ids {
		comments[i] = `{"id": "` + id + `", "text": "body"}`
	}

	return "[" + strings.Join(comments, ",") + "]"
}

func TestCommentList(t *testing.T) {
	const comments = `[
		{"id": 101, "text": "Fixed in abc123", "createdBy": {"id": "uid-a", "display": "john.doe"},
			"createdAt": "2026-09-19T14:22:31.000+0300", "updatedAt": "2026-09-20T10:00:00.000+0000"},
		{"id": 202, "text": "Thanks", "createdBy": {"id": "uid-b", "display": "jane.doe"},
			"createdAt": "2026-09-20T09:00:00.000+0000"}]`
	page := commentPage("", comments)
	list := func(extra ...string) []string { return slices.Concat([]string{"comment", "list", "PROJ-1"}, extra) }

	hundredTwenty := make([]string, 120)
	for i := range hundredTwenty {
		hundredTwenty[i] = strconv.Itoa(i + 1)
	}
	stuck := func(page int) []string {
		ids := make([]string, 100)
		for i := range ids {
			ids[i] = "page-" + strconv.Itoa(page) + "-" + strconv.Itoa(i)
		}
		ids[len(ids)-1] = "stuck"
		return ids
	}
	longBody := strings.Repeat("a long comment body ", 20)

	runLeafRows(t, []leafRow{
		{
			name: "Table", args: list(), exchanges: []faketracker.Exchange{page},
			stdout: "ID\tAUTHOR\tDATE\tBODY\n101\tjohn.doe\t2026-09-19T14:22:31+03:00\tFixed in abc123\n" +
				"202\tjane.doe\t2026-09-20T09:00:00Z\tThanks\n",
		},
		{
			name: "JSON", args: list("--json", "id,author,authorId,body,createdAt,updatedAt"),
			exchanges: []faketracker.Exchange{page},
			json: `[{"id": "101", "author": "john.doe", "authorId": "uid-a", "body": "Fixed in abc123",
					"createdAt": "2026-09-19T14:22:31+03:00", "updatedAt": "2026-09-20T10:00:00Z"},
				{"id": "202", "author": "jane.doe", "authorId": "uid-b", "body": "Thanks",
					"createdAt": "2026-09-20T09:00:00Z"}]`,
		},
		{
			name: "JSON of a comment without an author", args: list("--jq", "."),
			exchanges: []faketracker.Exchange{commentPage("", `[{"id": 7}]`)},
			json:      `[{"id": "7", "author": "", "authorId": "", "body": "", "createdAt": ""}]`,
		},
		{
			name: "Table of a bare comment", args: list(), exchanges: []faketracker.Exchange{commentPage("", `[{}]`)},
			stdout: "ID\tAUTHOR\tDATE\tBODY\n\t-\t-\t\n",
		},
		{
			name: "Namesakes keep their author IDs", args: list("--jq", "[.[].authorId]"),
			exchanges: []faketracker.Exchange{commentPage("", `[
				{"id": 1, "createdBy": {"id": "uid-a", "display": "Иван Петров"}},
				{"id": 2, "createdBy": {"id": "uid-b", "display": "Иван Петров"}}]`)},
			json: `["uid-a", "uid-b"]`,
		},
		{
			name: "Author ID alone", args: list("--json", "authorId"), exchanges: []faketracker.Exchange{page},
			json: `[{"authorId": "uid-a"}, {"authorId": "uid-b"}]`,
		},
		{
			name: "Quiet", args: list("--quiet"), exchanges: []faketracker.Exchange{page}, stdout: "101\n202\n",
		},
		{
			name: "Empty", args: list(), exchanges: []faketracker.Exchange{commentPage("", `[]`)},
			stdout: "No comments found\n",
		},
		{
			name: "Body on one escaped line", args: list(),
			exchanges: []faketracker.Exchange{commentPage("", `[{"id": 101, "text": "first\nsecond\tthird",
				"createdBy": {"display": "john.doe"}, "createdAt": "2026-09-19T14:22:31.000+0000"}]`)},
			stdout: "ID\tAUTHOR\tDATE\tBODY\n101\tjohn.doe\t2026-09-19T14:22:31Z\tfirst\\nsecond\\tthird\n",
		},
		{
			name: "Past the first page", args: list("--quiet"),
			exchanges: []faketracker.Exchange{
				commentPage("", numberedComments(hundredTwenty[:100])),
				commentPage("100", numberedComments(hundredTwenty[100:])),
			},
			stdout: strings.Join(hundredTwenty, "\n") + "\n",
			check:  assertRequestOrder("perPage=100", "id=100&perPage=100"),
		},
		{
			name: "A cursor that does not move", args: list("--quiet"),
			exchanges: []faketracker.Exchange{
				commentPage("", numberedComments(stuck(1))), commentPage("stuck", numberedComments(stuck(2))),
			},
			stdout: strings.Join(slices.Concat(stuck(1), stuck(2)), "\n") + "\n",
		},
		{
			name: "TTY", args: list(), term: output.Options{TTY: true, Colors: true},
			exchanges: []faketracker.Exchange{commentPage("", `[{"id": 101, "text": "Fixed in abc123",
				"createdBy": {"display": "john.doe"}, "createdAt": "`+trackerTime(time.Now())+`"}]`)},
			holds: []string{"ID   AUTHOR    DATE      BODY", "101  john.doe  just now  Fixed in abc123"},
			check: assertAlignedTable,
		},
		{
			name: "Long body on a TTY", args: list(), term: output.Options{TTY: true, Colors: true},
			exchanges: []faketracker.Exchange{commentPage("", `[{"id": 101, "text": "`+longBody+`",
				"createdBy": {"display": "john.doe"}, "createdAt": "`+trackerTime(time.Now())+`"}]`)},
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
