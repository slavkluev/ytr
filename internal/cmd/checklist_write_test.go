package cmd

import (
	"net/http"
	"slices"
	"testing"

	ytrerrors "github.com/slavkluev/ytr/internal/errors"
	"github.com/slavkluev/ytr/internal/faketracker"
)

// checklistAnswer is the issue Tracker answers a checklist write with, its
// checklist holding items.
func checklistAnswer(method, path, items string) faketracker.Exchange {
	return trackerWrite(method, path, http.StatusOK, `{"key": "PROJ-1", "checklistItems": [`+items+`]}`)
}

// The items the create and edit rows' Tracker answers hold, as ytr prints them.
const (
	createdChecklistItem = `{"id": "item-new", "text": "Review PR", "checked": false, "assignee": "Иван Петров",
		"assigneeId": "uid-b"}`
	editedChecklistItem = `{"id": "item-2", "text": "Updated", "checked": true, "assignee": "Иван Петров",
		"assigneeId": "uid-b"}`
)

func TestChecklistCreate(t *testing.T) {
	t.Parallel()

	const path = "/v3/issues/PROJ-1/checklistItems"
	created := checklistAnswer(http.MethodPost, path, `
		{"id": "item-old", "text": "Old task", "checked": true},
		{"id": "item-new", "text": "Review PR", "checked": false,
		 "assignee": {"id": "uid-b", "display": "Иван Петров"}},
		{"id": "item-other", "text": "Another task", "checked": false}`)
	create := func(extra ...string) []string {
		return slices.Concat([]string{"checklist", "create", "PROJ-1"}, extra)
	}

	runLeafRows(t, []leafRow{
		{
			name: "Flag body", args: create("--text", "Review PR", "--assignee", "uid-b"),
			exchanges: []faketracker.Exchange{created}, body: `{"text": "Review PR", "assignee": "uid-b"}`,
			json: createdChecklistItem,
		},
		{
			name: "JSON body", args: create("--from-json", `{"text": "Review PR", "assignee": "uid-b"}`),
			exchanges: []faketracker.Exchange{created}, body: `{"text": "Review PR", "assignee": "uid-b"}`,
			json: createdChecklistItem,
		},
		{
			name: "JSON body with a key no flag sets",
			args: create("--from-json", `{"text": "Review PR", "deadline": {"date": "2026-04-01T00:00:00.000+0000",
				"deadlineType": "date"}}`),
			exchanges: []faketracker.Exchange{created},
			body: `{"text": "Review PR", "deadline": {"date": "2026-04-01T00:00:00.000+0000",
				"deadlineType": "date"}}`,
			json: createdChecklistItem,
		},
		{
			name: "JSON", args: create("--text", "Review PR", "--json", "id,text"),
			exchanges: []faketracker.Exchange{created}, json: `{"id": "item-new", "text": "Review PR"}`,
		},
		{
			name: "Newest item with the text", args: create("--text", "Review PR", "--jq", ".id"),
			exchanges: []faketracker.Exchange{checklistAnswer(http.MethodPost, path, `
				{"id": "item-1", "text": "Review PR"}, {"id": "item-2", "text": "Review PR"}, {"id": "item-3", "text": "x"}`)},
			stdout: "item-2\n",
		},
		{
			name: "Last item when none has the text", args: create("--text", "Unseen", "--jq", ".id"),
			exchanges: []faketracker.Exchange{created}, stdout: "item-other\n",
		},
		{
			name:      "The request when Tracker sends no item",
			args:      create("--text", "Review PR", "--assignee", "uid-b"),
			exchanges: []faketracker.Exchange{checklistAnswer(http.MethodPost, path, ``)},
			json:      `{"id": "", "text": "Review PR", "checked": false, "assignee": "uid-b", "assigneeId": ""}`,
		},
		{
			name: "Unknown key", args: create("--from-json", `{"text": "x", "bogus": 1}`),
			code: ytrerrors.ExitUserError, stderr: []string{`"code":"invalid_field"`, `"invalidFields":["bogus"]`},
		},
		{
			name: "Bad arg", args: []string{"checklist", "create", "bad", "--text", "x"}, code: ytrerrors.ExitUserError,
			stderr: []string{`invalid issue key \"bad\"`},
		},
		failureRow(trackerNotFoundOn(http.MethodPost, path, "Issue not found"),
			create("--text", "x")...),
		helpRow("checklist create", "for the body key of the same name; \"deadline\" has no flag.\n\n"+
			"JSON FIELDS\n  id, text, checked, assignee, assigneeId\n"),
	})
}

func TestChecklistEdit(t *testing.T) {
	t.Parallel()

	const path = "/v3/issues/PROJ-1/checklistItems/item-2"
	edited := checklistAnswer(http.MethodPatch, path, `
		{"id": "item-1", "text": "Old task", "checked": false},
		{"id": "item-2", "text": "Updated", "checked": true, "assignee": {"id": "uid-b", "display": "Иван Петров"}}`)
	edit := func(extra ...string) []string {
		return slices.Concat([]string{"checklist", "edit", "PROJ-1", "item-2"}, extra)
	}

	runLeafRows(t, []leafRow{
		{
			name: "Flag body",
			args: edit("--text", "Updated", "--checked", "--assignee", "uid-b"),
			exchanges: []faketracker.Exchange{
				edited,
			},
			body: `{"text": "Updated", "checked": true, "assignee": "uid-b"}`,
			json: editedChecklistItem,
		},
		{
			name: "Partial", args: edit("--checked=false"), exchanges: []faketracker.Exchange{edited},
			body: `{"checked": false}`, json: editedChecklistItem,
		},
		{
			name: "Partial text", args: edit("--text", "Updated"), exchanges: []faketracker.Exchange{edited},
			body: `{"text": "Updated"}`, json: editedChecklistItem,
		},
		{
			name:      "JSON body",
			args:      edit("--from-json", `{"checked": false}`),
			exchanges: []faketracker.Exchange{edited},
			body:      `{"checked": false}`,
			json:      editedChecklistItem,
		},
		{
			name: "JSON", args: edit("--checked", "--json", "id,checked"),
			exchanges: []faketracker.Exchange{edited}, json: `{"id": "item-2", "checked": true}`,
		},
		{
			name:      "The request when Tracker sends no such item",
			args:      edit("--text", "Updated", "--checked", "--assignee", "uid-b"),
			exchanges: []faketracker.Exchange{checklistAnswer(http.MethodPatch, path, `{"id": "item-1", "text": "x"}`)},
			json:      `{"id": "item-2", "text": "Updated", "checked": true, "assignee": "uid-b", "assigneeId": ""}`,
		},
		{
			name: "The request when Tracker sends no item", args: edit("--checked"),
			exchanges: []faketracker.Exchange{checklistAnswer(http.MethodPatch, path, ``)},
			json:      `{"id": "item-2", "text": "", "checked": true, "assigneeId": ""}`,
		},
		{
			name: "Trimmed ID", args: []string{"checklist", "edit", "PROJ-1", " item-2 ", "--checked", "--jq", ".id"},
			exchanges: []faketracker.Exchange{edited}, stdout: "item-2\n",
		},
		{
			name:   "Unknown key",
			args:   edit("--from-json", `{"bogus": 1}`),
			code:   ytrerrors.ExitUserError,
			stderr: []string{`"code":"invalid_field"`, `"invalidFields":["bogus"]`},
		},
		{
			name:   "Empty ID",
			args:   []string{"checklist", "edit", "PROJ-1", " ", "--checked"},
			code:   ytrerrors.ExitUserError,
			stderr: []string{"invalid checklist item ID: expected a non-empty value"},
		},
		{
			name: "Bad issue key", args: []string{"checklist", "edit", "bad", "item-2", "--checked"},
			code: ytrerrors.ExitUserError, stderr: []string{`invalid issue key \"bad\"`},
		},
		failureRow(
			trackerNotFoundOn(http.MethodPatch, path, "Checklist item not found"),
			edit("--checked")...),
		helpRow("checklist edit", "Set \"checked\" to true to mark an item as done, to false to unmark it.\n\n"+
			"JSON FIELDS\n  id, text, checked, assignee, assigneeId\n"),
	})
}

func TestChecklistDelete(t *testing.T) {
	t.Parallel()

	const path = "/v3/issues/PROJ-1/checklistItems/item-1"
	deleted := checklistAnswer(http.MethodDelete, path, `{"id": "item-2", "text": "Kept"}`)
	args := []string{"checklist", "delete", "PROJ-1", "item-1"}

	runLeafRows(t, slices.Concat(deleteRows(args, deleted, "item-1"), []leafRow{
		{
			name: "Trimmed ID", args: []string{"checklist", "delete", "PROJ-1", " item-1 "},
			exchanges: []faketracker.Exchange{deleted}, json: `{"id": "item-1", "deleted": true}`,
		},
		{
			name: "Empty ID", args: []string{"checklist", "delete", "PROJ-1", " "}, code: ytrerrors.ExitUserError,
			stderr: []string{"invalid checklist item ID: expected a non-empty value"},
		},
		{
			name:   "Bad issue key",
			args:   []string{"checklist", "delete", "bad", "item-1"},
			code:   ytrerrors.ExitUserError,
			stderr: []string{`invalid issue key \"bad\"`},
		},
		failureRow(
			trackerNotFoundOn(http.MethodDelete, path, "Checklist item not found"),
			args...),
		helpRow(
			"checklist delete",
			"Delete a checklist item from a Yandex Tracker issue.\n\nJSON FIELDS\n  id, deleted\n",
		),
	}))
}
