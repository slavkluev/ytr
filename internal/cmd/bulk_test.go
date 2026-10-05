package cmd

import (
	"context"
	"net/http"
	"slices"
	"testing"

	ytrerrors "github.com/slavkluev/ytr/internal/errors"
	"github.com/slavkluev/ytr/internal/faketracker"
)

const (
	bulkCompleted = `{"id": "op-1", "status": "COMPLETED", "statusText": "Operation COMPLETED", "totalIssues": 2,
		"totalCompletedIssues": 2, "executionIssuePercent": 100, "executionChunkPercent": 100,
		"createdBy": {"id": "uid-a", "display": "Иван Петров"}, "createdAt": "2026-03-30T12:00:00.000+0000"}`
	bulkCompletedJSON = `{"id": "op-1", "status": "COMPLETED", "statusText": "Operation COMPLETED", "totalIssues": 2,
		"totalCompletedIssues": 2, "executionIssuePercent": 100, "executionChunkPercent": 100,
		"createdBy": "Иван Петров", "createdById": "uid-a", "createdAt": "2026-03-30T12:00:00Z", "suggestion": ""}`
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
			stdin: "STDIN-1\n", exchanges: done, json: bulkCompletedJSON,
			check: assertFirstBody(`{"queue": "TARGET", "issues": ["PROJ-1", "PROJ-2"]}`),
		},
		{
			name:      "Fields",
			args:      move("PROJ-1", "--queue", "TARGET", "--field", "priority=critical"),
			exchanges: done,
			json:      bulkCompletedJSON,
			check:     assertFirstBody(`{"queue": "TARGET", "issues": ["PROJ-1"], "values": {"priority": "critical"}}`),
		},
		{
			name:      "Keys on stdin",
			args:      move("--queue", "TARGET", "--jq", ".id"),
			stdin:     "PROJ-1\n\nPROJ-2\n  \nPROJ-1\n",
			exchanges: done,
			stdout:    "op-1\n",
			check:     assertFirstBody(`{"queue": "TARGET", "issues": ["PROJ-1", "PROJ-2"]}`),
		},
		{
			name: "JSON body",
			args: move("--from-json", `{"queue": "TARGET", "issues": ["PROJ-1"], "moveAllFields": true}`,
				"--jq", ".status"),
			exchanges: done, stdout: "COMPLETED\n",
			check: assertFirstBody(`{"queue": "TARGET", "issues": ["PROJ-1"], "moveAllFields": true}`),
		},
		{
			name: "JSON body on stdin", args: move("--from-json", "-", "--jq", ".id"),
			stdin: `{"queue": "TARGET", "issues": ["PROJ-1"]}`, exchanges: done, stdout: "op-1\n",
			check: assertFirstBody(`{"queue": "TARGET", "issues": ["PROJ-1"]}`),
		},
		{
			name:      "Keys as args are sent once",
			args:      move("PROJ-1", "PROJ-2", "PROJ-1", "--queue", "TARGET", "--jq", ".id"),
			exchanges: done,
			stdout:    "op-1\n",
			check:     assertFirstBody(`{"queue": "TARGET", "issues": ["PROJ-1", "PROJ-2"]}`),
		},
		{
			name: "Empty queue", args: move("PROJ-1", "--queue", "", "--jq", ".id"), exchanges: done, stdout: "op-1\n",
			check: assertFirstBody(`{"queue": "", "issues": ["PROJ-1"]}`),
		},
		{
			name:      "JSON keys in another case",
			args:      move("--from-json", `{"Queue": "T", "Issues": ["PROJ-1"]}`, "--jq", ".id"),
			exchanges: done,
			stdout:    "op-1\n",
			check:     assertFirstBody(`{"queue": "T", "issues": ["PROJ-1"]}`),
		},
		{
			name: "Keys as args beside JSON",
			args: move("PROJ-1", "--from-json", `{"queue": "T", "issues": ["PROJ-2"]}`),
			code: ytrerrors.ExitUserError,
			stderr: []string{
				`"message":"cannot combine --from-json with issue keys",` +
					`"suggestion":"Pass the issue keys as arguments or as the key \"issues\" in --from-json, not both"`,
			},
		},
		{
			name: "Keys as args beside JSON signed out", signedOut: true,
			args: move("PROJ-1", "--from-json", `{"queue": "T", "issues": ["PROJ-2"]}`, "--json="),
			code: ytrerrors.ExitUserError, stderr: []string{`"message":"cannot combine --from-json with issue keys"`},
		},
		{
			name:      "Keys on stdin beside JSON",
			args:      move("--from-json", `{"queue": "T", "issues": ["PROJ-2"]}`, "--jq", ".id"),
			stdin:     "PROJ-1\n",
			exchanges: done,
			stdout:    "op-1\n",
			check:     assertFirstBody(`{"queue": "T", "issues": ["PROJ-2"]}`),
		},
		{
			name: "Failed", args: move("PROJ-1", "PROJ-2", "PROJ-3", "--queue", "TARGET"),
			exchanges: []faketracker.Exchange{started, bulkFailed("3", "1")}, code: ytrerrors.ExitUserError,
			stderr: []string{
				`{"code":"bulk_failed","message":"bulk operation op-1 failed: Operation FAILED ` +
					`(1 of 3 issues completed)","operationId":"op-1","statusText":"Operation FAILED",` +
					`"totalIssues":3,"totalCompletedIssues":1,"suggestion":""}` + "\n",
			},
		},
		{
			name: "Empty stdin", args: move("--queue", "TARGET"), code: ytrerrors.ExitUserError,
			stderr: []string{
				`"message":"no issue keys provided via stdin",` +
					`"suggestion":"Pipe issue keys via stdin (one per line) or provide as arguments"`,
			},
		},
		{
			name: "Bad key on stdin", args: move("--queue", "TARGET"), stdin: "PROJ-1\ninvalid-key\n",
			code: ytrerrors.ExitUserError, stderr: []string{`"message":"invalid issue key or ID \"invalid-key\"`},
		},
		{
			name: "Bad key", args: move("bad-key", "--queue", "TARGET"), code: ytrerrors.ExitUserError,
			stderr: []string{
				`"message":"invalid issue key or ID \"bad-key\": expected QUEUE-123 or a 24-character hexadecimal ID`,
			},
		},
		{
			name: "Bad key signed out", args: move("", "--queue", "TARGET"), signedOut: true,
			code: ytrerrors.ExitUserError, stderr: []string{`"message":"invalid issue key or ID \"\"`},
		},
		{
			name: "Bad key on stdin signed out", args: move("--queue", "TARGET"), stdin: "bad\n", signedOut: true,
			code: ytrerrors.ExitUserError, stderr: []string{`"message":"invalid issue key or ID \"bad\"`},
		},
		{
			name: "Bad key before --json=", args: move("bad", "--queue", "TARGET", "--json="),
			code: ytrerrors.ExitUserError, stderr: []string{`"message":"invalid issue key or ID \"bad\"`},
		},
		{
			name: "Wait ends before the change does", args: move("PROJ-1", "--queue", "TARGET", "--timeout", "1ms"),
			exchanges: []faketracker.Exchange{started}, json: bulkStillRunning,
		},
		{
			name: "Wait ends before the change does, JSON", args: move("PROJ-1", "--queue", "TARGET", "--timeout", "0",
				"--json", "id,suggestion"),
			exchanges: []faketracker.Exchange{started},
			json:      `{"id": "op-1", "suggestion": "ytr bulk status op-1"}`,
		},
		{
			name:      "Wait ends before the change does, jq",
			args:      move("PROJ-1", "--queue", "TARGET", "--timeout", "1ms", "--jq", ".suggestion"),
			exchanges: []faketracker.Exchange{started},
			stdout:    "ytr bulk status op-1\n",
		},
		{
			name: "Default wait is one minute", args: move("--help"),
			holds: []string{"Maximum time to wait for the operation to finish (default 1m0s)"},
		},
		{
			name: "No operation ID", args: move("PROJ-1", "--queue", "TARGET"),
			exchanges: []faketracker.Exchange{trackerPOST("/v3/bulkchange/_move", `{"status": "CREATED"}`)},
			code:      ytrerrors.ExitUserError,
			stderr:    []string{`"code":"bulk_no_operation_id"`, "the API returned no operation ID"},
		},
		{
			name: "JSON body with no issues",
			args: move("--from-json", `{"queue": "TARGET", "issues": []}`),
			code: ytrerrors.ExitUserError,
			stderr: []string{`{"code":"user_error","message":"no issue keys provided",` +
				`"suggestion":"Pass them as the key \"issues\" in --from-json"}`},
		},
		{
			name: "Unknown key", args: move("--from-json", `{"queue": "TARGET", "issues": ["PROJ-1"], "bogus": 1}`),
			code: ytrerrors.ExitUserError, stderr: []string{`"code":"invalid_field"`, `"invalidFields":["bogus"]`},
		},
		failureRow(trackerNotFoundOn(http.MethodPost, "/v3/bulkchange/_move", "Queue not found"),
			move("PROJ-1", "--queue", "TARGET")...),
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
			name: "Bad key", args: update("bad-key", "--field", "a=b"), code: ytrerrors.ExitUserError,
			stderr: []string{
				`"message":"invalid issue key or ID \"bad-key\": expected QUEUE-123 or a 24-character hexadecimal ID`,
			},
		},
		{
			name: "Fields", args: update("PROJ-1", "--field", "priority=critical", "--field", "assignee=user1"),
			exchanges: []faketracker.Exchange{started, bulkStatusAnswer(bulkCompleted)}, json: bulkCompletedJSON,
			check: assertFirstBody(`{"issues": ["PROJ-1"], "values": {"priority": "critical", "assignee": "user1"}}`),
		},
		{
			name: "Polls until done", args: update("PROJ-1", "--field", "a=b"),
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
			name: "JSON body on stdin", args: update("--from-json", "-", "--jq", ".id"),
			stdin:     `{"issues": ["PROJ-1"], "values": {"priority": "critical"}}`,
			exchanges: []faketracker.Exchange{started, bulkStatusAnswer(bulkCompleted)}, stdout: "op-1\n",
			check: assertFirstBody(`{"issues": ["PROJ-1"], "values": {"priority": "critical"}}`),
		},
		{
			name: "Keys on stdin", args: update("--field", "a=b", "--jq", ".id"), stdin: "PROJ-1\nPROJ-2\n",
			exchanges: []faketracker.Exchange{started, bulkStatusAnswer(bulkCompleted)}, stdout: "op-1\n",
			check: assertFirstBody(`{"issues": ["PROJ-1", "PROJ-2"], "values": {"a": "b"}}`),
		},
		{
			name: "Issue ID", args: update("4ff3e8dae4b0e2ac00000001", "--field", "a=b", "--jq", ".id"),
			exchanges: []faketracker.Exchange{started, bulkStatusAnswer(bulkCompleted)}, stdout: "op-1\n",
			check: assertFirstBody(`{"issues": ["4ff3e8dae4b0e2ac00000001"], "values": {"a": "b"}}`),
		},
		{
			name: "Empty field value", args: update("PROJ-1", "--field", "a=", "--jq", ".id"),
			exchanges: []faketracker.Exchange{started, bulkStatusAnswer(bulkCompleted)}, stdout: "op-1\n",
			check: assertFirstBody(`{"issues": ["PROJ-1"], "values": {"a": ""}}`),
		},
		{
			name: "Failed", args: update("PROJ-1", "PROJ-2", "--field", "a=b"),
			exchanges: []faketracker.Exchange{started, bulkFailed("2", "0")}, code: ytrerrors.ExitUserError,
			stderr: []string{
				`{"code":"bulk_failed","message":"bulk operation op-1 failed: Operation FAILED ` +
					`(0 of 2 issues completed)","operationId":"op-1","statusText":"Operation FAILED",` +
					`"totalIssues":2,"totalCompletedIssues":0,"suggestion":""}` + "\n",
			},
		},
		{
			name: "Not a field", args: update("PROJ-1", "--field", "noequals"), code: ytrerrors.ExitUserError,
			stderr: []string{
				`"message":"invalid field format \"noequals\": expected key=value",` +
					`"suggestion":"Use --field key=value (e.g., --field priority=critical)"`,
			},
		},
		{
			name:      "Not a field signed out",
			args:      update("PROJ-1", "--field", "noequals"),
			signedOut: true,
			code:      ytrerrors.ExitUserError,
			stderr:    []string{`"message":"invalid field format \"noequals\": expected key=value"`},
		},
		{
			name: "Issue ID in JSON body",
			args: update(
				"--from-json",
				`{"issues": ["4ff3e8dae4b0e2ac00000001"], "values": {"a": "b"}}`,
				"--jq", ".id",
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
				`"message":"invalid issue key or ID \"bad\": expected QUEUE-123 or a 24-character hexadecimal ID`,
			},
		},
		{
			name: "Unknown key", args: update("--from-json", `{"issues": ["PROJ-1"], "bogus": 1}`),
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
			name: "Bad key", args: transition("bad-key", "--transition", "close"), code: ytrerrors.ExitUserError,
			stderr: []string{
				`"message":"invalid issue key or ID \"bad-key\": expected QUEUE-123 or a 24-character hexadecimal ID`,
			},
		},
		{
			name: "Flags", args: transition("PROJ-1", "PROJ-2", "--transition", "close", "--field", "resolution=fixed"),
			exchanges: done, json: bulkCompletedJSON,
			check: assertFirstBody(
				`{"transition": "close", "issues": ["PROJ-1", "PROJ-2"], "values": {"resolution": "fixed"}}`),
		},
		{
			name: "JSON body", args: transition("--from-json", `{"transition": "close", "issues": ["PROJ-1"]}`),
			exchanges: done, json: bulkCompletedJSON,
			check: assertFirstBody(`{"transition": "close", "issues": ["PROJ-1"]}`),
		},
		{
			name: "Keys on stdin", args: transition("--transition", "close", "--jq", ".id"), stdin: "PROJ-1\nPROJ-2\n",
			exchanges: done, stdout: "op-1\n",
			check: assertFirstBody(`{"transition": "close", "issues": ["PROJ-1", "PROJ-2"]}`),
		},
		{
			name:      "Field value holding =",
			args:      transition("PROJ-1", "--transition", "close", "--field", "k=a=b", "--jq", ".id"),
			exchanges: done,
			stdout:    "op-1\n",
			check:     assertFirstBody(`{"transition": "close", "issues": ["PROJ-1"], "values": {"k": "a=b"}}`),
		},
		{
			name: "JSON body on stdin", args: transition("--from-json", "-", "--jq", ".id"),
			stdin: `{"transition": "close", "issues": ["PROJ-1"]}`, exchanges: done, stdout: "op-1\n",
			check: assertFirstBody(`{"transition": "close", "issues": ["PROJ-1"]}`),
		},
		{
			name: "Failed", args: transition("PROJ-1", "PROJ-2", "--transition", "close"),
			exchanges: []faketracker.Exchange{started, bulkFailed("2", "0")}, code: ytrerrors.ExitUserError,
			stderr: []string{
				`{"code":"bulk_failed","message":"bulk operation op-1 failed: Operation FAILED ` +
					`(0 of 2 issues completed)","operationId":"op-1","statusText":"Operation FAILED",` +
					`"totalIssues":2,"totalCompletedIssues":0,"suggestion":""}` + "\n",
			},
		},
		{
			name: "Poll fails", args: transition("PROJ-1", "--transition", "close"),
			exchanges: []faketracker.Exchange{
				started, trackerError(http.MethodGet, "/v3/bulkchange/op-1", http.StatusInternalServerError, "Boom"),
			},
			code: ytrerrors.ExitUserError, stderr: []string{`"message":"Boom"`}, check: assertOneErrorDocument("Boom"),
		},
		{
			name: "Poll's own HTTP timeout fails like any poll",
			args: transition("PROJ-1", "--transition", "close", "--timeout", "1m"),
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
				"--jq", ".id",
			),
			exchanges: done,
			stdout:    "op-1\n",
			check:     assertFirstBody(`{"transition": "close", "issues": ["PROJ-2", "PROJ-1", "PROJ-2"]}`),
		},
		{
			name: "JSON body without issues",
			args: transition("--from-json", `{"transition": "close"}`),
			code: ytrerrors.ExitUserError,
			stderr: []string{
				`"message":"no issue keys provided",` + `"suggestion":"Pass them as the key \"issues\" in --from-json"`,
			},
		},
		{
			name:   "Unknown key",
			args:   transition("--from-json", `{"transition": "close", "issues": ["PROJ-1"], "bogus": 1}`),
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
		{name: "Every field", args: status(), exchanges: []faketracker.Exchange{completed}, json: bulkCompletedJSON},
		{
			name: "A bare change", args: status(),
			exchanges: []faketracker.Exchange{bulkStatusAnswer(`{"id": "op-1"}`)},
			json: `{"id": "op-1", "status": "", "statusText": "", "totalIssues": 0, "totalCompletedIssues": 0,
				"executionIssuePercent": 0, "executionChunkPercent": 0, "createdBy": "", "createdById": "",
				"createdAt": "", "suggestion": "ytr bulk status op-1"}`,
		},
		{
			name:      "A failed change fails as the bulk commands do",
			args:      status("--json", "status,totalIssues,totalCompletedIssues,suggestion"),
			exchanges: []faketracker.Exchange{bulkFailed("10", "3")},
			code:      ytrerrors.ExitUserError,
			stderr: []string{
				`{"code":"bulk_failed","message":"bulk operation op-1 failed: Operation FAILED ` +
					`(3 of 10 issues completed)","operationId":"op-1","statusText":"Operation FAILED",` +
					`"totalIssues":10,"totalCompletedIssues":3,"suggestion":""}` + "\n",
			},
		},
		{
			name: "A failed change fails under --jq", args: status("--jq", ".status"),
			exchanges: []faketracker.Exchange{bulkFailed("2", "0")}, code: ytrerrors.ExitUserError,
			stderr: []string{`"code":"bulk_failed"`, `"suggestion":""`},
		},
		{
			name: "An unfinished change suggests checking again", args: status("--jq", ".suggestion"),
			exchanges: []faketracker.Exchange{bulkStatusAnswer(`{"id": "op-1", "status": "CREATED"}`)},
			stdout:    "ytr bulk status op-1\n",
		},
		{
			name: "Padded ID", args: []string{"bulk", "status", " op-1 ", "--jq", ".id"},
			exchanges: []faketracker.Exchange{completed}, stdout: "op-1\n",
		},
		{
			name: "Bad arg", args: []string{"bulk", "status", " "}, code: ytrerrors.ExitUserError,
			stderr: []string{"invalid operation ID: expected a non-empty value"},
		},
		{
			name: "Bad arg before --json=", args: []string{"bulk", "status", " ", "--json="},
			code: ytrerrors.ExitUserError, stderr: []string{"invalid operation ID: expected a non-empty value"},
		},
		{
			name: "Bad arg signed out", args: []string{"bulk", "status", " "}, signedOut: true,
			code: ytrerrors.ExitUserError, stderr: []string{"invalid operation ID: expected a non-empty value"},
		},
		failureRow(trackerNotFound("/v3/bulkchange/op-1"), status()...),
	})
}
