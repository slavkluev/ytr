package queue

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"reflect"
	"slices"
	"strings"
	"sync"
	"testing"

	"github.com/slavkluev/go-yandex-tracker/tracker"

	"github.com/slavkluev/ytr/internal/cmd/jsonfields"
	"github.com/slavkluev/ytr/internal/config"
	ytrerrors "github.com/slavkluev/ytr/internal/errors"
	"github.com/slavkluev/ytr/internal/output"
	"github.com/slavkluev/ytr/internal/testutil"
)

// Fixtures follow the reference examples and the recorded read-only responses
// of the Tracker API: queue with expand=issueTypesConfig, /workflows/{id},
// /queues/{q}/components?fields=name, /queues/{q}/fields,
// /queues/{q}/localFields and /fields. They are decoded by the library, so the
// tests see what the command sees at run time.

const appQueueJSON = `{
  "self": "https://api.tracker.yandex.net/v3/queues/APP",
  "id": 140,
  "key": "APP",
  "version": 5,
  "name": "Demo application",
  "defaultType": {"self": "https://api.tracker.yandex.net/v3/issuetypes/2", "id": "2", "key": "task", "display": "Задача"},
  "defaultPriority": {"self": "https://api.tracker.yandex.net/v3/priorities/3", "id": "3", "key": "normal", "display": "Средний"},
  "issueTypesConfig": [
    {"issueType": {"self": "https://api.tracker.yandex.net/v3/issuetypes/2", "id": "2", "key": "task", "display": "Задача"},
     "workflow": {"self": "https://api.tracker.yandex.net/v3/workflows/W200", "id": "W200", "display": "W200"},
     "resolutions": [{"self": "https://api.tracker.yandex.net/v3/resolutions/1", "id": "1", "key": "fixed", "display": "Решен"}]},
    {"issueType": {"self": "https://api.tracker.yandex.net/v3/issuetypes/1", "id": "1", "key": "bug", "display": "Ошибка"},
     "workflow": {"self": "https://api.tracker.yandex.net/v3/workflows/W200", "id": "W200", "display": "W200"}}
  ]
}`

// w200JSON has a terminal step without actions, and two actions of the open
// step that lead to the same status.
const w200JSON = `{
  "self": "https://api.tracker.yandex.net/v3/workflows/W200",
  "id": "W200",
  "name": "W200",
  "version": 1,
  "steps": [
    {"status": {"self": "https://api.tracker.yandex.net/v3/statuses/1", "id": "1", "key": "open", "display": "Открыт"},
     "statusType": "new",
     "actions": [
       {"id": "close", "shortId": "close", "name": "Закрыть",
        "target": {"self": "https://api.tracker.yandex.net/v3/statuses/3", "id": "3", "key": "closed", "display": "Закрыт"}},
       {"id": "resolve", "shortId": "resolve", "name": "Решить",
        "target": {"self": "https://api.tracker.yandex.net/v3/statuses/3", "id": "3", "key": "closed", "display": "Закрыт"}}
     ]},
    {"status": {"self": "https://api.tracker.yandex.net/v3/statuses/3", "id": "3", "key": "closed", "display": "Закрыт"},
     "statusType": "done"}
  ],
  "initialAction": {"id": "open", "name": "Open",
    "target": {"self": "https://api.tracker.yandex.net/v3/statuses/1", "id": "1", "key": "open", "display": "Открыт"}},
  "queue": {"self": "https://api.tracker.yandex.net/v3/queues/APP", "id": "140", "key": "APP", "display": "Demo application"},
  "created": "2026-08-11T14:37:06.356+0000",
  "updated": "2026-08-11T14:37:06.356+0000",
  "deleted": false,
  "type": "visual"
}`

const appComponentsJSON = `[
  {"self": "https://api.tracker.yandex.net/v3/components/55", "id": 55, "name": "Hotfix"}
]`

// appQueueFieldsJSON pairs a required system field with a field that is not
// required and has per-queue values.
const appQueueFieldsJSON = `[
  {"self": "https://api.tracker.yandex.net/v3/fields/type", "id": "type", "name": "Тип", "key": "type", "version": 0,
   "schema": {"type": "issuetype", "required": true}, "readonly": false, "options": true, "suggest": true,
   "optionsProvider": {"type": "IssueTypeOptionsProvider"}, "order": 2, "type": "standard"},
  {"self": "https://api.tracker.yandex.net/v3/fields/stand", "id": "stand", "name": "Board", "version": 1361890459119,
   "schema": {"type": "string", "required": false}, "readonly": false, "options": true, "suggest": false,
   "optionsProvider": {"type": "QueueFixedListOptionsProvider",
     "values": {"DIRECT": ["Not specified", "Test"]}, "defaults": ["Not specified", "Test"]},
   "order": 222}
]`

const appLocalFieldsJSON = `[
  {"self": "https://api.tracker.yandex.net/v3/queues/APP/localFields/size",
   "id": "5d0e4f1a2b3c4d5e6f708192--size", "name": "Размер", "key": "size", "version": 1,
   "schema": {"type": "string", "required": false}, "readonly": false, "options": false, "suggest": false,
   "optionsProvider": {"type": "FixedListOptionsProvider", "needValidation": true, "values": ["S", "M", "L"]},
   "queryProvider": {"type": "StringOptionalQueryProvider"}, "order": 3,
   "queue": {"self": "https://api.tracker.yandex.net/v3/queues/APP", "id": "140", "key": "APP", "display": "Demo application"},
   "type": "local"}
]`

// globalFieldsJSON holds an editable field and a read-only one.
const globalFieldsJSON = `[
  {"self": "https://api.tracker.yandex.net/v3/fields/tags", "id": "tags", "name": "Теги", "key": "tags", "version": 0,
   "schema": {"type": "array", "items": "string"}, "readonly": false, "options": false, "suggest": true, "order": 30,
   "type": "standard"},
  {"self": "https://api.tracker.yandex.net/v3/fields/key", "id": "key", "name": "Ключ", "key": "key", "version": 0,
   "schema": {"type": "string"}, "readonly": true, "options": false, "suggest": false, "order": 1,
   "type": "standard"}
]`

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

// w100JSON starts in its own status and then shares open and closed with
// W200.
const w100JSON = `{
  "self": "https://api.tracker.yandex.net/v3/workflows/W100",
  "id": "W100",
  "name": "W100",
  "version": 1,
  "steps": [
    {"status": {"self": "https://api.tracker.yandex.net/v3/statuses/5", "id": "5", "key": "new", "display": "Новый"},
     "actions": [
       {"id": "open", "name": "Открыть",
        "target": {"self": "https://api.tracker.yandex.net/v3/statuses/1", "id": "1", "key": "open", "display": "Открыт"}}
     ]},
    {"status": {"self": "https://api.tracker.yandex.net/v3/statuses/1", "id": "1", "key": "open", "display": "Открыт"},
     "actions": [
       {"id": "close", "name": "Закрыть",
        "target": {"self": "https://api.tracker.yandex.net/v3/statuses/3", "id": "3", "key": "closed", "display": "Закрыт"}}
     ]},
    {"status": {"self": "https://api.tracker.yandex.net/v3/statuses/3", "id": "3", "key": "closed", "display": "Закрыт"}}
  ],
  "initialAction": {"id": "new", "name": "New",
    "target": {"self": "https://api.tracker.yandex.net/v3/statuses/5", "id": "5", "key": "new", "display": "Новый"}}
}`

// twoWorkflowQueueJSON is APP with one issue type on W200 and one on W100.
const twoWorkflowQueueJSON = `{
  "self": "https://api.tracker.yandex.net/v3/queues/APP", "id": 140, "key": "APP", "name": "Demo application",
  "issueTypesConfig": [
    {"issueType": {"self": "https://api.tracker.yandex.net/v3/issuetypes/2", "id": "2", "key": "task", "display": "Задача"},
     "workflow": {"self": "https://api.tracker.yandex.net/v3/workflows/W200", "id": "W200", "display": "W200"}},
    {"issueType": {"self": "https://api.tracker.yandex.net/v3/issuetypes/21", "id": "21", "key": "milestone", "display": "Веха"},
     "workflow": {"self": "https://api.tracker.yandex.net/v3/workflows/W100", "id": "W100", "display": "W100"}}
  ]
}`

// mockContextClient implements queueContextClient. The command calls it from
// several goroutines, so it records calls under a mutex; its fixtures are
// only read.
type mockContextClient struct {
	mu    sync.Mutex
	calls []string

	queue           *tracker.Queue
	queueErr        error
	workflows       map[string]*tracker.Workflow
	workflowErrs    map[string]error
	components      []*tracker.Component
	componentsErr   error
	queueFields     []*tracker.Field
	queueFieldsErr  error
	localFields     []*tracker.Field
	localFieldsErr  error
	globalFields    []*tracker.Field
	globalFieldsErr error
}

func (m *mockContextClient) record(call string) {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.calls = append(m.calls, call)
}

// sortedCalls returns the recorded calls in sorted order, since the part
// requests run concurrently.
func (m *mockContextClient) sortedCalls() []string {
	m.mu.Lock()
	defer m.mu.Unlock()
	calls := slices.Clone(m.calls)
	slices.Sort(calls)
	return calls
}

func (m *mockContextClient) GetQueue(
	_ context.Context,
	key string,
	opts *tracker.QueueGetOptions,
) (*tracker.Queue, *tracker.Response, error) {
	expand := ""
	if opts != nil {
		expand = opts.Expand
	}
	m.record("GetQueue " + key + " expand=" + expand)
	if m.queueErr != nil {
		return nil, nil, m.queueErr
	}
	return m.queue, &tracker.Response{}, nil
}

func (m *mockContextClient) GetWorkflow(_ context.Context, id string) (*tracker.Workflow, *tracker.Response, error) {
	m.record("GetWorkflow " + id)
	if err := m.workflowErrs[id]; err != nil {
		return nil, nil, err
	}
	return m.workflows[id], &tracker.Response{}, nil
}

func (m *mockContextClient) ListComponents(
	_ context.Context,
	key string,
	opts *tracker.QueueComponentsListOptions,
) ([]*tracker.Component, *tracker.Response, error) {
	fields := ""
	if opts != nil {
		fields = opts.Fields
	}
	m.record("ListComponents " + key + " fields=" + fields)
	if m.componentsErr != nil {
		return nil, nil, m.componentsErr
	}
	return m.components, &tracker.Response{}, nil
}

func (m *mockContextClient) ListQueueFields(
	_ context.Context,
	key string,
) ([]*tracker.Field, *tracker.Response, error) {
	m.record("ListQueueFields " + key)
	if m.queueFieldsErr != nil {
		return nil, nil, m.queueFieldsErr
	}
	return m.queueFields, &tracker.Response{}, nil
}

func (m *mockContextClient) ListLocalFields(
	_ context.Context,
	key string,
) ([]*tracker.Field, *tracker.Response, error) {
	m.record("ListLocalFields " + key)
	if m.localFieldsErr != nil {
		return nil, nil, m.localFieldsErr
	}
	return m.localFields, &tracker.Response{}, nil
}

func (m *mockContextClient) ListGlobalFields(_ context.Context) ([]*tracker.Field, *tracker.Response, error) {
	m.record("ListGlobalFields")
	if m.globalFieldsErr != nil {
		return nil, nil, m.globalFieldsErr
	}
	return m.globalFields, &tracker.Response{}, nil
}

// decodeFixture decodes a JSON fixture the way the library decodes a response.
func decodeFixture[T any](t *testing.T, data string) T {
	t.Helper()
	var v T
	if err := json.Unmarshal([]byte(data), &v); err != nil {
		t.Fatalf("decode fixture: %v\n%s", err, data)
	}
	return v
}

// newAPPMock returns a mock whose every request succeeds with APP-shaped data.
func newAPPMock(t *testing.T) *mockContextClient {
	t.Helper()
	return &mockContextClient{
		queue:        decodeFixture[*tracker.Queue](t, appQueueJSON),
		workflows:    map[string]*tracker.Workflow{"W200": decodeFixture[*tracker.Workflow](t, w200JSON)},
		components:   decodeFixture[[]*tracker.Component](t, appComponentsJSON),
		queueFields:  decodeFixture[[]*tracker.Field](t, appQueueFieldsJSON),
		localFields:  decodeFixture[[]*tracker.Field](t, appLocalFieldsJSON),
		globalFields: decodeFixture[[]*tracker.Field](t, globalFieldsJSON),
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

// runContextCmd runs queue context against the mock, with the root persistent
// flags it reads, and returns stdout and stderr separately.
func runContextCmd(t *testing.T, mock *mockContextClient, args ...string) (string, string, error) {
	t.Helper()

	origClient := newContextClient
	newContextClient = func(_ *config.ResolvedAuth) queueContextClient {
		return mock
	}
	t.Cleanup(func() { newContextClient = origClient })

	stdout, stderr := &bytes.Buffer{}, &bytes.Buffer{}
	cmd := newContextCmd()
	cmd.SetOut(stdout)
	cmd.SetErr(stderr)
	cmd.SilenceErrors = true
	cmd.SilenceUsage = true

	cmd.PersistentFlags().StringSliceVar(&output.JSONFields, "json", nil, "")
	cmd.PersistentFlags().StringVar(&output.JQFilter, "jq", "", "")
	cmd.PersistentFlags().BoolVar(&output.QuietFlag, "quiet", false, "")
	cmd.PersistentFlags().String("token", "", "")
	cmd.PersistentFlags().String("org-id", "", "")
	cmd.PersistentFlags().String("org-type", "", "")

	cmd.SetArgs(append(args, "--token", "test-token", "--org-id", "test-org", "--org-type", "360"))
	err := cmd.Execute()
	return stdout.String(), stderr.String(), err
}

// assertJSONEqual compares two JSON documents by value.
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

// topLevelKeys reads an object's keys back in the order they were encoded,
// so an ordering check does not depend on how the document is formatted.
func topLevelKeys(t *testing.T, doc string) []string {
	t.Helper()
	dec := json.NewDecoder(strings.NewReader(doc))
	if _, err := dec.Token(); err != nil {
		t.Fatalf("output is not a JSON object: %v\n%s", err, doc)
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

// assertExitCode checks that err carries the given exit code.
func assertExitCode(t *testing.T, err error, want int) {
	t.Helper()
	var exitErr *ytrerrors.ExitError
	if !errors.As(err, &exitErr) {
		t.Fatalf("expected an ExitError, got %T: %v", err, err)
	}
	if exitErr.ExitCode != want {
		t.Errorf("exit code = %d, want %d (%v)", exitErr.ExitCode, want, err)
	}
}

// assertCalls compares the recorded requests, in sorted order.
func assertCalls(t *testing.T, mock *mockContextClient, want ...string) {
	t.Helper()
	slices.Sort(want)
	if got := mock.sortedCalls(); !slices.Equal(got, want) {
		t.Errorf("requests = %q, want %q", got, want)
	}
}

func TestQueueContextFull(t *testing.T) {
	testutil.ResetOutputFlags(t)
	mock := newAPPMock(t)

	stdout, _, err := runContextCmd(t, mock, "APP")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	assertJSONEqual(t, stdout, appFullDocument)

	// The whole document keeps the part order.
	if got := topLevelKeys(t, stdout); !slices.Equal(got, QueueContextFields) {
		t.Errorf("parts = %q, want %q", got, QueueContextFields)
	}

	// W200 is fetched once, though two issue types follow it.
	assertCalls(t, mock,
		"GetQueue APP expand=issueTypesConfig",
		"GetWorkflow W200",
		"ListComponents APP fields=name",
		"ListQueueFields APP",
		"ListLocalFields APP",
		"ListGlobalFields",
	)
}

func TestQueueContextRequiredFieldsATS(t *testing.T) {
	testutil.ResetOutputFlags(t)
	mock := &mockContextClient{
		queue: decodeFixture[*tracker.Queue](t, `{
		  "self": "https://api.tracker.yandex.net/v3/queues/OPS", "id": "7", "key": "OPS", "name": "OPS",
		  "defaultType": {"self": "https://api.tracker.yandex.net/v3/issuetypes/2", "id": "2", "key": "task", "display": "Задача"},
		  "defaultPriority": {"self": "https://api.tracker.yandex.net/v3/priorities/3", "id": "3", "key": "normal", "display": "Средний"}
		}`),
		queueFields: decodeFixture[[]*tracker.Field](t, `[
		  {"self": "https://api.tracker.yandex.net/v3/fields/type", "id": "type", "name": "Тип", "key": "type", "version": 0,
		   "schema": {"type": "issuetype", "required": true}, "readonly": false, "options": true, "suggest": true,
		   "suggestProvider": {"type": "IssueTypeSuggestProvider"}, "optionsProvider": {"type": "IssueTypeOptionsProvider"},
		   "queryProvider": {"type": "IssueTypeQueryProvider"}, "order": 2, "type": "standard"},
		  {"self": "https://api.tracker.yandex.net/v3/fields/priority", "id": "priority", "name": "Приоритет", "key": "priority",
		   "version": 0, "schema": {"type": "priority", "required": true}, "readonly": false, "options": true, "suggest": true,
		   "optionsProvider": {"type": "PriorityOptionsProvider"}, "order": 3, "type": "standard"},
		  {"self": "https://api.tracker.yandex.net/v3/fields/createdBy", "id": "createdBy", "name": "Автор", "key": "createdBy",
		   "version": 0, "schema": {"type": "user", "required": true}, "readonly": false, "options": true, "suggest": true,
		   "optionsProvider": {"type": "TeamOptionsProvider"}, "order": 5, "type": "standard"},
		  {"self": "https://api.tracker.yandex.net/v3/fields/start", "id": "start", "name": "Начало", "key": "start",
		   "version": 0, "schema": {"type": "date", "required": false}, "readonly": false, "options": false, "suggest": false,
		   "order": 10, "type": "standard"},
		  {"self": "https://api.tracker.yandex.net/v3/fields/followers", "id": "followers", "name": "Наблюдатели",
		   "key": "followers", "version": 0, "schema": {"type": "array", "items": "user"}, "readonly": false,
		   "options": true, "suggest": true, "suggestProvider": {"type": "UserSuggestProvider"},
		   "optionsProvider": {"type": "TeamOptionsProvider"}, "queryProvider": {"type": "UserOptionalQueryProvider"},
		   "order": 22, "type": "standard"}
		]`),
	}

	stdout, _, err := runContextCmd(t, mock, "OPS", "--json", "requiredFields")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	assertJSONEqual(t, stdout, `{
	  "requiredFields": [
	    {"id": "summary"},
	    {"id": "type", "default": "task"},
	    {"id": "priority", "default": "normal"},
	    {"id": "createdBy"}
	  ],
	  "incomplete": []
	}`)
	assertCalls(t, mock, "GetQueue OPS expand=issueTypesConfig", "ListQueueFields OPS")
}

func TestQueueContextQueueFieldsEmpty(t *testing.T) {
	testutil.ResetOutputFlags(t)
	mock := newAPPMock(t)
	mock.queueFields = decodeFixture[[]*tracker.Field](t, `[]`)

	stdout, _, err := runContextCmd(t, mock, "SUP", "--json", "requiredFields")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	assertJSONEqual(t, stdout, `{
	  "requiredFields": [{"id": "summary"}],
	  "incomplete": [
	    {"part": "requiredFields", "reason": "Tracker listed no queue fields, so other fields may be required"}
	  ]
	}`)
}

func TestQueueContextQueueFieldsForbidden(t *testing.T) {
	testutil.ResetOutputFlags(t)
	const reason = "У вас недостаточно прав в очереди RESTRICTED."
	mock := newAPPMock(t)
	mock.queue = decodeFixture[*tracker.Queue](t, `{
	  "self": "https://api.tracker.yandex.net/v3/queues/RESTRICTED", "id": "9", "key": "RESTRICTED", "name": "Закрытая",
	  "defaultType": {"self": "https://api.tracker.yandex.net/v3/issuetypes/1", "id": "1", "key": "bug", "display": "Ошибка"},
	  "defaultPriority": {"self": "https://api.tracker.yandex.net/v3/priorities/3", "id": "3", "key": "normal", "display": "Средний"},
	  "issueTypesConfig": [
	    {"issueType": {"self": "https://api.tracker.yandex.net/v3/issuetypes/21", "id": "21", "key": "milestone", "display": "Веха"},
	     "workflow": {"self": "https://api.tracker.yandex.net/v3/workflows/W100", "id": "W100", "display": "W100"}}
	  ]
	}`)
	w100 := decodeFixture[*tracker.Workflow](t, strings.ReplaceAll(w200JSON, "W200", "W100"))
	mock.workflows = map[string]*tracker.Workflow{"W100": w100}
	mock.components = decodeFixture[[]*tracker.Component](t, `[]`)
	mock.localFields = decodeFixture[[]*tracker.Field](t, `[]`)
	mock.queueFieldsErr = newAPIError(http.StatusForbidden, reason)

	stdout, _, err := runContextCmd(t, mock, "RESTRICTED")
	if err != nil {
		t.Fatalf("a failed part must not fail the command, got: %v", err)
	}

	var doc map[string]json.RawMessage
	if err := json.Unmarshal([]byte(stdout), &doc); err != nil {
		t.Fatalf("output is not JSON: %v\n%s", err, stdout)
	}
	assertJSONEqual(t, string(doc["requiredFields"]), `null`)
	assertJSONEqual(t, string(doc["incomplete"]), `[{"part": "requiredFields", "reason": "`+reason+`"}]`)
	// Parts that were fetched but are empty stay [], unlike the failed one.
	assertJSONEqual(t, string(doc["components"]), `[]`)
	assertJSONEqual(t, string(doc["localFields"]), `[]`)
	assertJSONEqual(t, string(doc["issueTypes"]), `[{"key": "milestone", "name": "Веха", "workflow": "W100"}]`)
}

func TestQueueContextWorkflowFails(t *testing.T) {
	testutil.ResetOutputFlags(t)
	mock := newAPPMock(t)
	mock.queue = decodeFixture[*tracker.Queue](t, twoWorkflowQueueJSON)
	mock.workflowErrs = map[string]error{"W100": newAPIError(http.StatusNotFound, "Workflow W100 not found")}

	stdout, _, err := runContextCmd(t, mock, "APP")
	if err != nil {
		t.Fatalf("a failed part must not fail the command, got: %v", err)
	}

	var doc map[string]json.RawMessage
	if err := json.Unmarshal([]byte(stdout), &doc); err != nil {
		t.Fatalf("output is not JSON: %v\n%s", err, stdout)
	}
	assertJSONEqual(t, string(doc["workflows"]), `null`)
	assertJSONEqual(t, string(doc["statuses"]), `null`)
	assertJSONEqual(t, string(doc["incomplete"]), `[
	  {"part": "statuses", "reason": "workflow W100: Workflow W100 not found"},
	  {"part": "workflows", "reason": "workflow W100: Workflow W100 not found"}
	]`)
	// The other parts are unaffected.
	assertJSONEqual(t, string(doc["components"]), `[{"id": "55", "name": "Hotfix"}]`)
	assertJSONEqual(t, string(doc["globalFields"]), `[{"key": "tags", "name": "Теги"}]`)
}

func TestQueueContextStatusesOnly(t *testing.T) {
	testutil.ResetOutputFlags(t)
	mock := newAPPMock(t)

	stdout, _, err := runContextCmd(t, mock, "APP", "--json", "statuses")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	assertJSONEqual(t, stdout, `{
	  "statuses": [{"key": "open", "name": "Открыт"}, {"key": "closed", "name": "Закрыт"}],
	  "incomplete": []
	}`)
	assertCalls(t, mock, "GetQueue APP expand=issueTypesConfig", "GetWorkflow W200")
}

func TestQueueContextWorkflowFailsUnderSelection(t *testing.T) {
	testutil.ResetOutputFlags(t)
	mock := newAPPMock(t)
	mock.queue = decodeFixture[*tracker.Queue](t, twoWorkflowQueueJSON)
	mock.workflowErrs = map[string]error{"W100": newAPIError(http.StatusNotFound, "Workflow W100 not found")}

	stdout, _, err := runContextCmd(t, mock, "APP", "--json", "workflows")
	if err != nil {
		t.Fatalf("a failed part must not fail the command, got: %v", err)
	}

	// statuses was not selected, so incomplete does not name it.
	assertJSONEqual(t, stdout, `{
	  "workflows": null,
	  "incomplete": [{"part": "workflows", "reason": "workflow W100: Workflow W100 not found"}]
	}`)
}

func TestQueueContextStatusesAcrossWorkflows(t *testing.T) {
	testutil.ResetOutputFlags(t)
	mock := newAPPMock(t)
	mock.queue = decodeFixture[*tracker.Queue](t, twoWorkflowQueueJSON)
	mock.workflows = map[string]*tracker.Workflow{
		"W200": decodeFixture[*tracker.Workflow](t, w200JSON),
		"W100": decodeFixture[*tracker.Workflow](t, w100JSON),
	}

	stdout, _, err := runContextCmd(t, mock, "APP", "--json", "statuses,workflows")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	assertJSONEqual(t, stdout, `{
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

func TestQueueContextTerminalStep(t *testing.T) {
	testutil.ResetOutputFlags(t)
	mock := newAPPMock(t)

	stdout, _, err := runContextCmd(t, mock, "APP", "--json", "workflows")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	assertJSONEqual(t, stdout, `{
	  "workflows": [{"id": "W200", "initialStatus": "open", "transitions": {"open": ["closed"], "closed": []}}],
	  "incomplete": []
	}`)
	if !strings.Contains(stdout, `"closed":[]`) {
		t.Errorf("a step without actions must map to [], got:\n%s", stdout)
	}
	// Steps keep the workflow's order rather than sorting.
	if strings.Index(stdout, `"open":[`) > strings.Index(stdout, `"closed":[]`) {
		t.Errorf("transitions are not in step order:\n%s", stdout)
	}
}

func TestQueueContextOptionTypes(t *testing.T) {
	testutil.ResetOutputFlags(t)
	mock := newAPPMock(t)
	mock.localFields = decodeFixture[[]*tracker.Field](t, `[
	  {"self": "https://api.tracker.yandex.net/v3/queues/APP/localFields/flag", "id": "5d0e4f1a2b3c4d5e6f708192--flag",
	   "name": "Фича", "key": "flag", "version": 1, "schema": {"type": "integer", "required": false}, "readonly": false,
	   "optionsProvider": {"type": "FixedListOptionsProvider", "values": [0, 1]}, "type": "local"},
	  {"self": "https://api.tracker.yandex.net/v3/queues/APP/localFields/stand", "id": "5d0e4f1a2b3c4d5e6f708192--stand",
	   "name": "Среда", "key": "stand", "version": 1, "schema": {"type": "string", "required": false}, "readonly": false,
	   "optionsProvider": {"type": "QueueFixedListOptionsProvider",
	     "values": {"APP": ["Test", "Beta"], "DIRECT": ["Production"]}, "defaults": ["Not specified"]},
	   "type": "local"},
	  {"self": "https://api.tracker.yandex.net/v3/queues/APP/localFields/bench", "id": "5d0e4f1a2b3c4d5e6f708192--bench",
	   "name": "База", "key": "bench", "version": 1, "schema": {"type": "string", "required": false}, "readonly": true,
	   "optionsProvider": {"type": "QueueFixedListOptionsProvider",
	     "values": {"DIRECT": ["Production"]}, "defaults": ["Not specified", "Trunk"]},
	   "type": "local"}
	]`)

	stdout, _, err := runContextCmd(t, mock, "APP", "--json", "localFields")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	assertJSONEqual(t, stdout, `{
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

func TestQueueContextOptionsByQueueID(t *testing.T) {
	testutil.ResetOutputFlags(t)
	mock := newAPPMock(t)
	// Tracker accepts the queue id in place of its key, and the queue it
	// returns carries the key that the per-queue option lists use.
	mock.localFields = decodeFixture[[]*tracker.Field](t, `[
	  {"self": "https://api.tracker.yandex.net/v3/queues/APP/localFields/stand", "id": "5d0e4f1a2b3c4d5e6f708192--stand",
	   "name": "Среда", "key": "stand", "version": 1, "schema": {"type": "string", "required": false}, "readonly": false,
	   "optionsProvider": {"type": "QueueFixedListOptionsProvider",
	     "values": {"APP": ["Test", "Beta"], "DIRECT": ["Production"]}, "defaults": ["Not specified"]},
	   "type": "local"}
	]`)

	stdout, _, err := runContextCmd(t, mock, "140", "--json", "localFields")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	assertJSONEqual(t, stdout, `{
	  "localFields": [
	    {"id": "5d0e4f1a2b3c4d5e6f708192--stand", "key": "stand", "name": "Среда", "schema": "string",
	     "readonly": false, "options": ["Test", "Beta"]}
	  ],
	  "incomplete": []
	}`)
	assertCalls(t, mock, "GetQueue 140 expand=issueTypesConfig", "ListLocalFields 140")
}

func TestQueueContextSeveralPartsFail(t *testing.T) {
	testutil.ResetOutputFlags(t)
	mock := newAPPMock(t)
	mock.componentsErr = newAPIError(http.StatusForbidden, "components denied")
	mock.localFieldsErr = newAPIError(http.StatusNotFound, "local fields not found")
	mock.globalFieldsErr = newAPIError(http.StatusInternalServerError, "global fields unavailable")

	stdout, _, err := runContextCmd(t, mock, "APP")
	if err != nil {
		t.Fatalf("a failed part must not fail the command, got: %v", err)
	}

	var doc map[string]json.RawMessage
	if err := json.Unmarshal([]byte(stdout), &doc); err != nil {
		t.Fatalf("output is not JSON: %v\n%s", err, stdout)
	}
	for _, part := range []string{"components", "localFields", "globalFields"} {
		assertJSONEqual(t, string(doc[part]), `null`)
	}
	assertJSONEqual(t, string(doc["incomplete"]), `[
	  {"part": "components", "reason": "components denied"},
	  {"part": "localFields", "reason": "local fields not found"},
	  {"part": "globalFields", "reason": "global fields unavailable"}
	]`)
	// The parts whose requests succeeded are unaffected.
	assertJSONEqual(t, string(doc["requiredFields"]), `[{"id": "summary"}, {"id": "type", "default": "task"}]`)
}

func TestQueueContextSelectionIgnoresCase(t *testing.T) {
	testutil.ResetOutputFlags(t)
	mock := newAPPMock(t)

	stdout, _, err := runContextCmd(t, mock, "APP", "--json", "IssueTypes,COMPONENTS")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	assertJSONEqual(t, stdout, `{
	  "issueTypes": [
	    {"key": "task", "name": "Задача", "workflow": "W200"},
	    {"key": "bug", "name": "Ошибка", "workflow": "W200"}
	  ],
	  "components": [{"id": "55", "name": "Hotfix"}],
	  "incomplete": []
	}`)
	assertCalls(t, mock, "GetQueue APP expand=issueTypesConfig", "ListComponents APP fields=name")
}

func TestQueueContextSelection(t *testing.T) {
	testutil.ResetOutputFlags(t)
	mock := newAPPMock(t)

	stdout, _, err := runContextCmd(t, mock, "APP", "--json", "issueTypes,components")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	assertJSONEqual(t, stdout, `{
	  "issueTypes": [
	    {"key": "task", "name": "Задача", "workflow": "W200"},
	    {"key": "bug", "name": "Ошибка", "workflow": "W200"}
	  ],
	  "components": [{"id": "55", "name": "Hotfix"}],
	  "incomplete": []
	}`)
	assertCalls(t, mock, "GetQueue APP expand=issueTypesConfig", "ListComponents APP fields=name")
}

func TestQueueContextJQ(t *testing.T) {
	testutil.ResetOutputFlags(t)
	mock := newAPPMock(t)

	stdout, _, err := runContextCmd(t, mock, "APP", "--jq", ".localFields[].id")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if got, want := strings.TrimSpace(stdout), "5d0e4f1a2b3c4d5e6f708192--size"; got != want {
		t.Errorf("jq output = %q, want %q", got, want)
	}
}

func TestQueueContextQuiet(t *testing.T) {
	testutil.ResetOutputFlags(t)
	mock := newAPPMock(t)

	stdout, _, err := runContextCmd(t, mock, "APP", "--quiet")

	assertExitCode(t, err, ytrerrors.ExitUserError)
	if stdout != "" {
		t.Errorf("stdout must be empty, got:\n%s", stdout)
	}
	assertCalls(t, mock)
}

func TestQueueContextUnknownQueue(t *testing.T) {
	testutil.ResetOutputFlags(t)
	mock := newAPPMock(t)
	mock.queueErr = newAPIError(http.StatusNotFound, "Очередь не существует.")

	stdout, _, err := runContextCmd(t, mock, "NOPE")

	assertExitCode(t, err, ytrerrors.ExitNotFound)
	if !strings.Contains(err.Error(), "Очередь не существует.") {
		t.Errorf("error must carry the server's text, got: %v", err)
	}
	if stdout != "" {
		t.Errorf("stdout must be empty, got:\n%s", stdout)
	}
	assertCalls(t, mock, "GetQueue NOPE expand=issueTypesConfig")
}

func TestQueueContextEmptyArg(t *testing.T) {
	testutil.ResetOutputFlags(t)
	mock := newAPPMock(t)

	stdout, _, err := runContextCmd(t, mock, "")

	assertExitCode(t, err, ytrerrors.ExitUserError)
	if stdout != "" {
		t.Errorf("stdout must be empty, got:\n%s", stdout)
	}
	assertCalls(t, mock)
}

func TestQueueContextInvalidField(t *testing.T) {
	testutil.ResetOutputFlags(t)
	mock := newAPPMock(t)

	_, _, err := runContextCmd(t, mock, "APP", "--json", "foo")

	var invalidField *ytrerrors.InvalidFieldError
	if !errors.As(err, &invalidField) {
		t.Fatalf("expected an InvalidFieldError, got %T: %v", err, err)
	}
	if invalidField.InvalidField != "foo" {
		t.Errorf("invalid field = %q, want foo", invalidField.InvalidField)
	}
	assertExitCode(t, err, ytrerrors.ExitUserError)
	assertCalls(t, mock)
}

func TestQueueContextFieldHint(t *testing.T) {
	testutil.ResetOutputFlags(t)
	mock := newAPPMock(t)

	stdout, stderr, err := runContextCmd(t, mock, "APP", "--json=")

	assertExitCode(t, err, ytrerrors.ExitUserError)
	if stdout != "" {
		t.Errorf("stdout must be empty, got:\n%s", stdout)
	}
	if !strings.Contains(stderr, "Available fields for queue context") {
		t.Errorf("expected the parts hint, got:\n%s", stderr)
	}
	for _, part := range QueueContextFields {
		if !strings.Contains(stderr, "  "+part+"\n") {
			t.Errorf("hint does not list %q:\n%s", part, stderr)
		}
	}
	assertCalls(t, mock)
}

// TestQueueContextFieldListsAgree checks the places that name the parts: the
// fields slice, the document's json tags, the JSON FIELDS help block and the
// completion registry.
func TestQueueContextFieldListsAgree(t *testing.T) {
	testutil.ResetOutputFlags(t)
	cmd := newContextCmd()

	docType := reflect.TypeFor[queueContext]()
	tags := make([]string, 0, docType.NumField())
	for i := range docType.NumField() {
		tags = append(tags, strings.Split(docType.Field(i).Tag.Get("json"), ",")[0])
	}
	if !slices.Equal(tags, QueueContextFields) {
		t.Errorf("json tags %q do not match QueueContextFields %q", tags, QueueContextFields)
	}

	if want := "JSON FIELDS\n  " + strings.Join(QueueContextFields, ", ") + "\n"; !strings.Contains(cmd.Long, want) {
		t.Errorf("JSON FIELDS block does not list the parts; want %q in:\n%s", want, cmd.Long)
	}

	if got, ok := jsonfields.Get("ytr queue context"); !ok || !slices.Equal(got, QueueContextFields) {
		t.Errorf("registry = %q (registered %v), want %q", got, ok, QueueContextFields)
	}
}
