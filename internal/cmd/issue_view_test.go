package cmd

import (
	"net/http"
	"testing"

	ytrerrors "github.com/slavkluev/ytr/internal/errors"
	"github.com/slavkluev/ytr/internal/faketracker"
)

func TestIssueView(t *testing.T) {
	t.Parallel()

	const path = "/v3/issues/PROJ-123"
	const description = "The login page returns 500 error when submitting the form."
	issue := trackerGET(path, `{"key": "PROJ-123", "summary": "Fix login bug",
		"status": {"key": "inProgress", "display": "In Progress"}, "priority": {"display": "Critical"},
		"type": {"display": "Bug"}, "createdBy": {"id": "uid-author", "display": "Иван Петров"},
		"assignee": {"id": "uid-assignee", "display": "Иван Петров"},
		"createdAt": "2026-09-17T09:05:00.000+0300", "updatedAt": "2026-09-18T10:00:00.000+0000",
		"description": "`+description+`"}`)
	bare := trackerGET(path, `{"key": "PROJ-123", "summary": "Minimal issue"}`)
	const markdown = "See <b>R&D</b>: `a && b > c`"
	withMarkdown := trackerGET(path, `{"key": "PROJ-123", "description": "`+markdown+`"}`)
	const serverText = "<html><body>R&D-404 > archived</body></html>"

	runLeafRows(t, []leafRow{
		{
			name: "Every field", args: []string{"issue", "view", "PROJ-123"},
			exchanges: []faketracker.Exchange{issue},
			json: `{"key": "PROJ-123", "summary": "Fix login bug", "status": "In Progress", "priority": "Critical",
				"type": "Bug", "author": "Иван Петров", "authorId": "uid-author", "assignee": "Иван Петров",
				"assigneeId": "uid-assignee", "createdAt": "2026-09-17T09:05:00+03:00",
				"updatedAt": "2026-09-18T10:00:00Z", "description": "` + description + `"}`,
		},
		{
			name: "A bare issue", args: []string{"issue", "view", "PROJ-123"},
			exchanges: []faketracker.Exchange{bare},
			json: `{"key": "PROJ-123", "summary": "Minimal issue", "status": "", "authorId": "",
				"assigneeId": ""}`,
		},
		{
			name: "jq", args: []string{"issue", "view", "PROJ-123", "--json", "description", "--jq", ".description"},
			exchanges: []faketracker.Exchange{issue}, stdout: description + "\n",
		},
		{
			name: "Not an issue key", args: []string{"issue", "view", "123"},
			code: ytrerrors.ExitUserError, stderr: []string{`invalid issue key "123"`},
		},
		{
			name: "Extra arg", args: []string{"issue", "view", "PROJ-1", "PROJ-2"}, code: ytrerrors.ExitUserError,
			stderr: []string{"accepts 1 arg(s), received 2"},
		},
		notFoundRow("/v3/issues/NOEXIST-1", "issue", "view", "NOEXIST-1", "--json", "key"),
		{
			// Every field is selected by then, yet the error keeps the mode the
			// flags asked for.
			name: "Tracker 404 without an output flag", args: []string{"issue", "view", "NOEXIST-1"},
			exchanges: []faketracker.Exchange{trackerNotFound("/v3/issues/NOEXIST-1")},
			code:      ytrerrors.ExitNotFound, stderr: []string{"Error: Object not found\n"},
		},
		{
			name: "JSON keeps <, > and & literal", args: []string{"issue", "view", "PROJ-123", "--json", "description"},
			exchanges: []faketracker.Exchange{withMarkdown}, stdout: `{"description":"` + markdown + `"}` + "\n",
		},
		{
			name:      "jq result keeps <, > and & literal",
			args:      []string{"issue", "view", "PROJ-123", "--jq", "[.description]"},
			exchanges: []faketracker.Exchange{withMarkdown}, stdout: `["` + markdown + `"]` + "\n",
		},
		{
			name:      "Error document keeps <, > and & literal",
			args:      []string{"issue", "view", "PROJ-404", "--json", "key"},
			exchanges: []faketracker.Exchange{trackerNotFoundOn(http.MethodGet, "/v3/issues/PROJ-404", serverText)},
			code:      ytrerrors.ExitNotFound,
			stderr:    []string{`"message":"` + serverText + `"`},
		},
		{
			name:   "Invalid field document keeps <, > and & literal",
			args:   []string{"issue", "view", "PROJ-123", "--json", "R&D<x>"},
			code:   ytrerrors.ExitUserError,
			stderr: []string{`"message":"unknown field: \"R&D<x>\"","invalidField":"R&D<x>"`},
		},
	})
}
