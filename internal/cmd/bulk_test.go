package cmd

import (
	"context"
	"net/http"
	"slices"
	"testing"

	ytrerrors "github.com/slavkluev/ytr/internal/errors"
	"github.com/slavkluev/ytr/internal/faketracker"
	"github.com/slavkluev/ytr/internal/output"
)

const (
	bulkCompleted = `{"id": "op-1", "status": "COMPLETED", "statusText": "Operation COMPLETED", "totalIssues": 2,
		"totalCompletedIssues": 2, "executionIssuePercent": 100, "executionChunkPercent": 100,
		"createdBy": {"id": "uid-a", "display": "Иван Петров"}, "createdAt": "2026-03-30T12:00:00.000+0000"}`
	bulkCompletedJSON = `{"id": "op-1", "status": "COMPLETED", "statusText": "Operation COMPLETED", "totalIssues": 2,
		"totalCompletedIssues": 2, "executionIssuePercent": 100, "executionChunkPercent": 100,
		"createdBy": "Иван Петров", "createdById": "uid-a", "createdAt": "2026-03-30T12:00:00Z", "suggestion": ""}`
	bulkFields = "id,status,statusText,totalIssues,totalCompletedIssues,executionIssuePercent," +
		"executionChunkPercent,createdBy,createdById,createdAt,suggestion"
	bulkTable = "ID\tSTATUS\tTOTAL\tDONE\tPERCENT\tSUGGESTION\nop-1\tCOMPLETED\t2\t2\t100%\t-\n"
)

// bulkStarted answers the bulk change op starts, such as _move, with a
// change Tracker has only created.
func bulkStarted(op string) faketracker.Exchange {
	return trackerPOST("/v3/bulkchange/"+op, `{"id": "op-1", "status": "CREATED"}`)
}

// bulkStillRunning is the document of a change still running as bulkStarted
// left it, once the wait for it has ended.
const bulkStillRunning = `{"id": "op-1", "status": "CREATED", "statusText": "", "totalIssues": 0,
	"totalCompletedIssues": 0, "executionIssuePercent": 0, "executionChunkPercent": 0, "createdBy": "",
	"createdById": "", "createdAt": "", "suggestion": "ytr bulk status op-1"}`

func bulkStatusAnswer(body string) faketracker.Exchange {
	return trackerGET("/v3/bulkchange/op-1", body)
}

// bulkFailed is a change that ended FAILED after completing done of total
// issues.
func bulkFailed(total, done string) faketracker.Exchange {
	return bulkStatusAnswer(`{"id": "op-1", "status": "FAILED", "statusText": "Operation FAILED",
		"totalIssues": ` + total + `, "totalCompletedIssues": ` + done + `}`)
}

// assertFirstBody wants the first request, which starts a bulk change, to
// carry want as its body; the polls that follow it carry none.
func assertFirstBody(want string) func(*testing.T, cliResult) {
	return func(t *testing.T, res cliResult) {
		t.Helper()

		if len(res.Requests) == 0 {
			t.Fatalf("no request was sent, want one with body %s", want)
		}
		assertSameJSONAs(t, "request body", res.Requests[0].Body, want)
	}
}

func TestBulkMove(t *testing.T) {
	t.Parallel()

	started := bulkStarted("_move")
	done := []faketracker.Exchange{started, bulkStatusAnswer(bulkCompleted)}
	move := func(extra ...string) []string { return slices.Concat([]string{"bulk", "move"}, extra) }

	runLeafRows(t, []leafRow{
		{
			name: "Keys as args win over stdin", args: move("PROJ-1", "PROJ-2", "--queue", "TARGET"),
			stdin: "STDIN-1\n", exchanges: done, stdout: bulkTable,
			check: assertFirstBody(`{"queue": "TARGET", "issues": ["PROJ-1", "PROJ-2"]}`),
		},
		{
			name:      "Fields",
			args:      move("PROJ-1", "--queue", "TARGET", "--field", "priority=critical", "--json", bulkFields),
			exchanges: done,
			json:      bulkCompletedJSON,
			check:     assertFirstBody(`{"queue": "TARGET", "issues": ["PROJ-1"], "values": {"priority": "critical"}}`),
		},
		{
			name: "Keys on stdin", args: move("--queue", "TARGET", "--quiet"), stdin: "PROJ-1\n\nPROJ-2\n  \nPROJ-1\n",
			exchanges: done, stdout: "op-1\n",
			check: assertFirstBody(`{"queue": "TARGET", "issues": ["PROJ-1", "PROJ-2"]}`),
		},
		{
			name: "JSON body",
			args: move("--from-json", `{"queue": "TARGET", "issues": ["PROJ-1"], "moveAllFields": true}`,
				"--jq", ".status"),
			exchanges: done, stdout: "COMPLETED\n",
			check: assertFirstBody(`{"queue": "TARGET", "issues": ["PROJ-1"], "moveAllFields": true}`),
		},
		{
			name: "JSON body on stdin", args: move("--from-json", "-", "--quiet"),
			stdin: `{"queue": "TARGET", "issues": ["PROJ-1"]}`, exchanges: done, stdout: "op-1\n",
			check: assertFirstBody(`{"queue": "TARGET", "issues": ["PROJ-1"]}`),
		},
		{
			name:      "Keys as args are sent once",
			args:      move("PROJ-1", "PROJ-2", "PROJ-1", "--queue", "TARGET", "--quiet"),
			exchanges: done,
			stdout:    "op-1\n",
			check:     assertFirstBody(`{"queue": "TARGET", "issues": ["PROJ-1", "PROJ-2"]}`),
		},
		{
			name: "Empty queue", args: move("PROJ-1", "--queue", "", "--quiet"), exchanges: done, stdout: "op-1\n",
			check: assertFirstBody(`{"queue": "", "issues": ["PROJ-1"]}`),
		},
		{
			name:      "JSON keys in another case",
			args:      move("--from-json", `{"Queue": "T", "Issues": ["PROJ-1"]}`, "--quiet"),
			exchanges: done,
			stdout:    "op-1\n",
			check:     assertFirstBody(`{"queue": "T", "issues": ["PROJ-1"]}`),
		},
		{
			name: "Keys as args beside JSON",
			args: move("PROJ-1", "--from-json", `{"queue": "T", "issues": ["PROJ-2"]}`),
			code: ytrerrors.ExitUserError,
			stderr: []string{
				"Error: cannot combine --from-json with issue keys\n" +
					`Pass the issue keys as arguments or as the key "issues" in --from-json, not both` + "\n",
			},
		},
		{
			name: "Keys as args beside JSON signed out", signedOut: true,
			args: move("PROJ-1", "--from-json", `{"queue": "T", "issues": ["PROJ-2"]}`, "--json="),
			code: ytrerrors.ExitUserError, stderr: []string{"Error: cannot combine --from-json with issue keys\n"},
		},
		{
			name:      "Keys on stdin beside JSON",
			args:      move("--from-json", `{"queue": "T", "issues": ["PROJ-2"]}`, "--quiet"),
			stdin:     "PROJ-1\n",
			exchanges: done,
			stdout:    "op-1\n",
			check:     assertFirstBody(`{"queue": "T", "issues": ["PROJ-2"]}`),
		},
		{
			name: "Failed", args: move("PROJ-1", "PROJ-2", "PROJ-3", "--queue", "TARGET", "--quiet"),
			exchanges: []faketracker.Exchange{started, bulkFailed("3", "1")}, code: ytrerrors.ExitUserError,
			stderr: []string{
				"Error: bulk operation op-1 failed: Operation FAILED (1 of 3 issues completed)\nytr bulk status op-1\n",
			},
		},
		{
			name: "Empty stdin", args: move("--queue", "TARGET"), code: ytrerrors.ExitUserError,
			stderr: []string{
				"Error: no issue keys provided via stdin\n" +
					"Pipe issue keys via stdin (one per line) or provide as arguments\n",
			},
		},
		{
			name: "Bad key on stdin", args: move("--queue", "TARGET"), stdin: "PROJ-1\ninvalid-key\n",
			code: ytrerrors.ExitUserError, stderr: []string{`Error: invalid issue key or ID "invalid-key"`},
		},
		{
			name: "Bad key", args: move("bad-key", "--queue", "TARGET"), code: ytrerrors.ExitUserError,
			stderr: []string{
				`Error: invalid issue key or ID "bad-key": expected QUEUE-123 or a 24-character hexadecimal ID`,
			},
		},
		{
			name: "Bad key signed out", args: move("", "--queue", "TARGET"), signedOut: true,
			code: ytrerrors.ExitUserError, stderr: []string{`Error: invalid issue key or ID ""`},
		},
		{
			name: "Bad key on stdin signed out", args: move("--queue", "TARGET"), stdin: "bad\n", signedOut: true,
			code: ytrerrors.ExitUserError, stderr: []string{`Error: invalid issue key or ID "bad"`},
		},
		{
			name: "Bad key before the hint", args: move("bad", "--queue", "TARGET", "--json="),
			code: ytrerrors.ExitUserError, stderr: []string{`Error: invalid issue key or ID "bad"`},
		},
		{
			name: "Wait ends before the change does", args: move("PROJ-1", "--queue", "TARGET", "--timeout", "1ms"),
			exchanges: []faketracker.Exchange{started},
			stdout:    "ID\tSTATUS\tTOTAL\tDONE\tPERCENT\tSUGGESTION\nop-1\tCREATED\t-\t-\t-\tytr bulk status op-1\n",
		},
		{
			name: "Wait ends before the change does, JSON", args: move("PROJ-1", "--queue", "TARGET", "--timeout", "0",
				"--json", bulkFields),
			exchanges: []faketracker.Exchange{started}, json: bulkStillRunning,
		},
		{
			name:      "Wait ends before the change does, jq",
			args:      move("PROJ-1", "--queue", "TARGET", "--timeout", "1ms", "--jq", ".suggestion"),
			exchanges: []faketracker.Exchange{started},
			stdout:    "ytr bulk status op-1\n",
		},
		{
			name:      "Wait ends before the change does, quiet",
			args:      move("PROJ-1", "--queue", "TARGET", "--timeout", "1ms", "--quiet"),
			exchanges: []faketracker.Exchange{started},
			stdout:    "op-1\n",
		},
		{
			name: "Default wait is one minute", args: move("--help"),
			holds: []string{"Maximum time to wait for the operation to finish (default 1m0s)"},
		},
		{
			name: "No operation ID", args: move("PROJ-1", "--queue", "TARGET", "--json", "id"),
			exchanges: []faketracker.Exchange{trackerPOST("/v3/bulkchange/_move", `{"status": "CREATED"}`)},
			code:      ytrerrors.ExitUserError,
			stderr:    []string{`"code":"bulk_no_operation_id"`, "the API returned no operation ID"},
		},
		{
			name: "JSON body with no issues",
			args: move("--from-json", `{"queue": "TARGET", "issues": []}`, "--json", "id"),
			code: ytrerrors.ExitUserError,
			stderr: []string{`{"code":"user_error","message":"no issue keys provided",` +
				`"suggestion":"Pass them as the key \"issues\" in --from-json"}`},
		},
		{
			name: "Unknown key", args: move("--from-json", `{"queue": "TARGET", "issues": ["PROJ-1"], "bogus": 1}`,
				"--json", "id"),
			code: ytrerrors.ExitUserError, stderr: []string{`"code":"invalid_field"`, `"invalidFields":["bogus"]`},
		},
		failureRow(trackerNotFoundOn(http.MethodPost, "/v3/bulkchange/_move", "Queue not found"),
			move("PROJ-1", "--queue", "TARGET", "--json", "id")...),
	})
}

func TestBulkUpdate(t *testing.T) {
	t.Parallel()

	started := bulkStarted("_update")
	update := func(extra ...string) []string { return slices.Concat([]string{"bulk", "update"}, extra) }
	unfinished := bulkStatusAnswer(`{"id": "op-1", "status": "CREATED", "statusText": "Bulk change task created.",
		"totalIssues": 2, "totalCompletedIssues": 0}`)
	stillRunningAfterPoll := `{"id": "op-1", "status": "CREATED", "statusText": "Bulk change task created.",
		"totalIssues": 2, "suggestion": "ytr bulk status op-1"}`

	runLeafRows(t, []leafRow{
		{
			name: "Fields", args: update("PROJ-1", "--field", "priority=critical", "--field", "assignee=user1"),
			exchanges: []faketracker.Exchange{started, bulkStatusAnswer(bulkCompleted)}, stdout: bulkTable,
			check: assertFirstBody(`{"issues": ["PROJ-1"], "values": {"priority": "critical", "assignee": "user1"}}`),
		},
		{
			name: "Polls until done", args: update("PROJ-1", "--field", "a=b", "--json", bulkFields),
			exchanges: []faketracker.Exchange{
				started,
				bulkStatusAnswer(`{"id": "op-1", "status": "RUNNING", "totalIssues": 2, "totalCompletedIssues": 1}`),
				bulkStatusAnswer(bulkCompleted),
			},
			json: bulkCompletedJSON,
		},
		{
			name: "Wait ends after a poll", args: update("PROJ-1", "--field", "a=b", "--timeout", "2s",
				"--json", "id,status,statusText,totalIssues,suggestion"),
			exchanges: []faketracker.Exchange{started, unfinished},
			json:      stillRunningAfterPoll,
		},
		{
			name: "Wait ends during a poll", args: update("PROJ-1", "--field", "a=b", "--timeout", "4s",
				"--json", "id,status,statusText,totalIssues,suggestion"),
			exchanges: []faketracker.Exchange{
				started, unfinished, {Method: http.MethodGet, Path: "/v3/bulkchange/op-1", Stall: true},
			},
			json: stillRunningAfterPoll,
		},
		{
			name: "JSON body on stdin", args: update("--from-json", "-", "--quiet"),
			stdin:     `{"issues": ["PROJ-1"], "values": {"priority": "critical"}}`,
			exchanges: []faketracker.Exchange{started, bulkStatusAnswer(bulkCompleted)}, stdout: "op-1\n",
			check: assertFirstBody(`{"issues": ["PROJ-1"], "values": {"priority": "critical"}}`),
		},
		{
			name: "Keys on stdin", args: update("--field", "a=b", "--quiet"), stdin: "PROJ-1\nPROJ-2\n",
			exchanges: []faketracker.Exchange{started, bulkStatusAnswer(bulkCompleted)}, stdout: "op-1\n",
			check: assertFirstBody(`{"issues": ["PROJ-1", "PROJ-2"], "values": {"a": "b"}}`),
		},
		{
			name: "Issue ID", args: update("4ff3e8dae4b0e2ac00000001", "--field", "a=b", "--quiet"),
			exchanges: []faketracker.Exchange{started, bulkStatusAnswer(bulkCompleted)}, stdout: "op-1\n",
			check: assertFirstBody(`{"issues": ["4ff3e8dae4b0e2ac00000001"], "values": {"a": "b"}}`),
		},
		{
			name: "Empty field value", args: update("PROJ-1", "--field", "a=", "--quiet"),
			exchanges: []faketracker.Exchange{started, bulkStatusAnswer(bulkCompleted)}, stdout: "op-1\n",
			check: assertFirstBody(`{"issues": ["PROJ-1"], "values": {"a": ""}}`),
		},
		{
			name: "Failed", args: update("PROJ-1", "PROJ-2", "--field", "a=b", "--json", "id"),
			exchanges: []faketracker.Exchange{started, bulkFailed("2", "0")}, code: ytrerrors.ExitUserError,
			stderr: []string{
				`{"code":"bulk_failed","message":"bulk operation op-1 failed: Operation FAILED ` +
					`(0 of 2 issues completed)","operationId":"op-1","statusText":"Operation FAILED",` +
					`"totalIssues":2,"totalCompletedIssues":0,"suggestion":"ytr bulk status op-1"}` + "\n",
			},
		},
		{
			name: "Not a field", args: update("PROJ-1", "--field", "noequals"), code: ytrerrors.ExitUserError,
			stderr: []string{
				"Error: invalid field format \"noequals\": expected key=value\n" +
					"Use --field key=value (e.g., --field priority=critical)\n",
			},
		},
		{
			name:      "Not a field signed out",
			args:      update("PROJ-1", "--field", "noequals"),
			signedOut: true,
			code:      ytrerrors.ExitUserError,
			stderr:    []string{"Error: invalid field format \"noequals\": expected key=value\n"},
		},
		{
			name: "Issue ID in JSON body",
			args: update(
				"--from-json",
				`{"issues": ["4ff3e8dae4b0e2ac00000001"], "values": {"a": "b"}}`,
				"--quiet",
			),
			exchanges: []faketracker.Exchange{started, bulkStatusAnswer(bulkCompleted)},
			stdout:    "op-1\n",
			check:     assertFirstBody(`{"issues": ["4ff3e8dae4b0e2ac00000001"], "values": {"a": "b"}}`),
		},
		{
			name: "Bad issue in JSON body",
			args: update("--from-json", `{"issues": ["PROJ-1", "bad"], "values": {"a": "b"}}`),
			code: ytrerrors.ExitUserError,
			stderr: []string{
				`Error: invalid issue key or ID "bad": expected QUEUE-123 or a 24-character hexadecimal ID`,
			},
		},
		{
			name: "Unknown key", args: update("--from-json", `{"issues": ["PROJ-1"], "bogus": 1}`, "--json", "id"),
			code: ytrerrors.ExitUserError, stderr: []string{`"code":"invalid_field"`, `"invalidFields":["bogus"]`},
		},
	})
}

func TestBulkTransition(t *testing.T) {
	t.Parallel()

	started := bulkStarted("_transition")
	done := []faketracker.Exchange{started, bulkStatusAnswer(bulkCompleted)}
	transition := func(extra ...string) []string { return slices.Concat([]string{"bulk", "transition"}, extra) }

	runLeafRows(t, []leafRow{
		{
			name: "Flags", args: transition("PROJ-1", "PROJ-2", "--transition", "close", "--field", "resolution=fixed"),
			exchanges: done, stdout: bulkTable,
			check: assertFirstBody(
				`{"transition": "close", "issues": ["PROJ-1", "PROJ-2"], "values": {"resolution": "fixed"}}`),
		},
		{
			name: "JSON body", args: transition("--from-json", `{"transition": "close", "issues": ["PROJ-1"]}`,
				"--json", bulkFields),
			exchanges: done, json: bulkCompletedJSON,
			check: assertFirstBody(`{"transition": "close", "issues": ["PROJ-1"]}`),
		},
		{
			name: "Keys on stdin", args: transition("--transition", "close", "--quiet"), stdin: "PROJ-1\nPROJ-2\n",
			exchanges: done, stdout: "op-1\n",
			check: assertFirstBody(`{"transition": "close", "issues": ["PROJ-1", "PROJ-2"]}`),
		},
		{
			name:      "Field value holding =",
			args:      transition("PROJ-1", "--transition", "close", "--field", "k=a=b", "--quiet"),
			exchanges: done,
			stdout:    "op-1\n",
			check:     assertFirstBody(`{"transition": "close", "issues": ["PROJ-1"], "values": {"k": "a=b"}}`),
		},
		{
			name: "JSON body on stdin", args: transition("--from-json", "-", "--quiet"),
			stdin: `{"transition": "close", "issues": ["PROJ-1"]}`, exchanges: done, stdout: "op-1\n",
			check: assertFirstBody(`{"transition": "close", "issues": ["PROJ-1"]}`),
		},
		{
			name: "Failed", args: transition("PROJ-1", "PROJ-2", "--transition", "close"),
			exchanges: []faketracker.Exchange{started, bulkFailed("2", "0")}, code: ytrerrors.ExitUserError,
			stderr: []string{
				"Error: bulk operation op-1 failed: Operation FAILED (0 of 2 issues completed)\nytr bulk status op-1\n",
			},
		},
		{
			name: "Poll fails", args: transition("PROJ-1", "--transition", "close", "--json", "id"),
			exchanges: []faketracker.Exchange{
				started, trackerError(http.MethodGet, "/v3/bulkchange/op-1", http.StatusInternalServerError, "Boom"),
			},
			code: ytrerrors.ExitUserError, stderr: []string{`"message":"Boom"`}, check: assertOneErrorDocument("Boom"),
		},
		{
			name: "Poll's own HTTP timeout fails like any poll",
			args: transition("PROJ-1", "--transition", "close", "--timeout", "1m", "--json", "id"),
			exchanges: []faketracker.Exchange{
				started, {Method: http.MethodGet, Path: "/v3/bulkchange/op-1", Err: context.DeadlineExceeded},
			},
			code: ytrerrors.ExitUserError, stderr: []string{"API request failed", "context deadline exceeded"},
		},
		{
			name: "JSON issues are sent as given",
			args: transition(
				"--from-json",
				`{"transition": "close", "issues": ["PROJ-2", "PROJ-1", "PROJ-2"]}`,
				"--quiet",
			),
			exchanges: done,
			stdout:    "op-1\n",
			check:     assertFirstBody(`{"transition": "close", "issues": ["PROJ-2", "PROJ-1", "PROJ-2"]}`),
		},
		{
			name: "JSON body without issues", args: transition("--from-json", `{"transition": "close"}`),
			code:   ytrerrors.ExitUserError,
			stderr: []string{"Error: no issue keys provided\nPass them as the key \"issues\" in --from-json\n"},
		},
		{
			name: "Unknown key",
			args: transition("--from-json", `{"transition": "close", "issues": ["PROJ-1"], "bogus": 1}`,
				"--json", "id"),
			code:   ytrerrors.ExitUserError,
			stderr: []string{`"code":"invalid_field"`, `"invalidFields":["bogus"]`},
		},
	})
}

func TestBulkStatus(t *testing.T) {
	t.Parallel()

	completed := bulkStatusAnswer(bulkCompleted)
	status := func(extra ...string) []string { return slices.Concat([]string{"bulk", "status", "op-1"}, extra) }

	runLeafRows(t, []leafRow{
		{name: "Table", args: status(), exchanges: []faketracker.Exchange{completed}, stdout: bulkTable},
		{
			name: "TTY", args: status(), term: output.Options{TTY: true, Colors: true},
			exchanges: []faketracker.Exchange{completed},
			holds:     []string{"ID    STATUS     TOTAL  DONE  PERCENT", "op-1  COMPLETED  2      2     100%"},
			check:     assertAlignedTable,
		},
		{
			name: "JSON", args: status("--json", bulkFields), exchanges: []faketracker.Exchange{completed},
			json: bulkCompletedJSON,
		},
		{
			name: "JSON of a bare change", args: status("--jq", "."),
			exchanges: []faketracker.Exchange{bulkStatusAnswer(`{"id": "op-1"}`)},
			json: `{"id": "op-1", "status": "", "statusText": "", "totalIssues": 0, "totalCompletedIssues": 0,
				"executionIssuePercent": 0, "executionChunkPercent": 0, "createdBy": "", "createdById": "",
				"createdAt": "", "suggestion": "ytr bulk status op-1"}`,
		},
		{
			name: "Quiet", args: status("--quiet"), exchanges: []faketracker.Exchange{completed}, stdout: "op-1\n",
		},
		{
			name:      "A failed change is an answer",
			args:      status("--json", "status,totalIssues,totalCompletedIssues,suggestion"),
			exchanges: []faketracker.Exchange{bulkFailed("10", "3")},
			stdout:    `{"status":"FAILED","suggestion":"","totalCompletedIssues":3,"totalIssues":10}` + "\n",
		},
		{
			name: "An unfinished change suggests checking again", args: status("--jq", ".suggestion"),
			exchanges: []faketracker.Exchange{bulkStatusAnswer(`{"id": "op-1", "status": "CREATED"}`)},
			stdout:    "ytr bulk status op-1\n",
		},
		{
			name: "Padded ID", args: []string{"bulk", "status", " op-1 ", "--quiet"},
			exchanges: []faketracker.Exchange{completed}, stdout: "op-1\n",
		},
		{
			name: "Bad arg", args: []string{"bulk", "status", " "}, code: ytrerrors.ExitUserError,
			stderr: []string{"invalid operation ID: expected a non-empty value"},
		},
		{
			name: "Bad arg before the hint", args: []string{"bulk", "status", " ", "--json="},
			code: ytrerrors.ExitUserError, stderr: []string{"invalid operation ID: expected a non-empty value"},
		},
		{
			name: "Bad arg signed out", args: []string{"bulk", "status", " "}, signedOut: true,
			code: ytrerrors.ExitUserError, stderr: []string{"invalid operation ID: expected a non-empty value"},
		},
		failureRow(trackerNotFound("/v3/bulkchange/op-1"), status("--json", "id")...),
	})
}
