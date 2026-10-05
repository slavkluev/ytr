package cmd

import (
	"net/http"
	"os"
	"path/filepath"
	"slices"
	"testing"

	ytrerrors "github.com/slavkluev/ytr/internal/errors"
	"github.com/slavkluev/ytr/internal/faketracker"
)

const (
	componentAnswer = `{"id": 42, "version": 1, "name": "Backend", "queue": {"id": "12345", "key": "PROJ"},
		"lead": {"id": "uid-a", "display": "Иван Петров"}, "description": "Backend services", "assignAuto": true}`
	componentItemJSON = `{"id": "42", "name": "Backend", "queue": "PROJ", "lead": "Иван Петров", "leadId": "uid-a",
		"description": "Backend services", "assignAuto": true}`
	componentBody = `{"name": "Backend", "queue": "PROJ", "description": "Backend services", "lead": "uid-a",
		"assignAuto": true}`
)

func TestComponentCreate(t *testing.T) {
	t.Parallel()

	const path = "/v3/components"
	created := trackerPOST(path, componentAnswer)
	create := func(extra ...string) []string { return slices.Concat([]string{"component", "create"}, extra) }
	bodyFile := filepath.Join(t.TempDir(), "component.json")
	if err := os.WriteFile(bodyFile, []byte(componentBody), 0o600); err != nil {
		t.Fatal(err)
	}

	runLeafRows(t, []leafRow{
		{
			name: "Flag body",
			args: create("--name", "Backend", "--queue", "PROJ", "--description", "Backend services",
				"--lead", "uid-a", "--assign-auto"),
			exchanges: []faketracker.Exchange{created}, body: componentBody, json: componentItemJSON,
		},
		{
			name: "Required flags only", args: create("--name", "Backend", "--queue", "PROJ"),
			exchanges: []faketracker.Exchange{created}, body: `{"name": "Backend", "queue": "PROJ"}`,
			json: componentItemJSON,
		},
		{
			name: "JSON body", args: create("--from-json", componentBody), exchanges: []faketracker.Exchange{created},
			body: componentBody, json: componentItemJSON,
		},
		{
			name: "JSON body from a file", args: create("--from-json", "@"+bodyFile),
			exchanges: []faketracker.Exchange{created}, body: componentBody, json: componentItemJSON,
		},
		{
			name: "JSON", args: create("--name", "Backend", "--queue", "PROJ", "--json", "id,queue"),
			exchanges: []faketracker.Exchange{created}, json: `{"id": "42", "queue": "PROJ"}`,
		},
		{
			name:   "Unknown key",
			args:   create("--from-json", `{"name": "x", "queue": "PROJ", "bogus": 1}`, "--json", "id"),
			code:   ytrerrors.ExitUserError,
			stderr: []string{`"code":"invalid_field"`, `"invalidFields":["bogus"]`},
		},
		{
			name:   "Stray arg",
			args:   create("42", "--name", "Backend", "--queue", "PROJ"),
			code:   ytrerrors.ExitUserError,
			stderr: []string{`unknown command "42" for "ytr component create"`},
		},
		helpRow(
			"component create",
			"Provide --name and --queue for required fields, or --from-json for full JSON input.\n\n"+
				"JSON FIELDS\n  id, name, queue, lead, leadId, description, assignAuto\n",
		),
	})
}

func TestComponentEdit(t *testing.T) {
	t.Parallel()

	const path = "/v3/components/42"
	edited := trackerPATCH(path, componentAnswer)
	edit := func(extra ...string) []string { return slices.Concat([]string{"component", "edit", "42"}, extra) }

	runLeafRows(t, []leafRow{
		{
			name: "Flag body",
			args: edit("--name", "Backend", "--queue", "PROJ", "--description", "Backend services",
				"--lead", "uid-a", "--assign-auto"),
			exchanges: []faketracker.Exchange{edited}, body: componentBody, json: componentItemJSON,
		},
		{
			name: "Partial", args: edit("--lead", "uid-b"), exchanges: []faketracker.Exchange{edited},
			body: `{"lead": "uid-b"}`, json: componentItemJSON,
		},
		{
			name:      "Partial assign-auto off",
			args:      edit("--assign-auto=false"),
			exchanges: []faketracker.Exchange{edited},
			body:      `{"assignAuto": false}`,
			json:      componentItemJSON,
		},
		{
			name: "JSON body", args: edit("--from-json", `{"name": "Backend", "description": "Updated"}`),
			exchanges: []faketracker.Exchange{edited}, body: `{"name": "Backend", "description": "Updated"}`,
			json: componentItemJSON,
		},
		{
			name: "JSON", args: edit("--name", "Backend", "--json", "id,name"),
			exchanges: []faketracker.Exchange{edited}, json: `{"id": "42", "name": "Backend"}`,
		},
		{
			name: "ID as given", args: []string{"component", "edit", "042", "--name", "Backend"},
			exchanges: []faketracker.Exchange{trackerPATCH("/v3/components/042", componentAnswer)},
			json:      componentItemJSON,
		},
		{
			name:   "Unknown key",
			args:   edit("--from-json", `{"bogus": 1}`, "--json", "id"),
			code:   ytrerrors.ExitUserError,
			stderr: []string{`"code":"invalid_field"`, `"invalidFields":["bogus"]`},
		},
		{
			name: "Bad arg", args: []string{"component", "edit", "abc", "--name", "x"}, code: ytrerrors.ExitUserError,
			stderr: []string{`invalid component ID "abc": expected a positive integer`},
		},
		failureRow(trackerNotFoundOn(http.MethodPatch, path, "Component not found"),
			edit("--name", "x", "--json", "id")...),
		helpRow("component edit", "Provide one or more flags to update, or --from-json for full JSON input.\n\n"+
			"JSON FIELDS\n  id, name, queue, lead, leadId, description, assignAuto\n"),
	})
}

func TestComponentDelete(t *testing.T) {
	t.Parallel()

	const path = "/v3/components/42"
	args := []string{"component", "delete", "42"}

	runLeafRows(t, slices.Concat(deleteRows(args, trackerDELETE(path), "42"), []leafRow{
		{
			name: "ID as given", args: []string{"component", "delete", "042"},
			exchanges: []faketracker.Exchange{trackerDELETE("/v3/components/042")},
			json:      `{"id": "042", "deleted": true}`,
		},
		{
			name: "Bad arg", args: []string{"component", "delete", "abc"}, code: ytrerrors.ExitUserError,
			stderr: []string{`invalid component ID "abc": expected a positive integer`},
		},
		failureRow(trackerNotFoundOn(http.MethodDelete, path, "Component not found"),
			slices.Concat(args, []string{"--json", "id"})...),
		helpRow("component delete", "Delete a project component from Yandex Tracker.\n\nJSON FIELDS\n  id, deleted\n"),
	}))
}
