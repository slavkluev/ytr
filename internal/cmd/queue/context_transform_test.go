package queue

import (
	"encoding/json"
	"net/http"
	"reflect"
	"slices"
	"strings"
	"testing"

	"github.com/slavkluev/go-yandex-tracker/tracker"
)

// The builders follow the recorded read-only responses of the Tracker API for
// the APP queue, cut down to the fields the document reads.

var (
	statusNew    = &tracker.Status{Key: new("new"), Display: new("Новый")}
	statusOpen   = &tracker.Status{Key: new("open"), Display: new("Открыт")}
	statusClosed = &tracker.Status{Key: new("closed"), Display: new("Закрыт")}
)

func issueTypeConfig(key, name, workflowID string) *tracker.QueueIssueTypeConfig {
	return &tracker.QueueIssueTypeConfig{
		IssueType: &tracker.IssueType{Key: new(key), Display: new(name)},
		Workflow:  &tracker.Workflow{ID: new(tracker.FlexString(workflowID))},
	}
}

// step leads from status to each target through an action of its own.
func step(status *tracker.Status, targets ...*tracker.Status) *tracker.WorkflowStep {
	s := &tracker.WorkflowStep{Status: status}
	for _, target := range targets {
		s.Actions = append(s.Actions, &tracker.WorkflowAction{Target: target})
	}
	return s
}

func workflow(id string, initial *tracker.Status, steps ...*tracker.WorkflowStep) *tracker.Workflow {
	return &tracker.Workflow{
		ID:            new(tracker.FlexString(id)),
		InitialAction: &tracker.WorkflowAction{Target: initial},
		Steps:         steps,
	}
}

func appQueue() *tracker.Queue {
	return &tracker.Queue{
		Key:             new("APP"),
		Name:            new("Demo application"),
		DefaultType:     &tracker.IssueType{Key: new("task")},
		DefaultPriority: &tracker.Priority{Key: new("normal")},
		IssueTypesConfig: []*tracker.QueueIssueTypeConfig{
			issueTypeConfig("task", "Задача", "W200"),
			issueTypeConfig("bug", "Ошибка", "W200"),
		},
	}
}

// twoWorkflowQueue is APP without defaults, with one issue type on W200 and
// one on W100.
func twoWorkflowQueue() *tracker.Queue {
	return &tracker.Queue{
		Key:  new("APP"),
		Name: new("Demo application"),
		IssueTypesConfig: []*tracker.QueueIssueTypeConfig{
			issueTypeConfig("task", "Задача", "W200"),
			issueTypeConfig("milestone", "Веха", "W100"),
		},
	}
}

// w200 has a terminal step without actions, and two actions of the open step
// that lead to the same status.
func w200() *tracker.Workflow {
	return workflow("W200", statusOpen,
		step(statusOpen, statusClosed, statusClosed),
		step(statusClosed),
	)
}

// w100 starts in its own status and then shares open and closed with W200.
func w100() *tracker.Workflow {
	return workflow("W100", statusNew,
		step(statusNew, statusOpen),
		step(statusOpen, statusClosed),
		step(statusClosed),
	)
}

func appComponents() []*tracker.Component {
	return []*tracker.Component{{ID: new(tracker.FlexString("55")), Name: new("Hotfix")}}
}

// appQueueFields pairs a required system field with a field that is not
// required.
func appQueueFields() []*tracker.Field {
	return []*tracker.Field{
		{ID: new(tracker.FlexString("type")), Key: new("type"), Schema: &tracker.FieldSchema{Required: new(true)}},
		{ID: new(tracker.FlexString("stand")), Key: new("stand"), Schema: &tracker.FieldSchema{Required: new(false)}},
	}
}

func appLocalFields() []*tracker.Field {
	return []*tracker.Field{{
		ID:              new(tracker.FlexString("5d0e4f1a2b3c4d5e6f708192--size")),
		Key:             new("size"),
		Name:            new("Размер"),
		Schema:          &tracker.FieldSchema{Type: new("string")},
		OptionsProvider: &tracker.OptionsProvider{Values: []any{"S", "M", "L"}},
	}}
}

// appGlobalFields holds an editable field and a read-only one.
func appGlobalFields() []*tracker.Field {
	return []*tracker.Field{
		{Key: new("tags"), Name: new("Теги")},
		{Key: new("key"), Name: new("Ключ"), Readonly: new(true)},
	}
}

func appResults() contextResults {
	return contextResults{
		workflows:    []*tracker.Workflow{w200()},
		components:   appComponents(),
		queueFields:  appQueueFields(),
		localFields:  appLocalFields(),
		globalFields: appGlobalFields(),
	}
}

// newAPIError returns the error the library returns for a failed request.
func newAPIError(status int, message string) error {
	return &tracker.ErrorResponse{
		Response: &http.Response{
			StatusCode: status,
			Request:    &http.Request{Method: http.MethodGet},
		},
		ErrorMessages: []string{message},
	}
}

func assertJSONEqual(t *testing.T, got, want string) {
	t.Helper()
	var gotValue, wantValue any
	if err := json.Unmarshal([]byte(got), &gotValue); err != nil {
		t.Fatalf("output is not JSON: %v\n%s", err, got)
	}
	if err := json.Unmarshal([]byte(want), &wantValue); err != nil {
		t.Fatalf("expected value is not JSON: %v\n%s", err, want)
	}
	if !reflect.DeepEqual(gotValue, wantValue) {
		t.Errorf("document mismatch\ngot:\n%s\nwant:\n%s", got, want)
	}
}

func marshalDocument(t *testing.T, doc any) string {
	t.Helper()
	data, err := json.Marshal(doc)
	if err != nil {
		t.Fatalf("marshal document: %v", err)
	}
	return string(data)
}

// topLevelKeys reads an object's keys back in the order they were encoded,
// which a comparison by value cannot see.
func topLevelKeys(t *testing.T, doc string) []string {
	t.Helper()
	dec := json.NewDecoder(strings.NewReader(doc))
	if _, err := dec.Token(); err != nil {
		t.Fatalf("document is not a JSON object: %v\n%s", err, doc)
	}
	var keys []string
	for dec.More() {
		tok, err := dec.Token()
		if err != nil {
			t.Fatalf("reading a key failed: %v\n%s", err, doc)
		}
		key, ok := tok.(string)
		if !ok {
			t.Fatalf("expected a key, got %v\n%s", tok, doc)
		}
		keys = append(keys, key)
		var value json.RawMessage
		if err := dec.Decode(&value); err != nil {
			t.Fatalf("reading %q failed: %v\n%s", key, err, doc)
		}
	}
	return keys
}

const appFullDocument = `{
  "key": "APP",
  "name": "Demo application",
  "defaultType": "task",
  "defaultPriority": "normal",
  "issueTypes": [
    {"key": "task", "name": "Задача", "workflow": "W200"},
    {"key": "bug", "name": "Ошибка", "workflow": "W200"}
  ],
  "statuses": [{"key": "open", "name": "Открыт"}, {"key": "closed", "name": "Закрыт"}],
  "workflows": [{"id": "W200", "initialStatus": "open", "transitions": {"open": ["closed"], "closed": []}}],
  "components": [{"id": "55", "name": "Hotfix"}],
  "requiredFields": [{"id": "summary"}, {"id": "type", "default": "task"}],
  "localFields": [
    {"id": "5d0e4f1a2b3c4d5e6f708192--size", "key": "size", "name": "Размер", "schema": "string",
     "readonly": false, "options": ["S", "M", "L"]}
  ],
  "globalFields": [{"key": "tags", "name": "Теги"}],
  "incomplete": []
}`

func TestContextDocumentFull(t *testing.T) {
	doc := marshalDocument(t, contextDocument(appQueue(), "APP", appResults(), nil))

	assertJSONEqual(t, doc, appFullDocument)
	if got := topLevelKeys(t, doc); !slices.Equal(got, QueueContextFields) {
		t.Errorf("parts = %q, want %q", got, QueueContextFields)
	}
}

func TestContextDocumentSelection(t *testing.T) {
	doc := contextDocument(appQueue(), "APP", appResults(), []string{"issueTypes", "components"})

	assertJSONEqual(t, marshalDocument(t, doc), `{
	  "issueTypes": [
	    {"key": "task", "name": "Задача", "workflow": "W200"},
	    {"key": "bug", "name": "Ошибка", "workflow": "W200"}
	  ],
	  "components": [{"id": "55", "name": "Hotfix"}],
	  "incomplete": []
	}`)
}

func TestContextDocumentRequiredFieldsATS(t *testing.T) {
	ops := &tracker.Queue{
		Key:             new("OPS"),
		DefaultType:     &tracker.IssueType{Key: new("task")},
		DefaultPriority: &tracker.Priority{Key: new("normal")},
	}
	required := func(id string, isRequired bool) *tracker.Field {
		return &tracker.Field{ID: new(tracker.FlexString(id)), Schema: &tracker.FieldSchema{Required: new(isRequired)}}
	}
	results := contextResults{queueFields: []*tracker.Field{
		required("type", true),
		required("priority", true),
		required("createdBy", true),
		required("start", false),
		{ID: new(tracker.FlexString("followers")), Schema: &tracker.FieldSchema{Type: new("array")}},
	}}

	doc := contextDocument(ops, "OPS", results, []string{"requiredFields"})

	assertJSONEqual(t, marshalDocument(t, doc), `{
	  "requiredFields": [
	    {"id": "summary"},
	    {"id": "type", "default": "task"},
	    {"id": "priority", "default": "normal"},
	    {"id": "createdBy"}
	  ],
	  "incomplete": []
	}`)
}

func TestContextDocumentQueueFieldsEmpty(t *testing.T) {
	results := appResults()
	results.queueFields = []*tracker.Field{}

	doc := contextDocument(appQueue(), "APP", results, []string{"requiredFields"})

	assertJSONEqual(t, marshalDocument(t, doc), `{
	  "requiredFields": [{"id": "summary"}],
	  "incomplete": [
	    {"part": "requiredFields", "reason": "Tracker listed no queue fields, so other fields may be required"}
	  ]
	}`)
}

func TestContextDocumentQueueFieldsForbidden(t *testing.T) {
	restricted := &tracker.Queue{
		Key:              new("RESTRICTED"),
		Name:             new("Закрытая"),
		DefaultType:      &tracker.IssueType{Key: new("bug")},
		DefaultPriority:  &tracker.Priority{Key: new("normal")},
		IssueTypesConfig: []*tracker.QueueIssueTypeConfig{issueTypeConfig("milestone", "Веха", "W100")},
	}
	results := appResults()
	results.workflows = []*tracker.Workflow{workflow("W100", statusOpen,
		step(statusOpen, statusClosed),
		step(statusClosed),
	)}
	results.components = []*tracker.Component{}
	results.localFields = []*tracker.Field{}
	results.queueFields = nil
	results.queueFieldsErr = newAPIError(http.StatusForbidden, "У вас недостаточно прав в очереди RESTRICTED.")

	doc := contextDocument(restricted, "RESTRICTED", results, nil)

	// A failed part is null, unlike the fetched but empty ones, which stay [].
	assertJSONEqual(t, marshalDocument(t, doc), `{
	  "key": "RESTRICTED",
	  "name": "Закрытая",
	  "defaultType": "bug",
	  "defaultPriority": "normal",
	  "issueTypes": [{"key": "milestone", "name": "Веха", "workflow": "W100"}],
	  "statuses": [{"key": "open", "name": "Открыт"}, {"key": "closed", "name": "Закрыт"}],
	  "workflows": [{"id": "W100", "initialStatus": "open", "transitions": {"open": ["closed"], "closed": []}}],
	  "components": [],
	  "requiredFields": null,
	  "localFields": [],
	  "globalFields": [{"key": "tags", "name": "Теги"}],
	  "incomplete": [{"part": "requiredFields", "reason": "У вас недостаточно прав в очереди RESTRICTED."}]
	}`)
}

func failedW100Results() contextResults {
	results := appResults()
	results.workflows = nil
	results.failedWorkflow = "W100"
	results.workflowsErr = newAPIError(http.StatusNotFound, "Workflow W100 not found")
	return results
}

func TestContextDocumentWorkflowFails(t *testing.T) {
	doc := contextDocument(twoWorkflowQueue(), "APP", failedW100Results(), nil)

	assertJSONEqual(t, marshalDocument(t, doc), `{
	  "key": "APP",
	  "name": "Demo application",
	  "defaultType": "",
	  "defaultPriority": "",
	  "issueTypes": [
	    {"key": "task", "name": "Задача", "workflow": "W200"},
	    {"key": "milestone", "name": "Веха", "workflow": "W100"}
	  ],
	  "statuses": null,
	  "workflows": null,
	  "components": [{"id": "55", "name": "Hotfix"}],
	  "requiredFields": [{"id": "summary"}, {"id": "type"}],
	  "localFields": [
	    {"id": "5d0e4f1a2b3c4d5e6f708192--size", "key": "size", "name": "Размер", "schema": "string",
	     "readonly": false, "options": ["S", "M", "L"]}
	  ],
	  "globalFields": [{"key": "tags", "name": "Теги"}],
	  "incomplete": [
	    {"part": "statuses", "reason": "workflow W100: Workflow W100 not found"},
	    {"part": "workflows", "reason": "workflow W100: Workflow W100 not found"}
	  ]
	}`)
}

func TestContextDocumentWorkflowFailsUnderSelection(t *testing.T) {
	doc := contextDocument(twoWorkflowQueue(), "APP", failedW100Results(), []string{"workflows"})

	// statuses was not selected, so incomplete does not name it.
	assertJSONEqual(t, marshalDocument(t, doc), `{
	  "workflows": null,
	  "incomplete": [{"part": "workflows", "reason": "workflow W100: Workflow W100 not found"}]
	}`)
}

func TestContextDocumentStatusesOnly(t *testing.T) {
	doc := contextDocument(appQueue(), "APP", appResults(), []string{"statuses"})

	assertJSONEqual(t, marshalDocument(t, doc), `{
	  "statuses": [{"key": "open", "name": "Открыт"}, {"key": "closed", "name": "Закрыт"}],
	  "incomplete": []
	}`)
}

func TestContextDocumentStatusesAcrossWorkflows(t *testing.T) {
	results := appResults()
	results.workflows = []*tracker.Workflow{w200(), w100()}

	doc := contextDocument(twoWorkflowQueue(), "APP", results, []string{"statuses", "workflows"})

	assertJSONEqual(t, marshalDocument(t, doc), `{
	  "statuses": [
	    {"key": "open", "name": "Открыт"},
	    {"key": "closed", "name": "Закрыт"},
	    {"key": "new", "name": "Новый"}
	  ],
	  "workflows": [
	    {"id": "W200", "initialStatus": "open", "transitions": {"open": ["closed"], "closed": []}},
	    {"id": "W100", "initialStatus": "new", "transitions": {"new": ["open"], "open": ["closed"], "closed": []}}
	  ],
	  "incomplete": []
	}`)
}

func TestContextDocumentTerminalStep(t *testing.T) {
	doc := marshalDocument(t, contextDocument(appQueue(), "APP", appResults(), []string{"workflows"}))

	assertJSONEqual(t, doc, `{
	  "workflows": [{"id": "W200", "initialStatus": "open", "transitions": {"open": ["closed"], "closed": []}}],
	  "incomplete": []
	}`)
	if !strings.Contains(doc, `"closed":[]`) {
		t.Errorf("a step without actions must map to [], got:\n%s", doc)
	}
	// Steps keep the workflow's order rather than sorting.
	if strings.Index(doc, `"open":[`) > strings.Index(doc, `"closed":[]`) {
		t.Errorf("transitions are not in step order:\n%s", doc)
	}
}

func TestContextDocumentOptionTypes(t *testing.T) {
	results := appResults()
	results.localFields = []*tracker.Field{
		{
			ID: new(tracker.FlexString("5d0e4f1a2b3c4d5e6f708192--flag")), Key: new("flag"), Name: new("Фича"),
			Schema:          &tracker.FieldSchema{Type: new("integer")},
			OptionsProvider: &tracker.OptionsProvider{Values: []any{json.Number("0"), json.Number("1")}},
		},
		{
			ID: new(tracker.FlexString("5d0e4f1a2b3c4d5e6f708192--stand")), Key: new("stand"), Name: new("Среда"),
			Schema: &tracker.FieldSchema{Type: new("string")},
			OptionsProvider: &tracker.OptionsProvider{
				QueueValues: map[string][]any{"APP": {"Test", "Beta"}, "DIRECT": {"Production"}},
				Defaults:    []any{"Not specified"},
			},
		},
		{
			ID: new(tracker.FlexString("5d0e4f1a2b3c4d5e6f708192--bench")), Key: new("bench"), Name: new("База"),
			Schema:   &tracker.FieldSchema{Type: new("string")},
			Readonly: new(true),
			OptionsProvider: &tracker.OptionsProvider{
				QueueValues: map[string][]any{"DIRECT": {"Production"}},
				Defaults:    []any{"Not specified", "Trunk"},
			},
		},
	}

	doc := contextDocument(appQueue(), "APP", results, []string{"localFields"})

	assertJSONEqual(t, marshalDocument(t, doc), `{
	  "localFields": [
	    {"id": "5d0e4f1a2b3c4d5e6f708192--flag", "key": "flag", "name": "Фича", "schema": "integer",
	     "readonly": false, "options": [0, 1]},
	    {"id": "5d0e4f1a2b3c4d5e6f708192--stand", "key": "stand", "name": "Среда", "schema": "string",
	     "readonly": false, "options": ["Test", "Beta"]},
	    {"id": "5d0e4f1a2b3c4d5e6f708192--bench", "key": "bench", "name": "База", "schema": "string",
	     "readonly": true, "options": ["Not specified", "Trunk"]}
	  ],
	  "incomplete": []
	}`)
}

// Tracker accepts the queue id in place of its key, and the queue it returns
// carries the key that the per-queue option lists use.
func TestContextDocumentOptionsByQueueID(t *testing.T) {
	results := appResults()
	results.localFields = []*tracker.Field{{
		ID: new(tracker.FlexString("5d0e4f1a2b3c4d5e6f708192--stand")), Key: new("stand"), Name: new("Среда"),
		Schema: &tracker.FieldSchema{Type: new("string")},
		OptionsProvider: &tracker.OptionsProvider{
			QueueValues: map[string][]any{"APP": {"Test", "Beta"}, "DIRECT": {"Production"}},
			Defaults:    []any{"Not specified"},
		},
	}}

	doc := contextDocument(appQueue(), "140", results, []string{"localFields"})

	assertJSONEqual(t, marshalDocument(t, doc), `{
	  "localFields": [
	    {"id": "5d0e4f1a2b3c4d5e6f708192--stand", "key": "stand", "name": "Среда", "schema": "string",
	     "readonly": false, "options": ["Test", "Beta"]}
	  ],
	  "incomplete": []
	}`)
}

func TestContextDocumentSeveralPartsFail(t *testing.T) {
	results := appResults()
	results.components, results.componentsErr = nil, newAPIError(http.StatusForbidden, "components denied")
	results.localFields, results.localFieldsErr = nil, newAPIError(http.StatusNotFound, "local fields not found")
	results.globalFields, results.globalErr = nil, newAPIError(
		http.StatusInternalServerError, "global fields unavailable",
	)

	doc := contextDocument(appQueue(), "APP", results, nil)

	assertJSONEqual(t, marshalDocument(t, doc), `{
	  "key": "APP",
	  "name": "Demo application",
	  "defaultType": "task",
	  "defaultPriority": "normal",
	  "issueTypes": [
	    {"key": "task", "name": "Задача", "workflow": "W200"},
	    {"key": "bug", "name": "Ошибка", "workflow": "W200"}
	  ],
	  "statuses": [{"key": "open", "name": "Открыт"}, {"key": "closed", "name": "Закрыт"}],
	  "workflows": [{"id": "W200", "initialStatus": "open", "transitions": {"open": ["closed"], "closed": []}}],
	  "components": null,
	  "requiredFields": [{"id": "summary"}, {"id": "type", "default": "task"}],
	  "localFields": null,
	  "globalFields": null,
	  "incomplete": [
	    {"part": "components", "reason": "components denied"},
	    {"part": "localFields", "reason": "local fields not found"},
	    {"part": "globalFields", "reason": "global fields unavailable"}
	  ]
	}`)
}
