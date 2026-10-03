package cmd

import (
	"net/http"
	"slices"
	"testing"

	ytrerrors "github.com/slavkluev/ytr/internal/errors"
	"github.com/slavkluev/ytr/internal/faketracker"
)

var worklogFields = []string{"id", "author", "authorId", "duration", "start", "comment"}

const (
	worklogAnswer = `{"id": 101, "createdBy": {"id": "uid-a", "display": "Иван Петров"},
		"start": "2026-03-30T10:00:00.000+0000", "duration": "PT1H30M", "comment": "Code review"}`
	worklogItemJSON = `{"id": "101", "author": "Иван Петров", "authorId": "uid-a", "duration": "PT1H30M",
		"start": "2026-03-30T10:00:00Z", "comment": "Code review"}`
	worklogBody = `{"start": "2026-03-30T10:00:00.000+0000", "duration": "PT1H30M", "comment": "Code review"}`
)

func TestWorklogCreate(t *testing.T) {
	const path = "/v3/issues/PROJ-1/worklog"
	created := trackerPOST(path, worklogAnswer)
	create := func(extra ...string) []string {
		return slices.Concat([]string{"worklog", "create", "PROJ-1"}, extra)
	}
	required := []string{"--duration", "PT1H30M", "--start", "2026-03-30T10:00:00Z"}

	runLeafRows(t, []leafRow{
		{
			name: "Flag body", args: create(slices.Concat(required, []string{"--comment", "Code review"})...),
			exchanges: []faketracker.Exchange{created}, body: worklogBody, stdout: "Worklog 101 created on PROJ-1\n",
		},
		{
			name:      "Flag body in Tracker's own form",
			args:      create("--duration", "PT90M", "--start", "2026-03-30T13:00:00+03:00"),
			exchanges: []faketracker.Exchange{created},
			body:      `{"start": "2026-03-30T13:00:00.000+0300", "duration": "PT1H30M"}`,
			stdout:    "Worklog 101 created on PROJ-1\n",
		},
		{
			name: "JSON body",
			args: create("--from-json",
				`{"start": "2026-03-30T10:00:00Z", "duration": "PT1H30M", "comment": "Code review"}`),
			exchanges: []faketracker.Exchange{created}, body: worklogBody, stdout: "Worklog 101 created on PROJ-1\n",
		},
		{
			name: "JSON",
			args: create(
				slices.Concat(required, []string{"--json", "id,author,authorId,duration,start,comment"})...),
			exchanges: []faketracker.Exchange{created},
			json:      worklogItemJSON,
		},
		{
			name: "Quiet", args: create(slices.Concat(required, []string{"--quiet"})...),
			exchanges: []faketracker.Exchange{created}, stdout: "101\n",
		},
		{
			name: "jq", args: create(slices.Concat(required, []string{"--jq", ".duration"})...),
			exchanges: []faketracker.Exchange{created}, stdout: "PT1H30M\n",
		},
		{
			name: "Bad duration", args: create("--duration", "1h", "--start", "2026-03-30T10:00:00Z"),
			code: ytrerrors.ExitUserError, stderr: []string{
				"Error: invalid ISO 8601 duration \"1h\"\n" +
					"Use ISO 8601 format: PT1H30M (1h30m), PT45M (45min), P1D (1 day), P1DT2H (1 day 2 hours)\n",
			},
		},
		{
			name:   "Bad value before the hint",
			args:   create("--duration", "1h", "--start", "2026-03-30T10:00:00Z", "--json="),
			code:   ytrerrors.ExitUserError,
			stderr: []string{`Error: invalid ISO 8601 duration "1h"`},
		},
		{
			name: "Bad start", args: create("--duration", "PT1H", "--start", "x"), code: ytrerrors.ExitUserError,
			stderr: []string{"Error: invalid timestamp \"x\"\nUse RFC 3339 format: 2026-03-30T10:00:00Z\n"},
		},
		{
			name: "Unknown key",
			args: create("--from-json", `{"start": "2026-03-30T10:00:00Z", "duration": "PT1H", "bogus": 1}`,
				"--json", "id"),
			code:   ytrerrors.ExitUserError,
			stderr: []string{`"code":"invalid_field"`, `"invalidFields":["bogus"]`},
		},
		{
			name: "Bad arg", args: []string{"worklog", "create", "bad", "--duration", "PT1H", "--start", "x"},
			code: ytrerrors.ExitUserError, stderr: []string{`invalid issue key "bad"`},
		},
		fieldHintRow("worklog create", slices.Concat([]string{"PROJ-1"}, required), worklogFields...),
		failureRow("Tracker 404", trackerFailure(http.MethodPost, path, http.StatusNotFound, "Issue not found"),
			create(slices.Concat(required, []string{"--json", "id"})...)...),
		failureRow(
			"Tracker 500",
			trackerFailure(http.MethodPost, path, http.StatusInternalServerError, "Internal error"),
			create(slices.Concat(required, []string{"--json", "id"})...)...),
		helpRow("worklog create", "Tracker requires both duration and start time when creating a worklog.\n\n"+
			"JSON FIELDS\n  id, author, authorId, duration, start, comment\n\n"+
			"SEE ALSO\n  ytr worklog list    - List worklogs on issue\n  ytr worklog edit    - Edit a worklog\n"+
			"  ytr worklog delete  - Delete a worklog\n"),
	})
}

func TestWorklogEdit(t *testing.T) {
	const path = "/v3/issues/PROJ-1/worklog/101"
	edited := trackerPATCH(path, worklogAnswer)
	edit := func(extra ...string) []string {
		return slices.Concat([]string{"worklog", "edit", "PROJ-1", "101"}, extra)
	}

	runLeafRows(t, []leafRow{
		{
			name:      "Flag body",
			args:      edit("--duration", "PT1H30M", "--start", "2026-03-30T10:00:00Z", "--comment", "Code review"),
			exchanges: []faketracker.Exchange{edited}, body: worklogBody, stdout: "Worklog 101 updated on PROJ-1\n",
		},
		{
			name: "Partial", args: edit("--comment", "Code review"), exchanges: []faketracker.Exchange{edited},
			body: `{"comment": "Code review"}`, stdout: "Worklog 101 updated on PROJ-1\n",
		},
		{
			name: "Partial duration", args: edit("--duration", "PT2H"), exchanges: []faketracker.Exchange{edited},
			body: `{"duration": "PT2H"}`, stdout: "Worklog 101 updated on PROJ-1\n",
		},
		{
			name:      "Partial start",
			args:      edit("--start", "2026-03-30T10:00:00Z"),
			exchanges: []faketracker.Exchange{edited},
			body:      `{"start": "2026-03-30T10:00:00.000+0000"}`,
			stdout:    "Worklog 101 updated on PROJ-1\n",
		},
		{
			name:      "JSON body",
			args:      edit("--from-json", `{"duration": "PT3H"}`),
			exchanges: []faketracker.Exchange{edited},
			body:      `{"duration": "PT3H"}`,
			stdout:    "Worklog 101 updated on PROJ-1\n",
		},
		{
			name: "JSON", args: edit("--duration", "PT1H30M", "--json", "id,author,authorId,duration,start,comment"),
			exchanges: []faketracker.Exchange{edited}, json: worklogItemJSON,
		},
		{
			name: "Quiet", args: edit("--duration", "PT1H", "--quiet"), exchanges: []faketracker.Exchange{edited},
			stdout: "101\n",
		},
		{
			name: "Trimmed ID", args: []string{"worklog", "edit", "PROJ-1", " 101 ", "--comment", "x"},
			exchanges: []faketracker.Exchange{edited}, stdout: "Worklog 101 updated on PROJ-1\n",
		},
		{
			name: "Bad duration", args: edit("--duration", "1h"), code: ytrerrors.ExitUserError,
			stderr: []string{`Error: invalid ISO 8601 duration "1h"`},
		},
		{
			name: "Bad start", args: edit("--start", "x"), code: ytrerrors.ExitUserError,
			stderr: []string{`Error: invalid timestamp "x"`},
		},
		{
			name:   "Unknown key",
			args:   edit("--from-json", `{"bogus": 1}`, "--json", "id"),
			code:   ytrerrors.ExitUserError,
			stderr: []string{`"code":"invalid_field"`, `"invalidFields":["bogus"]`},
		},
		{
			name: "Empty ID", args: []string{"worklog", "edit", "PROJ-1", " ", "--comment", "x"},
			code: ytrerrors.ExitUserError, stderr: []string{"invalid worklog ID: expected a non-empty value"},
		},
		{
			name: "Bad issue key", args: []string{"worklog", "edit", "bad", "101", "--comment", "x"},
			code: ytrerrors.ExitUserError, stderr: []string{`invalid issue key "bad"`},
		},
		fieldHintRow("worklog edit", []string{"PROJ-1", "101", "--comment", "x"}, worklogFields...),
		failureRow("Tracker 404", trackerFailure(http.MethodPatch, path, http.StatusNotFound, "Worklog not found"),
			edit("--comment", "x", "--json", "id")...),
		helpRow("worklog edit", "Provide one or more flags to update, or --from-json for full JSON input.\n\n"+
			"JSON FIELDS\n  id, author, authorId, duration, start, comment\n\n"+
			"SEE ALSO\n  ytr worklog list    - List worklogs on issue\n  ytr worklog create  - Create a worklog\n"+
			"  ytr worklog delete  - Delete a worklog\n"),
	})
}

func TestWorklogDelete(t *testing.T) {
	const path = "/v3/issues/PROJ-1/worklog/101"
	args := []string{"worklog", "delete", "PROJ-1", "101"}

	runLeafRows(t, slices.Concat(deleteRows(args, trackerDELETE(path), "101", "Worklog 101 deleted"), []leafRow{
		{
			name: "Trimmed ID", args: []string{"worklog", "delete", "PROJ-1", " 101 "},
			exchanges: []faketracker.Exchange{trackerDELETE(path)}, stdout: "Worklog 101 deleted\n",
		},
		{
			name: "Empty ID", args: []string{"worklog", "delete", "PROJ-1", " "}, code: ytrerrors.ExitUserError,
			stderr: []string{"invalid worklog ID: expected a non-empty value"},
		},
		{
			name: "Bad issue key", args: []string{"worklog", "delete", "bad", "101"}, code: ytrerrors.ExitUserError,
			stderr: []string{`invalid issue key "bad"`},
		},
		failureRow("Tracker 404", trackerFailure(http.MethodDelete, path, http.StatusNotFound, "Worklog not found"),
			slices.Concat(args, []string{"--json", "id"})...),
		helpRow("worklog delete", "Delete a worklog from a Yandex Tracker issue.\n\nJSON FIELDS\n  id, deleted\n\n"+
			"SEE ALSO\n  ytr worklog list    - List worklogs on issue\n  ytr worklog create  - Create a worklog\n"+
			"  ytr worklog edit    - Edit a worklog\n"),
	}))
}
