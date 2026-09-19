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

const mtpQueueJSON = `{
  "self": "https://api.tracker.yandex.net/v3/queues/MTP",
  "id": 140,
  "key": "MTP",
  "version": 5,
  "name": "Metal trading platform",
  "defaultType": {"self": "https://api.tracker.yandex.net/v3/issuetypes/2", "id": "2", "key": "task", "display": "Задача"},
  "defaultPriority": {"self": "https://api.tracker.yandex.net/v3/priorities/3", "id": "3", "key": "normal", "display": "Средний"},
  "issueTypesConfig": [
    {"issueType": {"self": "https://api.tracker.yandex.net/v3/issuetypes/2", "id": "2", "key": "task", "display": "Задача"},
     "workflow": {"self": "https://api.tracker.yandex.net/v3/workflows/W207", "id": "W207", "display": "W207"},
     "resolutions": [{"self": "https://api.tracker.yandex.net/v3/resolutions/1", "id": "1", "key": "fixed", "display": "Решен"}]},
    {"issueType": {"self": "https://api.tracker.yandex.net/v3/issuetypes/1", "id": "1", "key": "bug", "display": "Ошибка"},
     "workflow": {"self": "https://api.tracker.yandex.net/v3/workflows/W207", "id": "W207", "display": "W207"}}
  ]
}`

// w207JSON has a terminal step without actions, and two actions of the open
// step that lead to the same status.
const w207JSON = `{
  "self": "https://api.tracker.yandex.net/v3/workflows/W207",
  "id": "W207",
  "name": "W207",
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
  "queue": {"self": "https://api.tracker.yandex.net/v3/queues/MTP", "id": "140", "key": "MTP", "display": "Metal trading platform"},
  "created": "2026-08-11T14:37:06.356+0000",
  "updated": "2026-08-11T14:37:06.356+0000",
  "deleted": false,
  "type": "visual"
}`

const mtpComponentsJSON = `[
  {"self": "https://api.tracker.yandex.net/v3/components/55", "id": 55, "name": "Expedite"}
]`

// mtpQueueFieldsJSON pairs a required system field with a field that is not
// required and has per-queue values.
const mtpQueueFieldsJSON = `[
  {"self": "https://api.tracker.yandex.net/v3/fields/type", "id": "type", "name": "Тип", "key": "type", "version": 0,
   "schema": {"type": "issuetype", "required": true}, "readonly": false, "options": true, "suggest": true,
   "optionsProvider": {"type": "IssueTypeOptionsProvider"}, "order": 2, "type": "standard"},
  {"self": "https://api.tracker.yandex.net/v3/fields/stand", "id": "stand", "name": "Bench", "version": 1361890459119,
   "schema": {"type": "string", "required": false}, "readonly": false, "options": true, "suggest": false,
   "optionsProvider": {"type": "QueueFixedListOptionsProvider",
     "values": {"DIRECT": ["Not specified", "Test"]}, "defaults": ["Not specified", "Test"]},
   "order": 222}
]`

const mtpLocalFieldsJSON = `[
  {"self": "https://api.tracker.yandex.net/v3/queues/MTP/localFields/size",
   "id": "66fd07bba913292094b4403c--size", "name": "Размер задачи", "key": "size", "version": 1,
   "schema": {"type": "string", "required": false}, "readonly": false, "options": false, "suggest": false,
   "optionsProvider": {"type": "FixedListOptionsProvider", "needValidation": true, "values": ["S", "M", "L"]},
   "queryProvider": {"type": "StringOptionalQueryProvider"}, "order": 3,
   "queue": {"self": "https://api.tracker.yandex.net/v3/queues/MTP", "id": "140", "key": "MTP", "display": "Metal trading platform"},
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

const mtpFullDocument = `{
  "key": "MTP",
  "name": "Metal trading platform",
  "defaultType": "task",
  "defaultPriority": "normal",
  "issueTypes": [
    {"key": "task", "name": "Задача", "workflow": "W207"},
    {"key": "bug", "name": "Ошибка", "workflow": "W207"}
  ],
  "statuses": [{"key": "open", "name": "Открыт"}, {"key": "closed", "name": "Закрыт"}],
  "workflows": [{"id": "W207", "initialStatus": "open", "transitions": {"open": ["closed"], "closed": []}}],
  "components": [{"id": "55", "name": "Expedite"}],
  "requiredFields": [{"id": "summary"}, {"id": "type", "default": "task"}],
  "localFields": [
    {"id": "66fd07bba913292094b4403c--size", "key": "size", "name": "Размер задачи", "schema": "string",
     "readonly": false, "options": ["S", "M", "L"]}
  ],
  "globalFields": [{"key": "tags", "name": "Теги"}],
  "incomplete": []
}`

// w109JSON starts in its own status and then shares open and closed with
// W207.
const w109JSON = `{
  "self": "https://api.tracker.yandex.net/v3/workflows/W109",
  "id": "W109",
  "name": "W109",
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

// twoWorkflowQueueJSON is MTP with one issue type on W207 and one on W109.
const twoWorkflowQueueJSON = `{
  "self": "https://api.tracker.yandex.net/v3/queues/MTP", "id": 140, "key": "MTP", "name": "Metal trading platform",
  "issueTypesConfig": [
    {"issueType": {"self": "https://api.tracker.yandex.net/v3/issuetypes/2", "id": "2", "key": "task", "display": "Задача"},
     "workflow": {"self": "https://api.tracker.yandex.net/v3/workflows/W207", "id": "W207", "display": "W207"}},
    {"issueType": {"self": "https://api.tracker.yandex.net/v3/issuetypes/21", "id": "21", "key": "milestone", "display": "Веха"},
     "workflow": {"self": "https://api.tracker.yandex.net/v3/workflows/W109", "id": "W109", "display": "W109"}}
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

// newMTPMock returns a mock whose every request succeeds with MTP-shaped data.
func newMTPMock(t *testing.T) *mockContextClient {
	t.Helper()
	return &mockContextClient{
		queue:        decodeFixture[*tracker.Queue](t, mtpQueueJSON),
		workflows:    map[string]*tracker.Workflow{"W207": decodeFixture[*tracker.Workflow](t, w207JSON)},
		components:   decodeFixture[[]*tracker.Component](t, mtpComponentsJSON),
		queueFields:  decodeFixture[[]*tracker.Field](t, mtpQueueFieldsJSON),
		localFields:  decodeFixture[[]*tracker.Field](t, mtpLocalFieldsJSON),
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
	mock := newMTPMock(t)

	stdout, _, err := runContextCmd(t, mock, "MTP")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	assertJSONEqual(t, stdout, mtpFullDocument)

	// The whole document keeps the part order.
	last := -1
	for _, key := range QueueContextFields {
		idx := strings.Index(stdout, "\n  \""+key+"\": ")
		if idx <= last {
			t.Errorf("part %q is out of order in:\n%s", key, stdout)
		}
		last = idx
	}

	// W207 is fetched once, though two issue types follow it.
	assertCalls(t, mock,
		"GetQueue MTP expand=issueTypesConfig",
		"GetWorkflow W207",
		"ListComponents MTP fields=name",
		"ListQueueFields MTP",
		"ListLocalFields MTP",
		"ListGlobalFields",
	)
}

func TestQueueContextRequiredFieldsATS(t *testing.T) {
	testutil.ResetOutputFlags(t)
	mock := &mockContextClient{
		queue: decodeFixture[*tracker.Queue](t, `{
		  "self": "https://api.tracker.yandex.net/v3/queues/ATS", "id": "7", "key": "ATS", "name": "ATS",
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

	stdout, _, err := runContextCmd(t, mock, "ATS", "--json", "requiredFields")
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
	assertCalls(t, mock, "GetQueue ATS expand=issueTypesConfig", "ListQueueFields ATS")
}

func TestQueueContextQueueFieldsEmpty(t *testing.T) {
	testutil.ResetOutputFlags(t)
	mock := newMTPMock(t)
	mock.queueFields = decodeFixture[[]*tracker.Field](t, `[]`)

	stdout, _, err := runContextCmd(t, mock, "RPA", "--json", "requiredFields")
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
	const reason = "У вас недостаточно прав в очереди RECYCLEBIN."
	mock := newMTPMock(t)
	mock.queue = decodeFixture[*tracker.Queue](t, `{
	  "self": "https://api.tracker.yandex.net/v3/queues/RECYCLEBIN", "id": "9", "key": "RECYCLEBIN", "name": "Корзина",
	  "defaultType": {"self": "https://api.tracker.yandex.net/v3/issuetypes/1", "id": "1", "key": "bug", "display": "Ошибка"},
	  "defaultPriority": {"self": "https://api.tracker.yandex.net/v3/priorities/3", "id": "3", "key": "normal", "display": "Средний"},
	  "issueTypesConfig": [
	    {"issueType": {"self": "https://api.tracker.yandex.net/v3/issuetypes/21", "id": "21", "key": "milestone", "display": "Веха"},
	     "workflow": {"self": "https://api.tracker.yandex.net/v3/workflows/W109", "id": "W109", "display": "W109"}}
	  ]
	}`)
	w109 := decodeFixture[*tracker.Workflow](t, strings.ReplaceAll(w207JSON, "W207", "W109"))
	mock.workflows = map[string]*tracker.Workflow{"W109": w109}
	mock.components = decodeFixture[[]*tracker.Component](t, `[]`)
	mock.localFields = decodeFixture[[]*tracker.Field](t, `[]`)
	mock.queueFieldsErr = newAPIError(http.StatusForbidden, reason)

	stdout, _, err := runContextCmd(t, mock, "RECYCLEBIN")
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
	assertJSONEqual(t, string(doc["issueTypes"]), `[{"key": "milestone", "name": "Веха", "workflow": "W109"}]`)
}

func TestQueueContextWorkflowFails(t *testing.T) {
	testutil.ResetOutputFlags(t)
	mock := newMTPMock(t)
	mock.queue = decodeFixture[*tracker.Queue](t, twoWorkflowQueueJSON)
	mock.workflowErrs = map[string]error{"W109": newAPIError(http.StatusNotFound, "Workflow W109 not found")}

	stdout, _, err := runContextCmd(t, mock, "MTP")
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
	  {"part": "statuses", "reason": "workflow W109: Workflow W109 not found"},
	  {"part": "workflows", "reason": "workflow W109: Workflow W109 not found"}
	]`)
	// The other parts are unaffected.
	assertJSONEqual(t, string(doc["components"]), `[{"id": "55", "name": "Expedite"}]`)
	assertJSONEqual(t, string(doc["globalFields"]), `[{"key": "tags", "name": "Теги"}]`)
}

func TestQueueContextStatusesOnly(t *testing.T) {
	testutil.ResetOutputFlags(t)
	mock := newMTPMock(t)

	stdout, _, err := runContextCmd(t, mock, "MTP", "--json", "statuses")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	assertJSONEqual(t, stdout, `{
	  "statuses": [{"key": "open", "name": "Открыт"}, {"key": "closed", "name": "Закрыт"}],
	  "incomplete": []
	}`)
	assertCalls(t, mock, "GetQueue MTP expand=issueTypesConfig", "GetWorkflow W207")
}

func TestQueueContextWorkflowFailsUnderSelection(t *testing.T) {
	testutil.ResetOutputFlags(t)
	mock := newMTPMock(t)
	mock.queue = decodeFixture[*tracker.Queue](t, twoWorkflowQueueJSON)
	mock.workflowErrs = map[string]error{"W109": newAPIError(http.StatusNotFound, "Workflow W109 not found")}

	stdout, _, err := runContextCmd(t, mock, "MTP", "--json", "workflows")
	if err != nil {
		t.Fatalf("a failed part must not fail the command, got: %v", err)
	}

	// statuses was not selected, so incomplete does not name it.
	assertJSONEqual(t, stdout, `{
	  "workflows": null,
	  "incomplete": [{"part": "workflows", "reason": "workflow W109: Workflow W109 not found"}]
	}`)
}

func TestQueueContextStatusesAcrossWorkflows(t *testing.T) {
	testutil.ResetOutputFlags(t)
	mock := newMTPMock(t)
	mock.queue = decodeFixture[*tracker.Queue](t, twoWorkflowQueueJSON)
	mock.workflows = map[string]*tracker.Workflow{
		"W207": decodeFixture[*tracker.Workflow](t, w207JSON),
		"W109": decodeFixture[*tracker.Workflow](t, w109JSON),
	}

	stdout, _, err := runContextCmd(t, mock, "MTP", "--json", "statuses,workflows")
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
	    {"id": "W207", "initialStatus": "open", "transitions": {"open": ["closed"], "closed": []}},
	    {"id": "W109", "initialStatus": "new", "transitions": {"new": ["open"], "open": ["closed"], "closed": []}}
	  ],
	  "incomplete": []
	}`)
}

func TestQueueContextTerminalStep(t *testing.T) {
	testutil.ResetOutputFlags(t)
	mock := newMTPMock(t)

	stdout, _, err := runContextCmd(t, mock, "MTP", "--json", "workflows")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	assertJSONEqual(t, stdout, `{
	  "workflows": [{"id": "W207", "initialStatus": "open", "transitions": {"open": ["closed"], "closed": []}}],
	  "incomplete": []
	}`)
	if !strings.Contains(stdout, `"closed": []`) {
		t.Errorf("a step without actions must map to [], got:\n%s", stdout)
	}
	// Steps keep the workflow's order rather than sorting.
	if strings.Index(stdout, `"open": [`) > strings.Index(stdout, `"closed": []`) {
		t.Errorf("transitions are not in step order:\n%s", stdout)
	}
}

func TestQueueContextOptionTypes(t *testing.T) {
	testutil.ResetOutputFlags(t)
	mock := newMTPMock(t)
	mock.localFields = decodeFixture[[]*tracker.Field](t, `[
	  {"self": "https://api.tracker.yandex.net/v3/queues/MTP/localFields/flag", "id": "66fd07bba913292094b4403c--flag",
	   "name": "Флаг", "key": "flag", "version": 1, "schema": {"type": "integer", "required": false}, "readonly": false,
	   "optionsProvider": {"type": "FixedListOptionsProvider", "values": [0, 1]}, "type": "local"},
	  {"self": "https://api.tracker.yandex.net/v3/queues/MTP/localFields/stand", "id": "66fd07bba913292094b4403c--stand",
	   "name": "Стенд", "key": "stand", "version": 1, "schema": {"type": "string", "required": false}, "readonly": false,
	   "optionsProvider": {"type": "QueueFixedListOptionsProvider",
	     "values": {"MTP": ["Test", "Beta"], "DIRECT": ["Production"]}, "defaults": ["Not specified"]},
	   "type": "local"},
	  {"self": "https://api.tracker.yandex.net/v3/queues/MTP/localFields/bench", "id": "66fd07bba913292094b4403c--bench",
	   "name": "Бенч", "key": "bench", "version": 1, "schema": {"type": "string", "required": false}, "readonly": true,
	   "optionsProvider": {"type": "QueueFixedListOptionsProvider",
	     "values": {"DIRECT": ["Production"]}, "defaults": ["Not specified", "Trunk"]},
	   "type": "local"}
	]`)

	stdout, _, err := runContextCmd(t, mock, "MTP", "--json", "localFields")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	assertJSONEqual(t, stdout, `{
	  "localFields": [
	    {"id": "66fd07bba913292094b4403c--flag", "key": "flag", "name": "Флаг", "schema": "integer",
	     "readonly": false, "options": [0, 1]},
	    {"id": "66fd07bba913292094b4403c--stand", "key": "stand", "name": "Стенд", "schema": "string",
	     "readonly": false, "options": ["Test", "Beta"]},
	    {"id": "66fd07bba913292094b4403c--bench", "key": "bench", "name": "Бенч", "schema": "string",
	     "readonly": true, "options": ["Not specified", "Trunk"]}
	  ],
	  "incomplete": []
	}`)
}

func TestQueueContextOptionsByQueueID(t *testing.T) {
	testutil.ResetOutputFlags(t)
	mock := newMTPMock(t)
	// Tracker accepts the queue id in place of its key, and the queue it
	// returns carries the key that the per-queue option lists use.
	mock.localFields = decodeFixture[[]*tracker.Field](t, `[
	  {"self": "https://api.tracker.yandex.net/v3/queues/MTP/localFields/stand", "id": "66fd07bba913292094b4403c--stand",
	   "name": "Стенд", "key": "stand", "version": 1, "schema": {"type": "string", "required": false}, "readonly": false,
	   "optionsProvider": {"type": "QueueFixedListOptionsProvider",
	     "values": {"MTP": ["Test", "Beta"], "DIRECT": ["Production"]}, "defaults": ["Not specified"]},
	   "type": "local"}
	]`)

	stdout, _, err := runContextCmd(t, mock, "140", "--json", "localFields")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	assertJSONEqual(t, stdout, `{
	  "localFields": [
	    {"id": "66fd07bba913292094b4403c--stand", "key": "stand", "name": "Стенд", "schema": "string",
	     "readonly": false, "options": ["Test", "Beta"]}
	  ],
	  "incomplete": []
	}`)
	assertCalls(t, mock, "GetQueue 140 expand=issueTypesConfig", "ListLocalFields 140")
}

func TestQueueContextSeveralPartsFail(t *testing.T) {
	testutil.ResetOutputFlags(t)
	mock := newMTPMock(t)
	mock.componentsErr = newAPIError(http.StatusForbidden, "components denied")
	mock.localFieldsErr = newAPIError(http.StatusNotFound, "local fields not found")
	mock.globalFieldsErr = newAPIError(http.StatusInternalServerError, "global fields unavailable")

	stdout, _, err := runContextCmd(t, mock, "MTP")
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
	mock := newMTPMock(t)

	stdout, _, err := runContextCmd(t, mock, "MTP", "--json", "IssueTypes,COMPONENTS")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	assertJSONEqual(t, stdout, `{
	  "issueTypes": [
	    {"key": "task", "name": "Задача", "workflow": "W207"},
	    {"key": "bug", "name": "Ошибка", "workflow": "W207"}
	  ],
	  "components": [{"id": "55", "name": "Expedite"}],
	  "incomplete": []
	}`)
	assertCalls(t, mock, "GetQueue MTP expand=issueTypesConfig", "ListComponents MTP fields=name")
}

func TestQueueContextSelection(t *testing.T) {
	testutil.ResetOutputFlags(t)
	mock := newMTPMock(t)

	stdout, _, err := runContextCmd(t, mock, "MTP", "--json", "issueTypes,components")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	assertJSONEqual(t, stdout, `{
	  "issueTypes": [
	    {"key": "task", "name": "Задача", "workflow": "W207"},
	    {"key": "bug", "name": "Ошибка", "workflow": "W207"}
	  ],
	  "components": [{"id": "55", "name": "Expedite"}],
	  "incomplete": []
	}`)
	assertCalls(t, mock, "GetQueue MTP expand=issueTypesConfig", "ListComponents MTP fields=name")
}

func TestQueueContextJQ(t *testing.T) {
	testutil.ResetOutputFlags(t)
	mock := newMTPMock(t)

	stdout, _, err := runContextCmd(t, mock, "MTP", "--jq", ".localFields[].id")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if got, want := strings.TrimSpace(stdout), "66fd07bba913292094b4403c--size"; got != want {
		t.Errorf("jq output = %q, want %q", got, want)
	}
}

func TestQueueContextQuiet(t *testing.T) {
	testutil.ResetOutputFlags(t)
	mock := newMTPMock(t)

	stdout, _, err := runContextCmd(t, mock, "MTP", "--quiet")

	assertExitCode(t, err, ytrerrors.ExitUserError)
	if stdout != "" {
		t.Errorf("stdout must be empty, got:\n%s", stdout)
	}
	assertCalls(t, mock)
}

func TestQueueContextUnknownQueue(t *testing.T) {
	testutil.ResetOutputFlags(t)
	mock := newMTPMock(t)
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
	mock := newMTPMock(t)

	stdout, _, err := runContextCmd(t, mock, "")

	assertExitCode(t, err, ytrerrors.ExitUserError)
	if stdout != "" {
		t.Errorf("stdout must be empty, got:\n%s", stdout)
	}
	assertCalls(t, mock)
}

func TestQueueContextInvalidField(t *testing.T) {
	testutil.ResetOutputFlags(t)
	mock := newMTPMock(t)

	_, _, err := runContextCmd(t, mock, "MTP", "--json", "foo")

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
	mock := newMTPMock(t)

	stdout, stderr, err := runContextCmd(t, mock, "MTP", "--json=")

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
