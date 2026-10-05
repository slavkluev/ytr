package cmd

import (
	"testing"

	ytrerrors "github.com/slavkluev/ytr/internal/errors"
	"github.com/slavkluev/ytr/internal/faketracker"
)

func TestComponentList(t *testing.T) {
	t.Parallel()

	const path = "/v3/components"
	components := trackerGET(path, `[
		{"id": 1, "name": "Backend", "queue": {"key": "PROJ"},
		 "lead": {"id": "uid-a", "display": "Иван Петров"}, "description": "Backend services", "assignAuto": true},
		{"id": 2, "name": "Frontend", "queue": {"key": "WEB"},
		 "lead": {"id": "uid-b", "display": "Иван Петров"}, "assignAuto": false},
		{"id": 5, "name": "Orphan"},
		{}
	]`)

	runLeafRows(t, []leafRow{
		{
			name: "Every field", args: []string{"component", "list"}, exchanges: []faketracker.Exchange{components},
			json: `[
				{"id": "1", "name": "Backend", "queue": "PROJ", "lead": "Иван Петров", "leadId": "uid-a",
				 "description": "Backend services", "assignAuto": true},
				{"id": "2", "name": "Frontend", "queue": "WEB", "lead": "Иван Петров", "leadId": "uid-b",
				 "assignAuto": false},
				{"id": "5", "name": "Orphan", "leadId": "", "assignAuto": false},
				{"id": "", "name": "", "leadId": "", "assignAuto": false}
			]`,
		},
		{
			name: "JSON", args: []string{"component", "list", "--json", "id,leadId"},
			exchanges: []faketracker.Exchange{components},
			json: `[{"id": "1", "leadId": "uid-a"}, {"id": "2", "leadId": "uid-b"}, {"id": "5", "leadId": ""},
				{"id": "", "leadId": ""}]`,
		},
		{
			name: "jq", args: []string{"component", "list", "--jq", ".[].name"},
			exchanges: []faketracker.Exchange{components}, stdout: "Backend\nFrontend\nOrphan\n\n",
		},
		{
			name: "Empty", args: []string{"component", "list"},
			exchanges: []faketracker.Exchange{trackerGET(path, `[]`)}, json: `[]`,
		},
		notFoundRow(path, "component", "list"),
	})
}

func TestComponentGet(t *testing.T) {
	t.Parallel()

	const path = "/v3/components/42"
	component := trackerGET(path, `{"id": 42, "name": "Backend", "queue": {"key": "PROJ"},
		"lead": {"id": "uid-a", "display": "Иван Петров"}, "description": "Backend services", "assignAuto": true}`)
	bare := trackerGET(path, `{"name": "Orphan"}`)

	runLeafRows(t, []leafRow{
		{
			name: "Every field", args: []string{"component", "get", "42"}, exchanges: []faketracker.Exchange{component},
			json: `{"id": "42", "name": "Backend", "queue": "PROJ", "lead": "Иван Петров", "leadId": "uid-a",
				"description": "Backend services", "assignAuto": true}`,
		},
		{
			name: "A bare component", args: []string{"component", "get", "42"},
			exchanges: []faketracker.Exchange{bare},
			json:      `{"id": "", "name": "Orphan", "leadId": "", "assignAuto": false}`,
		},
		{
			name: "jq", args: []string{"component", "get", "42", "--jq", ".lead"},
			exchanges: []faketracker.Exchange{component}, stdout: "Иван Петров\n",
		},
		{
			name: "ID as given", args: []string{"component", "get", "042", "--jq", ".id"},
			exchanges: []faketracker.Exchange{trackerGET("/v3/components/042", `{"id": 42}`)}, stdout: "42\n",
		},
		{
			name: "Bad arg", args: []string{"component", "get", "abc"}, code: ytrerrors.ExitUserError,
			stderr: []string{`"message":"invalid component ID \"abc\": expected a positive integer"`},
		},
		{
			name: "Bad arg before --json=", args: []string{"component", "get", "abc", "--json="},
			code: ytrerrors.ExitUserError, stderr: []string{`invalid component ID \"abc\"`},
		},
		notFoundRow(path, "component", "get", "42"),
	})
}
