package cmd

import (
	"testing"
	"time"

	ytrerrors "github.com/slavkluev/ytr/internal/errors"
	"github.com/slavkluev/ytr/internal/faketracker"
	"github.com/slavkluev/ytr/internal/output"
)

func TestWorklogList(t *testing.T) {
	const path = "/v3/issues/PROJ-1/worklog"
	worklogs := trackerGET(path, `[
		{"id": 101, "createdBy": {"id": "uid-a", "display": "Иван Петров"},
		 "start": "2026-03-30T10:00:00.000+0000", "duration": "PT1H30M", "comment": "Bug fix"},
		{"id": 102, "createdBy": {"id": "uid-b", "display": "Иван Петров"},
		 "start": "2026-09-17T09:05:00.000+0300", "duration": "PT45M"},
		{"id": 103}
	]`)
	recent := trackerGET(path, `[{"id": 101, "createdBy": {"display": "Иван Петров"},
		"start": "`+trackerTime(time.Now().Add(-150*time.Minute))+`", "duration": "PT1H30M"}]`)
	empty := trackerGET(path, `[]`)
	tty := output.Options{TTY: true, Colors: true}

	runLeafRows(t, []leafRow{
		{
			name:      "JSON",
			args:      []string{"worklog", "list", "PROJ-1", "--json", "id,author,authorId,duration,start,comment"},
			exchanges: []faketracker.Exchange{worklogs},
			json: `[
				{"id": "101", "author": "Иван Петров", "authorId": "uid-a", "duration": "PT1H30M",
				 "start": "2026-03-30T10:00:00Z", "comment": "Bug fix"},
				{"id": "102", "author": "Иван Петров", "authorId": "uid-b", "duration": "PT45M",
				 "start": "2026-09-17T09:05:00+03:00"},
				{"id": "103", "author": "", "authorId": "", "duration": "-", "start": ""}
			]`,
		},
		{
			name: "Table", args: []string{"worklog", "list", "PROJ-1"}, exchanges: []faketracker.Exchange{worklogs},
			stdout: "ID\tAUTHOR\tDURATION\tSTART\n" +
				"101\tИван Петров\tPT1H30M\t2026-03-30T10:00:00Z\n" +
				"102\tИван Петров\tPT45M\t2026-09-17T09:05:00+03:00\n" +
				"103\t-\t-\t-\n",
		},
		{
			name: "TTY", args: []string{"worklog", "list", "PROJ-1"}, term: tty,
			exchanges: []faketracker.Exchange{recent},
			holds:     []string{"ID", "AUTHOR", "DURATION", "START", "101", "Иван Петров", "PT1H30M", "2h ago"},
		},
		{
			name: "Quiet", args: []string{"worklog", "list", "PROJ-1", "--quiet"},
			exchanges: []faketracker.Exchange{worklogs}, stdout: "101\n102\n103\n",
		},
		{
			name: "jq", args: []string{"worklog", "list", "PROJ-1", "--jq", ".[].duration"},
			exchanges: []faketracker.Exchange{worklogs}, stdout: "PT1H30M\nPT45M\n-\n",
		},
		{
			name: "Empty", args: []string{"worklog", "list", "PROJ-1"},
			exchanges: []faketracker.Exchange{empty}, stdout: "No worklogs found\n",
		},
		{
			name: "Bad arg", args: []string{"worklog", "list", "bad"}, code: ytrerrors.ExitUserError,
			stderr: []string{`invalid issue key "bad": expected format QUEUE-123`},
		},
		{
			name: "Bad arg before the hint", args: []string{"worklog", "list", "bad", "--json="},
			code: ytrerrors.ExitUserError, stderr: []string{`invalid issue key "bad"`},
		},
		fieldHintRow("worklog list", []string{"PROJ-1"}, "id", "author", "authorId", "duration", "start", "comment"),
		notFoundRow(path, "worklog", "list", "PROJ-1", "--json", "id"),
		helpRow("worklog list", "JSON FIELDS\n  id, author, authorId, duration, start, comment\n\n"+
			"SEE ALSO\n  ytr worklog create  - Create a worklog\n  ytr worklog edit    - Edit a worklog\n"+
			"  ytr worklog delete  - Delete a worklog\n"),
	})
}

func TestLinkList(t *testing.T) {
	const path = "/v3/issues/PROJ-1/links"
	links := trackerGET(path, `[
		{"id": 101, "direction": "inward",
		 "type": {"id": "depends", "inward": "depends on", "outward": "is dependency for"},
		 "object": {"key": "PROJ-456", "summary": "Setup database"}},
		{"id": 202, "direction": "outward",
		 "type": {"id": "relates", "inward": "is related to", "outward": "relates to"},
		 "object": {"key": "PROJ-789", "summary": "Add tests"}},
		{"id": 303, "direction": "both", "type": {"id": "duplicates"}, "object": {"key": "PROJ-2"}},
		{}
	]`)

	runLeafRows(t, []leafRow{
		{
			name: "JSON", args: []string{"link", "list", "PROJ-1", "--json", "id,type,issue,summary"},
			exchanges: []faketracker.Exchange{links},
			json: `[
				{"id": "101", "type": "depends on", "issue": "PROJ-456", "summary": "Setup database"},
				{"id": "202", "type": "relates to", "issue": "PROJ-789", "summary": "Add tests"},
				{"id": "303", "type": "duplicates", "issue": "PROJ-2", "summary": ""},
				{"id": "", "type": "-", "issue": "", "summary": ""}
			]`,
		},
		{
			name: "Table", args: []string{"link", "list", "PROJ-1"}, exchanges: []faketracker.Exchange{links},
			stdout: "ID\tTYPE\tISSUE\tSUMMARY\n" +
				"101\tdepends on\tPROJ-456\tSetup database\n" +
				"202\trelates to\tPROJ-789\tAdd tests\n" +
				"303\tduplicates\tPROJ-2\t\n" +
				"\t-\t\t\n",
		},
		{
			name: "TTY", args: []string{"link", "list", "PROJ-1"}, term: output.Options{TTY: true, Colors: true},
			exchanges: []faketracker.Exchange{links},
			holds:     []string{"ID", "TYPE", "ISSUE", "SUMMARY", "depends on", "PROJ-456", "Setup database"},
		},
		{
			name: "Quiet", args: []string{"link", "list", "PROJ-1", "--quiet"},
			exchanges: []faketracker.Exchange{links}, stdout: "101\n202\n303\n\n",
		},
		{
			name: "jq", args: []string{"link", "list", "PROJ-1", "--jq", ".[0].type"},
			exchanges: []faketracker.Exchange{links}, stdout: "depends on\n",
		},
		{
			name: "Empty", args: []string{"link", "list", "PROJ-1"},
			exchanges: []faketracker.Exchange{trackerGET(path, `[]`)}, stdout: "No links found\n",
		},
		{
			name: "Bad arg", args: []string{"link", "list", "bad"}, code: ytrerrors.ExitUserError,
			stderr: []string{`invalid issue key "bad": expected format QUEUE-123`},
		},
		fieldHintRow("link list", []string{"PROJ-1"}, "id", "type", "issue", "summary"),
		notFoundRow(path, "link", "list", "PROJ-1", "--json", "id"),
		helpRow("link list", "JSON FIELDS\n  id, type, issue, summary\n\n"+
			"SEE ALSO\n  ytr link create  - Create a link to another issue\n  ytr link delete  - Delete a link\n"),
	})
}

func TestChecklistList(t *testing.T) {
	const path = "/v3/issues/PROJ-1/checklistItems"
	items := trackerGET(path, `[
		{"id": "item-1", "text": "Review code", "checked": true,
		 "assignee": {"id": "uid-a", "display": "Иван Петров"}},
		{"id": "item-2", "text": "Write tests", "checked": false,
		 "assignee": {"id": "uid-b", "display": "Иван Петров"}},
		{"id": "item-3", "text": "Deploy"},
		{}
	]`)

	runLeafRows(t, []leafRow{
		{
			name:      "JSON",
			args:      []string{"checklist", "list", "PROJ-1", "--json", "id,text,checked,assignee,assigneeId"},
			exchanges: []faketracker.Exchange{items},
			json: `[
				{"id": "item-1", "text": "Review code", "checked": true, "assignee": "Иван Петров", "assigneeId": "uid-a"},
				{"id": "item-2", "text": "Write tests", "checked": false, "assignee": "Иван Петров", "assigneeId": "uid-b"},
				{"id": "item-3", "text": "Deploy", "checked": false, "assigneeId": ""},
				{"id": "", "text": "", "checked": false, "assigneeId": ""}
			]`,
		},
		{
			name: "Table", args: []string{"checklist", "list", "PROJ-1"}, exchanges: []faketracker.Exchange{items},
			stdout: "ID\tTEXT\tCHECKED\tASSIGNEE\n" +
				"item-1\tReview code\tyes\tИван Петров\n" +
				"item-2\tWrite tests\tno\tИван Петров\n" +
				"item-3\tDeploy\tno\t-\n" +
				"\t\tno\t-\n",
		},
		{
			name: "TTY", args: []string{"checklist", "list", "PROJ-1"}, term: output.Options{TTY: true, Colors: true},
			exchanges: []faketracker.Exchange{items},
			holds:     []string{"ID", "TEXT", "CHECKED", "ASSIGNEE", "item-1", "Review code", "yes", "Иван Петров"},
		},
		{
			name: "Quiet", args: []string{"checklist", "list", "PROJ-1", "--quiet"},
			exchanges: []faketracker.Exchange{items}, stdout: "item-1\nitem-2\nitem-3\n\n",
		},
		{
			name: "jq", args: []string{"checklist", "list", "PROJ-1", "--jq", ".[].id"},
			exchanges: []faketracker.Exchange{items}, stdout: "item-1\nitem-2\nitem-3\n\n",
		},
		{
			name: "Empty", args: []string{"checklist", "list", "PROJ-1"},
			exchanges: []faketracker.Exchange{trackerGET(path, `[]`)}, stdout: "No checklist items found\n",
		},
		{
			name: "Bad arg", args: []string{"checklist", "list", "bad"}, code: ytrerrors.ExitUserError,
			stderr: []string{`invalid issue key "bad": expected format QUEUE-123`},
		},
		fieldHintRow("checklist list", []string{"PROJ-1"}, "id", "text", "checked", "assignee", "assigneeId"),
		notFoundRow(path, "checklist", "list", "PROJ-1", "--json", "id"),
		helpRow("checklist list", "JSON FIELDS\n  id, text, checked, assignee, assigneeId\n\n"+
			"SEE ALSO\n  ytr checklist create  - Add checklist item to issue\n"+
			"  ytr checklist edit    - Edit a checklist item\n  ytr checklist delete  - Delete a checklist item\n"),
	})
}
