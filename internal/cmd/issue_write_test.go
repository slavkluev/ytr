package cmd

import (
	"net/http"
	"slices"
	"testing"

	ytrerrors "github.com/slavkluev/ytr/internal/errors"
	"github.com/slavkluev/ytr/internal/faketracker"
)

const (
	issueAnswer = `{"key": "PROJ-7", "summary": "Fix login bug", "status": {"key": "open", "display": "Open"},
		"priority": {"display": "Normal"}, "type": {"display": "Task"},
		"createdBy": {"id": "uid-author", "display": "Иван Петров"},
		"assignee": {"id": "uid-assignee", "display": "Мария Иванова"},
		"createdAt": "2026-09-17T09:05:00.000+0300", "updatedAt": "2026-09-18T10:00:00.000+0000",
		"description": "Steps to reproduce"}`
	issueDetailJSON = `{"key": "PROJ-7", "summary": "Fix login bug", "status": "Open", "priority": "Normal",
		"type": "Task", "author": "Иван Петров", "authorId": "uid-author", "assignee": "Мария Иванова",
		"assigneeId": "uid-assignee", "createdAt": "2026-09-17T09:05:00+03:00", "updatedAt": "2026-09-18T10:00:00Z",
		"description": "Steps to reproduce"}`
	issueDetailFields = "key,summary,status,priority,type,author,authorId,assignee,assigneeId,createdAt,updatedAt," +
		"description"
	issueCard = "Key\tPROJ-7\nSummary\tFix login bug\nStatus\tOpen\n"
)

func TestIssueCreate(t *testing.T) {
	t.Parallel()

	const path = "/v3/issues/"
	created := trackerPOST(path, issueAnswer)
	create := func(extra ...string) []string { return slices.Concat([]string{"issue", "create"}, extra) }
	required := []string{"--queue", "PROJ", "--summary", "Fix login bug"}

	runLeafRows(t, []leafRow{
		{
			name: "Required flags", args: create(required...), exchanges: []faketracker.Exchange{created},
			body: `{"queue": "PROJ", "summary": "Fix login bug"}`, stdout: issueCard,
		},
		{
			name: "Every flag",
			args: create(slices.Concat(required, []string{
				"--description", "Steps to reproduce", "--type", "task", "--priority", "normal",
				"--assignee", "uid-assignee", "--parent", "PROJ-1",
			})...),
			exchanges: []faketracker.Exchange{created},
			body: `{"queue": "PROJ", "summary": "Fix login bug", "description": "Steps to reproduce", "type": "task",
				"priority": "normal", "assignee": "uid-assignee", "parent": "PROJ-1"}`,
			stdout: issueCard,
		},
		{
			name: "Empty description", args: create(slices.Concat(required, []string{"--description", ""})...),
			exchanges: []faketracker.Exchange{created},
			body:      `{"queue": "PROJ", "summary": "Fix login bug", "description": ""}`, stdout: issueCard,
		},
		{
			name: "JSON body", args: create("--from-json", `{"queue": "PROJ", "summary": "Fix login bug"}`),
			exchanges: []faketracker.Exchange{created},
			body:      `{"queue": "PROJ", "summary": "Fix login bug"}`, stdout: issueCard,
		},
		{
			name: "JSON body on stdin", args: create("--from-json", "-"),
			stdin:     `{"queue": "PROJ", "summary": "Fix login bug", "tags": ["backend"]}`,
			exchanges: []faketracker.Exchange{created},
			body:      `{"queue": "PROJ", "summary": "Fix login bug", "tags": ["backend"]}`, stdout: issueCard,
		},
		{
			name: "JSON", args: create(slices.Concat(required, []string{"--json", issueDetailFields})...),
			exchanges: []faketracker.Exchange{created}, json: issueDetailJSON,
		},
		{
			name: "Quiet", args: create(slices.Concat(required, []string{"--quiet"})...),
			exchanges: []faketracker.Exchange{created}, stdout: "PROJ-7\n",
		},
		{
			name: "jq", args: create(slices.Concat(required, []string{"--jq", ".key"})...),
			exchanges: []faketracker.Exchange{created}, stdout: "PROJ-7\n",
		},
		{
			name: "Card of a bare issue", args: create(required...),
			exchanges: []faketracker.Exchange{trackerPOST(path, `{"key": "PROJ-7"}`)},
			stdout:    "Key\tPROJ-7\nSummary\t-\nStatus\t-\n",
		},
		{
			name: "Missing queue", args: create("--summary", "Fix login bug"), code: ytrerrors.ExitUserError,
			stderr: []string{"Error: missing --queue\n"},
		},
		{
			name: "Bad value", args: create("--queue", "PROJ", "--summary", "hello\x00world"),
			code:   ytrerrors.ExitUserError,
			stderr: []string{"Error: control character U+0000 at position 5 in summary\n"},
		},
		{
			name: "Bad description", args: create(slices.Concat(required, []string{"--description", "a\x01b"})...),
			code:   ytrerrors.ExitUserError,
			stderr: []string{"Error: control character U+0001 at position 1 in description\n"},
		},
		{
			name: "Bad value signed out", args: create("--queue", "PROJ", "--summary", "a\x00"),
			signedOut: true, code: ytrerrors.ExitUserError,
			stderr: []string{"Error: control character U+0000 at position 1 in summary\n"},
		},
		{
			name: "Bad value before the hint", args: create("--queue", "PROJ", "--summary", "a\x00", "--json="),
			signedOut: true, code: ytrerrors.ExitUserError,
			stderr: []string{"Error: control character U+0000 at position 1 in summary\n"},
		},
		{
			name: "Flag next to JSON signed out", args: create("--summary", "x", "--from-json", "{}"),
			signedOut: true, code: ytrerrors.ExitUserError,
			stderr: []string{"Error: cannot combine --from-json with --summary\n"},
		},
		{
			name:   "Bad value in JSON",
			args:   create("--from-json", `{"queue": "PROJ", "summary": "a\u0000"}`),
			code:   ytrerrors.ExitUserError,
			stderr: []string{"Error: control character U+0000 at position 1 in summary\n"},
		},
		{
			name: "Unknown key",
			args: create("--from-json", `{"queue": "PROJ", "summary": "Fix login bug", "size": ["L"]}`,
				"--json", "key"),
			code:   ytrerrors.ExitUserError,
			stderr: []string{`"code":"invalid_field"`, `"invalidFields":["size"]`},
		},
		{
			name: "Malformed JSON", args: create("--from-json", `{"queue":`), code: ytrerrors.ExitUserError,
			stderr: []string{"Error: invalid JSON input: unexpected end of JSON input\n"},
		},
		failureRow(trackerNotFoundOn(http.MethodPost, path, "Queue not found"),
			create(slices.Concat(required, []string{"--json", "key"})...)...),
	})
}

func TestIssueUpdate(t *testing.T) {
	t.Parallel()

	const path = "/v3/issues/PROJ-7"
	edited := trackerPATCH(path, issueAnswer)
	update := func(extra ...string) []string {
		return slices.Concat([]string{"issue", "update", "PROJ-7"}, extra)
	}

	runLeafRows(t, []leafRow{
		{
			name: "One flag", args: update("--summary", "Fix login bug"), exchanges: []faketracker.Exchange{edited},
			body: `{"summary": "Fix login bug"}`, stdout: issueCard,
		},
		{
			name: "Every flag",
			args: update("--summary", "Fix login bug", "--description", "Steps to reproduce", "--type", "task",
				"--priority", "critical", "--assignee", "uid-assignee", "--parent", "PROJ-1"),
			exchanges: []faketracker.Exchange{edited},
			body: `{"summary": "Fix login bug", "description": "Steps to reproduce", "type": "task",
				"priority": "critical", "assignee": "uid-assignee", "parent": "PROJ-1"}`,
			stdout: issueCard,
		},
		{
			name: "Empty assignee", args: update("--assignee", ""), exchanges: []faketracker.Exchange{edited},
			body: `{"assignee": ""}`, stdout: issueCard,
		},
		{
			name: "Empty description", args: update("--description", ""), exchanges: []faketracker.Exchange{edited},
			body: `{"description": ""}`, stdout: issueCard,
		},
		{
			name: "Empty JSON value", args: update("--from-json", `{"assignee": ""}`),
			exchanges: []faketracker.Exchange{edited}, body: `{"assignee": ""}`, stdout: issueCard,
		},
		{
			name: "JSON body", args: update("--from-json", `{"summary": "Fix login bug"}`),
			exchanges: []faketracker.Exchange{edited}, body: `{"summary": "Fix login bug"}`, stdout: issueCard,
		},
		{
			name: "JSON body on stdin", args: update("--from-json", "-"), stdin: `{"priority": "critical"}`,
			exchanges: []faketracker.Exchange{edited}, body: `{"priority": "critical"}`, stdout: issueCard,
		},
		{
			name: "JSON", args: update("--summary", "x", "--json", issueDetailFields),
			exchanges: []faketracker.Exchange{edited}, json: issueDetailJSON,
		},
		{
			name: "Quiet", args: update("--summary", "x", "--quiet"), exchanges: []faketracker.Exchange{edited},
			stdout: "PROJ-7\n",
		},
		{
			name: "Bad value", args: update("--summary", "hello\x00world"), code: ytrerrors.ExitUserError,
			stderr: []string{"Error: control character U+0000 at position 5 in summary\n"},
		},
		{
			name: "Bad description", args: update("--description", "a\x01b"), code: ytrerrors.ExitUserError,
			stderr: []string{"Error: control character U+0001 at position 1 in description\n"},
		},
		{
			name: "Bad description signed out", args: update("--description", "a\x00"),
			signedOut: true, code: ytrerrors.ExitUserError,
			stderr: []string{"Error: control character U+0000 at position 1 in description\n"},
		},
		{
			name: "Bad description before the hint", args: update("--description", "a\x00", "--json="),
			signedOut: true, code: ytrerrors.ExitUserError,
			stderr: []string{"Error: control character U+0000 at position 1 in description\n"},
		},
		{
			name: "Bad description in JSON", args: update("--from-json", `{"description": "a\u0001b"}`),
			code:   ytrerrors.ExitUserError,
			stderr: []string{"Error: control character U+0001 at position 1 in description\n"},
		},
		{
			name:   "Bad value in JSON under a key in another case",
			args:   update("--from-json", `{"Summary": "a\u0000"}`),
			code:   ytrerrors.ExitUserError,
			stderr: []string{"Error: control character U+0000 at position 1 in summary\n"},
		},
		{
			name:      "Tab and line breaks",
			args:      update("--description", "Steps:\r\n\t1. Log in\n"),
			exchanges: []faketracker.Exchange{edited},
			body:      `{"description": "Steps:\r\n\t1. Log in\n"}`,
			stdout:    issueCard,
		},
		{
			name:      "Tab and line breaks in JSON",
			args:      update("--from-json", `{"description": "Steps:\r\n\t1. Log in\n"}`),
			exchanges: []faketracker.Exchange{edited},
			body:      `{"description": "Steps:\r\n\t1. Log in\n"}`,
			stdout:    issueCard,
		},
		{
			name: "Bad arg", args: []string{"issue", "update", "bad-key", "--summary", "x"},
			code: ytrerrors.ExitUserError, stderr: []string{`invalid issue key "bad-key"`},
		},
		failureRow(trackerNotFoundOn(http.MethodPatch, path, "Issue not found"),
			update("--summary", "x", "--json", "key")...),
	})
}

func TestIssueTransition(t *testing.T) {
	t.Parallel()

	const path = "/v3/issues/PROJ-123/transitions"
	transitions := trackerGET(path, `[
		{"id": "1", "to": {"key": "open", "display": "Open"}},
		{"id": "2", "to": {"key": "inProgress", "display": "In Progress"}},
		{"id": "3", "to": {"key": "closed", "display": "Closed"}}]`)
	executed := trackerWrite(http.MethodPost, path+"/2/_execute", http.StatusOK, `[]`)
	transition := func(extra ...string) []string {
		return slices.Concat([]string{"issue", "transition", "PROJ-123"}, extra)
	}
	both := []faketracker.Exchange{transitions, executed}

	runLeafRows(t, []leafRow{
		{
			name: "By key", args: transition("--to", "inProgress"), exchanges: both,
			stdout: "PROJ-123 transitioned to In Progress\n",
		},
		{
			name: "By display name", args: transition("--to", "In Progress"), exchanges: both,
			stdout: "PROJ-123 transitioned to In Progress\n",
		},
		{
			name: "By display name in another case", args: transition("--to", "in progress"), exchanges: both,
			stdout: "PROJ-123 transitioned to In Progress\n",
		},
		{
			name: "JSON", args: transition("--to", "inProgress", "--json", "key,transition"), exchanges: both,
			json: `{"key": "PROJ-123", "transition": "In Progress"}`,
		},
		{
			name: "Quiet", args: transition("--to", "inProgress", "--quiet"), exchanges: both,
			stdout: "PROJ-123\n",
		},
		{
			name: "jq", args: transition("--to", "inProgress", "--jq", ".transition"), exchanges: both,
			stdout: "In Progress\n",
		},
		{
			name: "Status without a display name", args: transition("--to", "review"),
			exchanges: []faketracker.Exchange{
				trackerGET(path, `[{"id": "2", "to": {"key": "review"}}]`), executed,
			},
			stdout: "PROJ-123 transitioned to review\n",
		},
		{
			name: "Not available", args: transition("--to", "nonexistent"),
			exchanges: []faketracker.Exchange{transitions}, code: ytrerrors.ExitUserError,
			stderr: []string{
				"Error: transition \"nonexistent\" is not available for PROJ-123\n" +
					"Valid transitions: Open, In Progress, Closed\n" +
					"Try: ytr issue transition PROJ-123 --to \"Open\"\n",
			},
		},
		{
			name: "Empty target", args: transition("--to", ""), exchanges: []faketracker.Exchange{transitions},
			code: ytrerrors.ExitUserError,
			stderr: []string{
				"Error: transition \"\" is not available for PROJ-123\n" +
					"Valid transitions: Open, In Progress, Closed\n",
			},
		},
		{
			name: "Bad arg", args: []string{"issue", "transition", "bad-key", "--to", "open"},
			code: ytrerrors.ExitUserError, stderr: []string{`invalid issue key "bad-key"`},
		},
		named("Transitions fail", failureRow(trackerNotFoundOn(http.MethodGet, path, "Issue not found"),
			transition("--to", "inProgress", "--json", "key")...)),
		{
			name: "Execute fails", args: transition("--to", "inProgress", "--json", "key"),
			exchanges: []faketracker.Exchange{
				transitions, trackerError(http.MethodPost, path+"/2/_execute", http.StatusInternalServerError, "Boom"),
			},
			code: ytrerrors.ExitUserError, stderr: []string{`"message":"Boom"`},
			check: assertOneErrorDocument("Boom"),
		},
	})
}
