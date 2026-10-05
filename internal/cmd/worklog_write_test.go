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
	t.Parallel()

	const path = "/v3/issues/PROJ-1/worklog"
	created := trackerPOST(path, worklogAnswer)
	create := func(extra ...string) []string {
		return slices.Concat([]string{"worklog", "create", "PROJ-1"}, extra)
	}
	required := []string{"--duration", "PT1H30M", "--start", "2026-03-30T10:00:00Z"}

	runLeafRows(t, []leafRow{
		{
			name: "Flag body", args: create(slices.Concat(required, []string{"--comment", "Code review"})...),
			exchanges: []faketracker.Exchange{created}, body: worklogBody, json: worklogItemJSON,
		},
		{
			name:      "Flag body in Tracker's own form",
			args:      create("--duration", "PT90M", "--start", "2026-03-30T13:00:00+03:00"),
			exchanges: []faketracker.Exchange{created},
			body:      `{"start": "2026-03-30T13:00:00.000+0300", "duration": "PT1H30M"}`,
			json:      worklogItemJSON,
		},
		{
			name: "JSON body",
			args: create("--from-json",
				`{"start": "2026-03-30T10:00:00Z", "duration": "PT1H30M", "comment": "Code review"}`),
			exchanges: []faketracker.Exchange{created}, body: worklogBody, json: worklogItemJSON,
		},
		{
			name: "JSON", args: create(slices.Concat(required, []string{"--json", "id,start"})...),
			exchanges: []faketracker.Exchange{created}, json: `{"id": "101", "start": "2026-03-30T10:00:00Z"}`,
		},
		{
			name: "jq", args: create(slices.Concat(required, []string{"--jq", ".duration"})...),
			exchanges: []faketracker.Exchange{created}, stdout: "PT1H30M\n",
		},
		{
			name: "Bad duration", args: create("--duration", "1h", "--start", "2026-03-30T10:00:00Z"),
			code: ytrerrors.ExitUserError, stderr: []string{
				`"message":"invalid ISO 8601 duration \"1h\"",` +
					`"suggestion":"Use ISO 8601 format: PT1H30M (1h30m), PT45M (45min), P1D (1 day), P1DT2H (1 day 2 hours)"`,
			},
		},
		{
			name:   "Bad value before the hint",
			args:   create("--duration", "1h", "--start", "2026-03-30T10:00:00Z", "--json="),
			code:   ytrerrors.ExitUserError,
			stderr: []string{`"message":"invalid ISO 8601 duration \"1h\"`},
		},
		{
			name: "Bad start",
			args: create("--duration", "PT1H", "--start", "x"),
			code: ytrerrors.ExitUserError,
			stderr: []string{
				`"message":"invalid timestamp \"x\"",` + `"suggestion":"Use RFC 3339 format: 2026-03-30T10:00:00Z"`,
			},
		},
		{
			name:   "Unknown key",
			args:   create("--from-json", `{"start": "2026-03-30T10:00:00Z", "duration": "PT1H", "bogus": 1}`),
			code:   ytrerrors.ExitUserError,
			stderr: []string{`"code":"invalid_field"`, `"invalidFields":["bogus"]`},
		},
		{
			name: "Bad arg", args: []string{"worklog", "create", "bad", "--duration", "PT1H", "--start", "x"},
			code: ytrerrors.ExitUserError, stderr: []string{`invalid issue key \"bad\"`},
		},
		named("Field hint before --from-json is read",
			fieldHintRow("worklog create", []string{"PROJ-1", "--from-json", `{"comment": "x", "bogus": 1}`},
				worklogFields...)),
		failureRow(trackerNotFoundOn(http.MethodPost, path, "Issue not found"),
			create(required...)...),
		helpRow("worklog create", "Tracker requires both duration and start time when creating a worklog.\n\n"+
			"JSON FIELDS\n  id, author, authorId, duration, start, comment\n"),
	})
}

func TestWorklogEdit(t *testing.T) {
	t.Parallel()

	const path = "/v3/issues/PROJ-1/worklog/101"
	edited := trackerPATCH(path, worklogAnswer)
	edit := func(extra ...string) []string {
		return slices.Concat([]string{"worklog", "edit", "PROJ-1", "101"}, extra)
	}

	runLeafRows(t, []leafRow{
		{
			name:      "Flag body",
			args:      edit("--duration", "PT1H30M", "--start", "2026-03-30T10:00:00Z", "--comment", "Code review"),
			exchanges: []faketracker.Exchange{edited}, body: worklogBody, json: worklogItemJSON,
		},
		{
			name: "Partial", args: edit("--comment", "Code review"), exchanges: []faketracker.Exchange{edited},
			body: `{"comment": "Code review"}`, json: worklogItemJSON,
		},
		{
			name: "Partial duration", args: edit("--duration", "PT2H"), exchanges: []faketracker.Exchange{edited},
			body: `{"duration": "PT2H"}`, json: worklogItemJSON,
		},
		{
			name:      "Partial start",
			args:      edit("--start", "2026-03-30T10:00:00Z"),
			exchanges: []faketracker.Exchange{edited},
			body:      `{"start": "2026-03-30T10:00:00.000+0000"}`,
			json:      worklogItemJSON,
		},
		{
			name:      "JSON body",
			args:      edit("--from-json", `{"duration": "PT3H"}`),
			exchanges: []faketracker.Exchange{edited},
			body:      `{"duration": "PT3H"}`,
			json:      worklogItemJSON,
		},
		{
			name: "JSON", args: edit("--duration", "PT1H30M", "--json", "id,duration"),
			exchanges: []faketracker.Exchange{edited}, json: `{"id": "101", "duration": "PT1H30M"}`,
		},
		{
			name: "Trimmed ID", args: []string{"worklog", "edit", "PROJ-1", " 101 ", "--comment", "x"},
			exchanges: []faketracker.Exchange{edited}, json: worklogItemJSON,
		},
		{
			name: "Bad duration", args: edit("--duration", "1h"), code: ytrerrors.ExitUserError,
			stderr: []string{`"message":"invalid ISO 8601 duration \"1h\"`},
		},
		{
			name: "Bad start", args: edit("--start", "x"), code: ytrerrors.ExitUserError,
			stderr: []string{`"message":"invalid timestamp \"x\"`},
		},
		{
			name:   "Unknown key",
			args:   edit("--from-json", `{"bogus": 1}`),
			code:   ytrerrors.ExitUserError,
			stderr: []string{`"code":"invalid_field"`, `"invalidFields":["bogus"]`},
		},
		{
			name: "Empty ID", args: []string{"worklog", "edit", "PROJ-1", " ", "--comment", "x"},
			code: ytrerrors.ExitUserError, stderr: []string{"invalid worklog ID: expected a non-empty value"},
		},
		{
			name: "Bad issue key", args: []string{"worklog", "edit", "bad", "101", "--comment", "x"},
			code: ytrerrors.ExitUserError, stderr: []string{`invalid issue key \"bad\"`},
		},
		failureRow(trackerNotFoundOn(http.MethodPatch, path, "Worklog not found"),
			edit("--comment", "x")...),
		helpRow("worklog edit", "Provide one or more flags to update, or --from-json for full JSON input.\n\n"+
			"JSON FIELDS\n  id, author, authorId, duration, start, comment\n"),
	})
}

func TestWorklogDelete(t *testing.T) {
	t.Parallel()

	const path = "/v3/issues/PROJ-1/worklog/101"
	args := []string{"worklog", "delete", "PROJ-1", "101"}

	runLeafRows(t, slices.Concat(deleteRows(args, trackerDELETE(path), "101"), []leafRow{
		{
			name: "Trimmed ID", args: []string{"worklog", "delete", "PROJ-1", " 101 "},
			exchanges: []faketracker.Exchange{trackerDELETE(path)}, json: `{"id": "101", "deleted": true}`,
		},
		{
			name: "Empty ID", args: []string{"worklog", "delete", "PROJ-1", " "}, code: ytrerrors.ExitUserError,
			stderr: []string{"invalid worklog ID: expected a non-empty value"},
		},
		{
			name: "Bad issue key", args: []string{"worklog", "delete", "bad", "101"}, code: ytrerrors.ExitUserError,
			stderr: []string{`invalid issue key \"bad\"`},
		},
		failureRow(trackerNotFoundOn(http.MethodDelete, path, "Worklog not found"),
			args...),
		helpRow("worklog delete", "Delete a worklog from a Yandex Tracker issue.\n\nJSON FIELDS\n  id, deleted\n"),
	}))
}
