package cmd

import (
	"net/http"
	"strings"
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
	everyField := `{"key": "PROJ-123", "summary": "Fix login bug", "status": "In Progress",
		"priority": "Critical", "type": "Bug", "author": "Иван Петров", "authorId": "uid-author",
		"assignee": "Иван Петров", "assigneeId": "uid-assignee", "createdAt": "2026-09-17T09:05:00+03:00",
		"updatedAt": "2026-09-18T10:00:00Z", "description": "` + description + `"}`

	runLeafRows(t, []leafRow{
		{
			name: "Every field", args: []string{"issue", "view", "PROJ-123"},
			exchanges: []faketracker.Exchange{issue}, json: everyField,
		},
		{
			name: "A bare issue", args: []string{"issue", "view", "PROJ-123"},
			exchanges: []faketracker.Exchange{bare},
			json: `{"key": "PROJ-123", "summary": "Minimal issue", "status": "", "authorId": "",
				"assigneeId": ""}`,
		},
		{
			name: "Hidden --debug still writes its lines", args: []string{"issue", "view", "PROJ-123", "--debug"},
			exchanges: []faketracker.Exchange{issue}, json: everyField,
			stderr: []string{"[debug] request GET " + path, "[debug] response 200 method=GET path=" + path},
			check: func(t *testing.T, res cliResult) {
				t.Helper()

				for line := range strings.Lines(res.Stderr) {
					if !strings.HasPrefix(line, "[debug] ") {
						t.Errorf("stderr line %q is not a debug line", line)
					}
				}
			},
		},
		{
			name: "jq", args: []string{"issue", "view", "PROJ-123", "--json", "description", "--jq", ".description"},
			exchanges: []faketracker.Exchange{issue}, stdout: description + "\n",
		},
		{
			name: "Not an issue key", args: []string{"issue", "view", "123"},
			code: ytrerrors.ExitUserError, stderr: []string{`invalid issue key \"123\"`},
		},
		{
			name: "Extra arg", args: []string{"issue", "view", "PROJ-1", "PROJ-2"}, code: ytrerrors.ExitUserError,
			stderr: []string{"accepts 1 arg(s), received 2"},
		},
		notFoundRow("/v3/issues/NOEXIST-1", "issue", "view", "NOEXIST-1"),
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
			args:      []string{"issue", "view", "PROJ-404"},
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
