package cmd

import (
	"testing"

	ytrerrors "github.com/slavkluev/ytr/internal/errors"
	"github.com/slavkluev/ytr/internal/faketracker"
)

func TestWorklogList(t *testing.T) {
	t.Parallel()

	const path = "/v3/issues/PROJ-1/worklog"
	worklogs := trackerGET(path, `[
		{"id": 101, "createdBy": {"id": "uid-a", "display": "Иван Петров"},
		 "start": "2026-03-30T10:00:00.000+0000", "duration": "PT1H30M", "comment": "Bug fix"},
		{"id": 102, "createdBy": {"id": "uid-b", "display": "Иван Петров"},
		 "start": "2026-09-17T09:05:00.000+0300", "duration": "PT45M"},
		{"id": 103}
	]`)
	empty := trackerGET(path, `[]`)

	runLeafRows(t, []leafRow{
		{
			name:      "Every field",
			args:      []string{"worklog", "list", "PROJ-1"},
			exchanges: []faketracker.Exchange{worklogs},
			json: wholeList(`[
				{"id": "101", "author": "Иван Петров", "authorId": "uid-a", "duration": "PT1H30M",
				 "start": "2026-03-30T10:00:00Z", "comment": "Bug fix"},
				{"id": "102", "author": "Иван Петров", "authorId": "uid-b", "duration": "PT45M",
				 "start": "2026-09-17T09:05:00+03:00"},
				{"id": "103", "author": "", "authorId": "", "duration": "", "start": ""}
			]`, 3),
		},
		{
			name:      "JSON",
			args:      []string{"worklog", "list", "PROJ-1", "--json", "id,duration"},
			exchanges: []faketracker.Exchange{worklogs},
			json: wholeList(`[{"id": "101", "duration": "PT1H30M"}, {"id": "102", "duration": "PT45M"},
				{"id": "103", "duration": ""}]`, 3),
		},
		{
			name: "jq", args: []string{"worklog", "list", "PROJ-1", "--jq", ".items[].duration"},
			exchanges: []faketracker.Exchange{worklogs}, stdout: "PT1H30M\nPT45M\n\n",
		},
		{
			name: "Empty", args: []string{"worklog", "list", "PROJ-1"},
			exchanges: []faketracker.Exchange{empty}, json: wholeList(`[]`, 0),
		},
		{
			name: "Extra arg", args: []string{"worklog", "list", "PROJ-1", "PROJ-2"}, code: ytrerrors.ExitUserError,
			stderr: []string{"accepts 1 arg(s), received 2"},
		},
		{
			name: "Bad arg", args: []string{"worklog", "list", "bad"}, code: ytrerrors.ExitUserError,
			stderr: []string{`"message":"invalid issue key \"bad\": expected format QUEUE-123"`},
		},
		{
			name: "Bad arg before --json=", args: []string{"worklog", "list", "bad", "--json="},
			code: ytrerrors.ExitUserError, stderr: []string{`invalid issue key \"bad\"`},
		},
		notFoundRow(path, "worklog", "list", "PROJ-1"),
	})
}

func TestLinkList(t *testing.T) {
	t.Parallel()

	const path = "/v3/issues/PROJ-1/links"
	links := trackerGET(path, `[
		{"id": 101, "direction": "inward",
		 "type": {"id": "depends", "inward": "depends on", "outward": "is dependency for"},
		 "object": {"key": "PROJ-456", "summary": "Setup database"}},
		{"id": 202, "direction": "outward",
		 "type": {"id": "relates", "inward": "is related to", "outward": "relates to"},
		 "object": {"key": "PROJ-789", "summary": "Add tests"}},
		{"id": 303, "direction": "both", "type": {"id": "duplicates"}, "object": {"key": "PROJ-2"}},
		{"id": 404, "direction": "inward", "type": {"id": "x"}},
		{"id": 405, "direction": "outward", "type": {"id": "x"}},
		{"id": 406, "direction": "both", "type": {}},
		{}
	]`)

	runLeafRows(t, []leafRow{
		{
			name: "Every field", args: []string{"link", "list", "PROJ-1"}, exchanges: []faketracker.Exchange{links},
			json: wholeList(`[
				{"id": "101", "type": "depends on", "issue": "PROJ-456", "summary": "Setup database"},
				{"id": "202", "type": "relates to", "issue": "PROJ-789", "summary": "Add tests"},
				{"id": "303", "type": "duplicates", "issue": "PROJ-2", "summary": ""},
				{"id": "404", "type": "", "issue": "", "summary": ""},
				{"id": "405", "type": "", "issue": "", "summary": ""},
				{"id": "406", "type": "", "issue": "", "summary": ""},
				{"id": "", "type": "", "issue": "", "summary": ""}
			]`, 7),
		},
		{
			name:      "JSON",
			args:      []string{"link", "list", "PROJ-1", "--json", "id,issue"},
			exchanges: []faketracker.Exchange{links},
			json: wholeList(`[{"id": "101", "issue": "PROJ-456"}, {"id": "202", "issue": "PROJ-789"},
				{"id": "303", "issue": "PROJ-2"}, {"id": "404", "issue": ""}, {"id": "405", "issue": ""},
				{"id": "406", "issue": ""}, {"id": "", "issue": ""}]`, 7),
		},
		{
			name: "jq", args: []string{"link", "list", "PROJ-1", "--jq", ".items[0].type"},
			exchanges: []faketracker.Exchange{links}, stdout: "depends on\n",
		},
		{
			name: "Empty", args: []string{"link", "list", "PROJ-1"},
			exchanges: []faketracker.Exchange{trackerGET(path, `[]`)}, json: wholeList(`[]`, 0),
		},
		{
			name: "Bad arg", args: []string{"link", "list", "bad"}, code: ytrerrors.ExitUserError,
			stderr: []string{`"message":"invalid issue key \"bad\": expected format QUEUE-123"`},
		},
		notFoundRow(path, "link", "list", "PROJ-1"),
	})
}

func TestChecklistList(t *testing.T) {
	t.Parallel()

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
			name:      "Every field",
			args:      []string{"checklist", "list", "PROJ-1"},
			exchanges: []faketracker.Exchange{items},
			json: wholeList(`[
				{"id": "item-1", "text": "Review code", "checked": true, "assignee": "Иван Петров", "assigneeId": "uid-a"},
				{"id": "item-2", "text": "Write tests", "checked": false, "assignee": "Иван Петров", "assigneeId": "uid-b"},
				{"id": "item-3", "text": "Deploy", "checked": false, "assigneeId": ""},
				{"id": "", "text": "", "checked": false, "assigneeId": ""}
			]`, 4),
		},
		{
			name: "JSON", args: []string{"checklist", "list", "PROJ-1", "--json", "id,checked"},
			exchanges: []faketracker.Exchange{items},
			json: wholeList(`[{"id": "item-1", "checked": true}, {"id": "item-2", "checked": false},
				{"id": "item-3", "checked": false}, {"id": "", "checked": false}]`, 4),
		},
		{
			name: "jq", args: []string{"checklist", "list", "PROJ-1", "--jq", ".items[].id"},
			exchanges: []faketracker.Exchange{items}, stdout: "item-1\nitem-2\nitem-3\n\n",
		},
		{
			name: "Empty", args: []string{"checklist", "list", "PROJ-1"},
			exchanges: []faketracker.Exchange{trackerGET(path, `[]`)}, json: wholeList(`[]`, 0),
		},
		{
			name: "Bad arg", args: []string{"checklist", "list", "bad"}, code: ytrerrors.ExitUserError,
			stderr: []string{`"message":"invalid issue key \"bad\": expected format QUEUE-123"`},
		},
		notFoundRow(path, "checklist", "list", "PROJ-1"),
	})
}
