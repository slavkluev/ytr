package cmd

import (
	"testing"

	ytrerrors "github.com/slavkluev/ytr/internal/errors"
	"github.com/slavkluev/ytr/internal/faketracker"
	"github.com/slavkluev/ytr/internal/output"
)

func TestComponentList(t *testing.T) {
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
			name:      "JSON",
			args:      []string{"component", "list", "--json", "id,name,queue,lead,leadId,description,assignAuto"},
			exchanges: []faketracker.Exchange{components},
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
			name: "Table", args: []string{"component", "list"}, exchanges: []faketracker.Exchange{components},
			stdout: "ID\tNAME\tQUEUE\tLEAD\n" +
				"1\tBackend\tPROJ\tИван Петров\n" +
				"2\tFrontend\tWEB\tИван Петров\n" +
				"5\tOrphan\t-\t-\n" +
				"\t-\t-\t-\n",
		},
		{
			name: "TTY", args: []string{"component", "list"}, term: output.Options{TTY: true, Colors: true},
			exchanges: []faketracker.Exchange{components},
			holds:     []string{"ID", "NAME", "QUEUE", "LEAD", "Backend", "PROJ", "Иван Петров"},
			check:     assertAlignedTable,
		},
		{
			name: "Quiet", args: []string{"component", "list", "--quiet"},
			exchanges: []faketracker.Exchange{components}, stdout: "1\n2\n5\n\n",
		},
		{
			name: "jq", args: []string{"component", "list", "--jq", ".[].name"},
			exchanges: []faketracker.Exchange{components}, stdout: "Backend\nFrontend\nOrphan\n\n",
		},
		{
			name: "Empty", args: []string{"component", "list"},
			exchanges: []faketracker.Exchange{trackerGET(path, `[]`)}, stdout: "No components found\n",
		},
		notFoundRow(path, "component", "list", "--json", "id"),
	})
}

func TestComponentGet(t *testing.T) {
	const path = "/v3/components/42"
	component := trackerGET(path, `{"id": 42, "name": "Backend", "queue": {"key": "PROJ"},
		"lead": {"id": "uid-a", "display": "Иван Петров"}, "description": "Backend services", "assignAuto": true}`)
	bare := trackerGET(path, `{"name": "Orphan"}`)

	runLeafRows(t, []leafRow{
		{
			name:      "JSON",
			args:      []string{"component", "get", "42", "--json", "id,name,queue,lead,leadId,description,assignAuto"},
			exchanges: []faketracker.Exchange{component},
			json: `{"id": "42", "name": "Backend", "queue": "PROJ", "lead": "Иван Петров", "leadId": "uid-a",
				"description": "Backend services", "assignAuto": true}`,
		},
		{
			name: "JSON of a bare component", args: []string{"component", "get", "42", "--jq", "."},
			exchanges: []faketracker.Exchange{bare},
			json:      `{"id": "", "name": "Orphan", "leadId": "", "assignAuto": false}`,
		},
		{
			name: "Card", args: []string{"component", "get", "42"}, exchanges: []faketracker.Exchange{component},
			stdout: "ID\t42\nName\tBackend\nQueue\tPROJ\nLead\tИван Петров\n" +
				"Description\tBackend services\nAssignAuto\tyes\n",
		},
		{
			name: "Card of a bare component", args: []string{"component", "get", "42"},
			exchanges: []faketracker.Exchange{bare},
			stdout:    "ID\t\nName\tOrphan\nQueue\t-\nLead\t-\nAssignAuto\tno\n",
		},
		{
			name: "TTY", args: []string{"component", "get", "42"}, term: output.Options{TTY: true, Colors: true},
			exchanges: []faketracker.Exchange{component},
			holds: []string{"ID:  42\nName:  Backend\nQueue:  PROJ\nLead:  Иван Петров\n" +
				"Description:  Backend services\nAssignAuto:  yes\n"},
		},
		{
			name: "Quiet", args: []string{"component", "get", "42", "--quiet"},
			exchanges: []faketracker.Exchange{component}, stdout: "42\n",
		},
		{
			name: "jq", args: []string{"component", "get", "42", "--jq", ".lead"},
			exchanges: []faketracker.Exchange{component}, stdout: "Иван Петров\n",
		},
		{
			name: "ID as given", args: []string{"component", "get", "042", "--quiet"},
			exchanges: []faketracker.Exchange{trackerGET("/v3/components/042", `{"id": 42}`)}, stdout: "42\n",
		},
		{
			name: "Bad arg", args: []string{"component", "get", "abc"}, code: ytrerrors.ExitUserError,
			stderr: []string{`invalid component ID "abc": expected a positive integer`},
		},
		{
			name: "Bad arg before the hint", args: []string{"component", "get", "abc", "--json="},
			code: ytrerrors.ExitUserError, stderr: []string{`invalid component ID "abc"`},
		},
		notFoundRow(path, "component", "get", "42", "--json", "id"),
	})
}
