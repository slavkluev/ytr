package cmd

import (
	"net/http"
	"net/url"
	"slices"
	"testing"

	ytrerrors "github.com/slavkluev/ytr/internal/errors"
	"github.com/slavkluev/ytr/internal/faketracker"
)

// The APP exchanges follow the recorded read-only responses of the Tracker
// API, cut down to the fields the document reads.
const (
	appQueueAnswer = `{"key": "APP", "name": "Demo application",
		"defaultType": {"key": "task"}, "defaultPriority": {"key": "normal"},
		"issueTypesConfig": [
			{"issueType": {"key": "task", "display": "Задача"}, "workflow": {"id": "W200"}},
			{"issueType": {"key": "bug", "display": "Ошибка"}, "workflow": {"id": "W200"}}]}`

	// A queue without defaults, with one issue type on W200 and one on W100.
	twoWorkflowQueueAnswer = `{"key": "APP", "name": "Demo application",
		"issueTypesConfig": [
			{"issueType": {"key": "task", "display": "Задача"}, "workflow": {"id": "W200"}},
			{"issueType": {"key": "milestone", "display": "Веха"}, "workflow": {"id": "W100"}}]}`

	// W200 has a terminal step without actions, and two actions of the open
	// step that lead to the same status.
	w200Answer = `{"id": "W200", "initialAction": {"target": {"key": "open", "display": "Открыт"}},
		"steps": [
			{"status": {"key": "open", "display": "Открыт"}, "actions": [
				{"target": {"key": "closed", "display": "Закрыт"}}, {"target": {"key": "closed", "display": "Закрыт"}}]},
			{"status": {"key": "closed", "display": "Закрыт"}}]}`

	// W100 starts in its own status and then shares open and closed with W200.
	w100Answer = `{"id": "W100", "initialAction": {"target": {"key": "new", "display": "Новый"}},
		"steps": [
			{"status": {"key": "new", "display": "Новый"}, "actions": [{"target": {"key": "open", "display": "Открыт"}}]},
			{"status": {"key": "open", "display": "Открыт"}, "actions": [
				{"target": {"key": "closed", "display": "Закрыт"}}]},
			{"status": {"key": "closed", "display": "Закрыт"}}]}`

	appComponentsAnswer = `[{"id": 55, "name": "Hotfix"}]`

	// A required system field and a field that is not required.
	appQueueFieldsAnswer = `[{"id": "type", "key": "type", "schema": {"required": true}},
		{"id": "stand", "key": "stand", "schema": {"required": false}}]`

	appLocalFieldsAnswer = `[{"id": "5d0e4f1a2b3c4d5e6f708192--size", "key": "size", "name": "Размер",
		"schema": {"type": "string"}, "optionsProvider": {"values": ["S", "M", "L"]}}]`

	// An editable global field and a read-only one.
	appGlobalFieldsAnswer = `[{"key": "tags", "name": "Теги"}, {"key": "key", "name": "Ключ", "readonly": true}]`

	appIssueTypesJSON = `[{"key": "task", "name": "Задача", "workflow": "W200"},
		{"key": "bug", "name": "Ошибка", "workflow": "W200"}]`
	appStatusesJSON    = `[{"key": "open", "name": "Открыт"}, {"key": "closed", "name": "Закрыт"}]`
	w200JSON           = `{"id": "W200", "initialStatus": "open", "transitions": {"open": ["closed"], "closed": []}}`
	appLocalFieldsJSON = `[{"id": "5d0e4f1a2b3c4d5e6f708192--size", "key": "size", "name": "Размер",
		"schema": "string", "readonly": false, "options": ["S", "M", "L"]}]`

	// The whole APP document, as queue context prints it with no flags.
	appContextJSON = `{"key":"APP","name":"Demo application","defaultType":"task","defaultPriority":"normal",` +
		`"issueTypes":[{"key":"task","name":"Задача","workflow":"W200"},` +
		`{"key":"bug","name":"Ошибка","workflow":"W200"}],` +
		`"statuses":[{"key":"open","name":"Открыт"},{"key":"closed","name":"Закрыт"}],` +
		`"workflows":[{"id":"W200","initialStatus":"open","transitions":{"open":["closed"],"closed":[]}}],` +
		`"components":[{"id":"55","name":"Hotfix"}],` +
		`"requiredFields":[{"id":"summary"},{"id":"type","default":"task"}],` +
		`"localFields":[{"id":"5d0e4f1a2b3c4d5e6f708192--size","key":"size","name":"Размер",` +
		`"schema":"string","readonly":false,"options":["S","M","L"]}],` +
		`"globalFields":[{"key":"tags","name":"Теги"}],"incomplete":[]}`
)

func queueAnswer(key, body string) faketracker.Exchange {
	return withQuery(trackerGET("/v3/queues/"+key, body), url.Values{"expand": {"issueTypesConfig"}})
}

func workflowAnswer(id, body string) faketracker.Exchange {
	return trackerGET("/v3/workflows/"+id, body)
}

func componentsAnswer(key, body string) faketracker.Exchange {
	return withQuery(trackerGET("/v3/queues/"+key+"/components", body), url.Values{"fields": {"name"}})
}

func queueFieldsAnswer(key, body string) faketracker.Exchange {
	return trackerGET("/v3/queues/"+key+"/fields", body)
}

func localFieldsAnswer(key, body string) faketracker.Exchange {
	return trackerGET("/v3/queues/"+key+"/localFields", body)
}

func globalFieldsAnswer(body string) faketracker.Exchange {
	return trackerGET("/v3/fields", body)
}

func TestQueueContext(t *testing.T) {
	t.Parallel()

	queue := queueAnswer("APP", appQueueAnswer)
	w200 := workflowAnswer("W200", w200Answer)
	components := componentsAnswer("APP", appComponentsAnswer)
	queueFields := queueFieldsAnswer("APP", appQueueFieldsAnswer)
	localFields := localFieldsAnswer("APP", appLocalFieldsAnswer)
	globalFields := globalFieldsAnswer(appGlobalFieldsAnswer)
	context := func(extra ...string) []string { return slices.Concat([]string{"queue", "context"}, extra) }

	runLeafRows(t, []leafRow{
		{
			// W200 is fetched once, though two issue types follow it.
			name: "Parts in order", args: context("APP"),
			exchanges: []faketracker.Exchange{queue, w200, components, queueFields, localFields, globalFields},
			stdout:    appContextJSON + "\n",
		},
		{
			name: "Document keeps <, > and & literal", args: context("APP"),
			exchanges: []faketracker.Exchange{
				queue, w200, componentsAnswer("APP", `[{"id": 55, "name": "R&D <core>"}]`),
				queueFields, localFields, globalFields,
			},
			holds: []string{`"components":[{"id":"55","name":"R&D <core>"}]`},
		},
		{
			name: "Whole document through jq", args: context("APP", "--jq", "."),
			exchanges: []faketracker.Exchange{queue, w200, components, queueFields, localFields, globalFields},
			json:      appContextJSON,
		},
		{
			name:      "Queue parts and global fields",
			args:      context("APP", "--json", "key,name,defaultType,defaultPriority,globalFields,incomplete"),
			exchanges: []faketracker.Exchange{queue, globalFields},
			json: `{"key": "APP", "name": "Demo application", "defaultType": "task", "defaultPriority": "normal",
				"globalFields": [{"key": "tags", "name": "Теги"}], "incomplete": []}`,
		},
		{
			name: "Every workflow of the queue, in order", args: context("APP", "--json", "statuses,workflows"),
			exchanges: []faketracker.Exchange{
				queueAnswer("APP", twoWorkflowQueueAnswer), w200, workflowAnswer("W100", w100Answer),
			},
			json: `{"statuses": [{"key": "open", "name": "Открыт"}, {"key": "closed", "name": "Закрыт"},
					{"key": "new", "name": "Новый"}],
				"workflows": [` + w200JSON + `,
					{"id": "W100", "initialStatus": "new", "transitions": {"new": ["open"], "open": ["closed"],
						"closed": []}}],
				"incomplete": []}`,
			check: assertRequestPaths("/v3/queues/APP", "/v3/workflows/W200", "/v3/workflows/W100"),
		},
		{
			name: "Required fields", args: context("OPS", "--json", "requiredFields"),
			exchanges: []faketracker.Exchange{
				queueAnswer(
					"OPS",
					`{"key": "OPS", "defaultType": {"key": "task"}, "defaultPriority": {"key": "normal"}}`,
				),
				queueFieldsAnswer("OPS", `[{"id": "type", "schema": {"required": true}},
					{"id": "priority", "schema": {"required": true}}, {"id": "createdBy", "schema": {"required": true}},
					{"id": "start", "schema": {"required": false}}, {"id": "followers", "schema": {"type": "array"}}]`),
			},
			json: `{"requiredFields": [{"id": "summary"}, {"id": "type", "default": "task"},
				{"id": "priority", "default": "normal"}, {"id": "createdBy"}], "incomplete": []}`,
		},
		{
			name:      "Statuses",
			args:      context("APP", "--json", "statuses"),
			exchanges: []faketracker.Exchange{queue, w200},
			json:      `{"statuses": ` + appStatusesJSON + `, "incomplete": []}`,
		},
		{
			name: "Local fields by queue ID", args: context("140", "--json", "localFields"),
			exchanges: []faketracker.Exchange{queueAnswer("140", appQueueAnswer), localFieldsAnswer("140", `[{
				"id": "5d0e4f1a2b3c4d5e6f708192--stand", "key": "stand", "name": "Среда", "schema": {"type": "string"},
				"optionsProvider": {"values": {"APP": ["Test", "Beta"], "DIRECT": ["Production"]},
					"defaults": ["Not specified"]}}]`)},
			json: `{"localFields": [{"id": "5d0e4f1a2b3c4d5e6f708192--stand", "key": "stand", "name": "Среда",
				"schema": "string", "readonly": false, "options": ["Test", "Beta"]}], "incomplete": []}`,
		},
		{
			name: "Issue types and components", args: context("APP", "--json", "issueTypes,components"),
			exchanges: []faketracker.Exchange{queue, components},
			json: `{"issueTypes": ` + appIssueTypesJSON + `, "components": [{"id": "55", "name": "Hotfix"}],
				"incomplete": []}`,
		},
		{
			name: "Selection in any case", args: context("APP", "--json", "IssueTypes,COMPONENTS"),
			exchanges: []faketracker.Exchange{queue, components},
			json: `{"issueTypes": ` + appIssueTypesJSON + `, "components": [{"id": "55", "name": "Hotfix"}],
				"incomplete": []}`,
		},
		{
			name: "jq", args: context("APP", "--jq", ".localFields[].id"),
			exchanges: []faketracker.Exchange{queue, w200, components, queueFields, localFields, globalFields},
			stdout:    "5d0e4f1a2b3c4d5e6f708192--size\n",
		},
		{
			name: "Queue fields forbidden", args: context("APP", "--json", "requiredFields,localFields"),
			exchanges: []faketracker.Exchange{
				queue, localFields,
				trackerError(http.MethodGet, "/v3/queues/APP/fields", http.StatusForbidden,
					"У вас недостаточно прав в очереди APP."),
			},
			json: `{"requiredFields": null, "localFields": ` + appLocalFieldsJSON + `,
				"incomplete": [{"part": "requiredFields", "reason": "У вас недостаточно прав в очереди APP."}]}`,
		},
		{
			name: "Queue fields empty", args: context("APP", "--json", "requiredFields"),
			exchanges: []faketracker.Exchange{queue, queueFieldsAnswer("APP", `[]`)},
			json: `{"requiredFields": [{"id": "summary"}], "incomplete": [{"part": "requiredFields",
				"reason": "Tracker listed no queue fields, so other fields may be required"}]}`,
		},
		{
			name: "Workflow fails", args: context("APP"),
			exchanges: []faketracker.Exchange{
				queueAnswer("APP", twoWorkflowQueueAnswer), w200, components, queueFields, localFields, globalFields,
				trackerNotFoundOn(http.MethodGet, "/v3/workflows/W100", "Workflow W100 not found"),
			},
			json: `{"key": "APP", "name": "Demo application", "defaultType": "", "defaultPriority": "",
				"issueTypes": [{"key": "task", "name": "Задача", "workflow": "W200"},
					{"key": "milestone", "name": "Веха", "workflow": "W100"}],
				"statuses": null, "workflows": null, "components": [{"id": "55", "name": "Hotfix"}],
				"requiredFields": [{"id": "summary"}, {"id": "type"}], "localFields": ` + appLocalFieldsJSON + `,
				"globalFields": [{"key": "tags", "name": "Теги"}],
				"incomplete": [{"part": "statuses", "reason": "workflow W100: Workflow W100 not found"},
					{"part": "workflows", "reason": "workflow W100: Workflow W100 not found"}]}`,
		},
		{
			name: "Workflow fails under a selection", args: context("APP", "--json", "workflows,components"),
			exchanges: []faketracker.Exchange{
				queueAnswer("APP", twoWorkflowQueueAnswer), w200, components,
				trackerNotFoundOn(http.MethodGet, "/v3/workflows/W100", "Workflow W100 not found"),
			},
			json: `{"workflows": null, "components": [{"id": "55", "name": "Hotfix"}],
				"incomplete": [{"part": "workflows", "reason": "workflow W100: Workflow W100 not found"}]}`,
		},
		{
			name: "Failed parts under a selection",
			args: context("APP", "--json", "statuses,components,requiredFields"),
			exchanges: []faketracker.Exchange{
				queueAnswer("APP", twoWorkflowQueueAnswer), w200, queueFields,
				trackerNotFoundOn(http.MethodGet, "/v3/workflows/W100", "Workflow W100 not found"),
				withQuery(trackerError(http.MethodGet, "/v3/queues/APP/components", http.StatusForbidden,
					"components denied"), url.Values{"fields": {"name"}}),
			},
			json: `{"statuses": null, "components": null, "requiredFields": [{"id": "summary"}, {"id": "type"}],
				"incomplete": [{"part": "statuses", "reason": "workflow W100: Workflow W100 not found"},
					{"part": "components", "reason": "components denied"}]}`,
		},
		{
			name: "Several parts fail", args: context("APP"),
			exchanges: []faketracker.Exchange{
				queue, w200, queueFields,
				withQuery(trackerError(http.MethodGet, "/v3/queues/APP/components", http.StatusForbidden,
					"components denied"), url.Values{"fields": {"name"}}),
				trackerNotFoundOn(http.MethodGet, "/v3/queues/APP/localFields", "local fields not found"),
				trackerError(http.MethodGet, "/v3/fields", http.StatusInternalServerError, "global fields unavailable"),
			},
			json: `{"key": "APP", "name": "Demo application", "defaultType": "task", "defaultPriority": "normal",
				"issueTypes": ` + appIssueTypesJSON + `, "statuses": ` + appStatusesJSON + `,
				"workflows": [` + w200JSON + `], "components": null,
				"requiredFields": [{"id": "summary"}, {"id": "type", "default": "task"}],
				"localFields": null, "globalFields": null,
				"incomplete": [{"part": "components", "reason": "components denied"},
					{"part": "localFields", "reason": "local fields not found"},
					{"part": "globalFields", "reason": "global fields unavailable"}]}`,
		},
		{
			name: "Quiet", args: context("APP", "--quiet"), code: ytrerrors.ExitUserError,
			stderr: []string{
				"Error: queue context does not support --quiet: its document is always JSON\n" +
					"Run without --quiet: ytr queue context APP\n",
			},
		},
		named("Unknown queue", failureRow(
			withQuery(trackerNotFoundOn(http.MethodGet, "/v3/queues/NOPE", "Очередь не существует."),
				url.Values{"expand": {"issueTypesConfig"}}),
			context("NOPE", "--json", "key")...)),
		{
			name: "Unknown queue, whole document", args: context("NOPE"),
			exchanges: []faketracker.Exchange{withQuery(
				trackerNotFoundOn(http.MethodGet, "/v3/queues/NOPE", "Очередь не существует."),
				url.Values{"expand": {"issueTypesConfig"}})},
			code: ytrerrors.ExitNotFound, stderr: []string{"Error: Очередь не существует.\n"},
		},
	})
}

// assertRequestPaths wants the run's requests to go to these paths, in order.
func assertRequestPaths(paths ...string) func(*testing.T, cliResult) {
	return func(t *testing.T, res cliResult) {
		t.Helper()

		var got []string
		for _, req := range res.Requests {
			got = append(got, req.Path)
		}
		if !slices.Equal(got, paths) {
			t.Errorf("request paths = %q, want %q", got, paths)
		}
	}
}
