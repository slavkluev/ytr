package cmd

import (
	"testing"
	"time"

	ytrerrors "github.com/slavkluev/ytr/internal/errors"
	"github.com/slavkluev/ytr/internal/faketracker"
	"github.com/slavkluev/ytr/internal/output"
)

func TestIssueView(t *testing.T) {
	const path = "/v3/issues/PROJ-123"
	const description = "The login page returns 500 error when submitting the form."
	issue := trackerGET(path, `{"key": "PROJ-123", "summary": "Fix login bug",
		"status": {"key": "inProgress", "display": "In Progress"}, "priority": {"display": "Critical"},
		"type": {"display": "Bug"}, "createdBy": {"id": "uid-author", "display": "Иван Петров"},
		"assignee": {"id": "uid-assignee", "display": "Иван Петров"},
		"createdAt": "2026-09-17T09:05:00.000+0300", "updatedAt": "2026-09-18T10:00:00.000+0000",
		"description": "`+description+`"}`)
	recent := trackerGET(path, `{"key": "PROJ-123", "summary": "Fix login bug",
		"status": {"key": "open"}, "priority": {"display": "Critical"}, "type": {"display": "Bug"},
		"createdBy": {"display": "Иван Петров"},
		"createdAt": "`+trackerTime(time.Now().Add(-73*time.Hour))+`",
		"updatedAt": "`+trackerTime(time.Now().Add(-150*time.Minute))+`"}`)
	bare := trackerGET(path, `{"key": "PROJ-123", "summary": "Minimal issue"}`)
	all := "key,summary,status,priority,type,author,authorId,assignee,assigneeId,createdAt,updatedAt,description"

	runLeafRows(t, []leafRow{
		{
			name: "JSON", args: []string{"issue", "view", "PROJ-123", "--json", all},
			exchanges: []faketracker.Exchange{issue},
			json: `{"key": "PROJ-123", "summary": "Fix login bug", "status": "In Progress", "priority": "Critical",
				"type": "Bug", "author": "Иван Петров", "authorId": "uid-author", "assignee": "Иван Петров",
				"assigneeId": "uid-assignee", "createdAt": "2026-09-17T09:05:00+03:00",
				"updatedAt": "2026-09-18T10:00:00Z", "description": "` + description + `"}`,
		},
		{
			name: "JSON of a bare issue", args: []string{"issue", "view", "PROJ-123", "--json", all},
			exchanges: []faketracker.Exchange{bare},
			json: `{"key": "PROJ-123", "summary": "Minimal issue", "status": "-", "authorId": "",
				"assigneeId": ""}`,
		},
		{
			name: "Card", args: []string{"issue", "view", "PROJ-123"}, exchanges: []faketracker.Exchange{issue},
			stdout: "Key\tPROJ-123\nTitle\tFix login bug\nStatus\tIn Progress\nPriority\tCritical\nType\tBug\n" +
				"Author\tИван Петров\nAssignee\tИван Петров\n" +
				"Created\t2026-09-17T09:05:00+03:00\nUpdated\t2026-09-18T10:00:00Z\n" +
				"\nDescription:\n  " + description + "\n",
		},
		{
			name: "Card of a bare issue", args: []string{"issue", "view", "PROJ-123"},
			exchanges: []faketracker.Exchange{bare},
			stdout: "Key\tPROJ-123\nTitle\tMinimal issue\nStatus\t-\nPriority\t-\nType\t-\n" +
				"Author\t-\nAssignee\t-\nCreated\t-\nUpdated\t-\n",
		},
		{
			name: "TTY", args: []string{"issue", "view", "PROJ-123"}, term: output.Options{TTY: true, Colors: true},
			exchanges: []faketracker.Exchange{recent},
			holds: []string{"Key:  PROJ-123\nTitle:  Fix login bug\nStatus:  open\nPriority:  Critical\n" +
				"Type:  Bug\nAuthor:  Иван Петров\nAssignee:  -\nCreated:  3d ago\nUpdated:  2h ago\n"},
		},
		{
			name: "Quiet", args: []string{"issue", "view", "PROJ-123", "--quiet"},
			exchanges: []faketracker.Exchange{issue}, stdout: "PROJ-123\n",
		},
		{
			name: "jq", args: []string{"issue", "view", "PROJ-123", "--json", "description", "--jq", ".description"},
			exchanges: []faketracker.Exchange{issue}, stdout: description + "\n",
		},
		{
			name: "Any arg", args: []string{"issue", "view", "123", "--quiet"},
			exchanges: []faketracker.Exchange{trackerGET("/v3/issues/123", `{"key": "PROJ-123"}`)},
			stdout:    "PROJ-123\n",
		},
		{
			name: "Extra arg", args: []string{"issue", "view", "PROJ-1", "PROJ-2"}, code: ytrerrors.ExitUserError,
			stderr: []string{"accepts 1 arg(s), received 2"},
		},
		fieldHintRow("issue view", []string{"PROJ-123"}, "key", "summary", "status", "priority", "type",
			"author", "authorId", "assignee", "assigneeId", "createdAt", "updatedAt", "description"),
		notFoundRow("/v3/issues/NOEXIST-1", "issue", "view", "NOEXIST-1", "--json", "key"),
		helpRow("issue view", "JSON FIELDS\n  key, summary, status, priority, type, author, authorId, "+
			"assignee, assigneeId, createdAt, updatedAt, description\n\n"+
			"SEE ALSO\n  ytr issue list        - List issues\n  ytr issue update      - Update an issue\n"+
			"  ytr issue transition  - Transition issue status\n"),
	})
}
