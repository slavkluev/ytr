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

	"github.com/slavkluev/ytr/internal/cmd/runner"
	"github.com/slavkluev/ytr/internal/config"
	ytrerrors "github.com/slavkluev/ytr/internal/errors"
	"github.com/slavkluev/ytr/internal/output"
)

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

// newAPPMock returns a mock whose every request succeeds with APP-shaped data.
func newAPPMock() *mockContextClient {
	return &mockContextClient{
		queue:        appQueue(),
		workflows:    map[string]*tracker.Workflow{"W200": w200()},
		components:   appComponents(),
		queueFields:  appQueueFields(),
		localFields:  appLocalFields(),
		globalFields: appGlobalFields(),
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

	opts := &output.Options{}
	cmd.PersistentFlags().StringSliceVar(&opts.JSONFields, "json", nil, "")
	cmd.PersistentFlags().StringVar(&opts.JQFilter, "jq", "", "")
	cmd.PersistentFlags().BoolVar(&opts.Quiet, "quiet", false, "")
	cmd.PersistentFlags().String("token", "", "")
	cmd.PersistentFlags().String("org-id", "", "")
	cmd.PersistentFlags().String("org-type", "", "")

	cmd.SetArgs(append(args, "--token", "test-token", "--org-id", "test-org", "--org-type", "360"))
	err := cmd.ExecuteContext(output.NewContext(t.Context(), opts))
	return stdout.String(), stderr.String(), err
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

func TestQueueContextRequests(t *testing.T) {
	tests := []struct {
		name  string
		args  []string
		setup func(*mockContextClient)
		calls []string
	}{
		{
			// W200 is fetched once, though two issue types follow it.
			name: "whole document",
			args: []string{"APP"},
			calls: []string{
				"GetQueue APP expand=issueTypesConfig",
				"GetWorkflow W200",
				"ListComponents APP fields=name",
				"ListQueueFields APP",
				"ListLocalFields APP",
				"ListGlobalFields",
			},
		},
		{
			name: "every workflow of the queue",
			args: []string{"APP", "--json", "workflows"},
			setup: func(m *mockContextClient) {
				m.queue = twoWorkflowQueue()
				m.workflows["W100"] = w100()
			},
			calls: []string{"GetQueue APP expand=issueTypesConfig", "GetWorkflow W200", "GetWorkflow W100"},
		},
		{
			name:  "requiredFields",
			args:  []string{"OPS", "--json", "requiredFields"},
			calls: []string{"GetQueue OPS expand=issueTypesConfig", "ListQueueFields OPS"},
		},
		{
			name:  "statuses",
			args:  []string{"APP", "--json", "statuses"},
			calls: []string{"GetQueue APP expand=issueTypesConfig", "GetWorkflow W200"},
		},
		{
			name:  "localFields by queue id",
			args:  []string{"140", "--json", "localFields"},
			calls: []string{"GetQueue 140 expand=issueTypesConfig", "ListLocalFields 140"},
		},
		{
			name:  "issueTypes and components",
			args:  []string{"APP", "--json", "issueTypes,components"},
			calls: []string{"GetQueue APP expand=issueTypesConfig", "ListComponents APP fields=name"},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			mock := newAPPMock()
			if tt.setup != nil {
				tt.setup(mock)
			}

			if _, _, err := runContextCmd(t, mock, tt.args...); err != nil {
				t.Fatalf("unexpected error: %v", err)
			}

			assertCalls(t, mock, tt.calls...)
		})
	}
}

func TestQueueContextFailedPartExitsZero(t *testing.T) {
	workflowFails := func(m *mockContextClient) {
		m.queue = twoWorkflowQueue()
		m.workflowErrs = map[string]error{"W100": newAPIError(http.StatusNotFound, "Workflow W100 not found")}
	}

	tests := []struct {
		name  string
		args  []string
		setup func(*mockContextClient)
	}{
		{
			name: "queue fields forbidden",
			args: []string{"APP"},
			setup: func(m *mockContextClient) {
				m.queueFieldsErr = newAPIError(http.StatusForbidden, "У вас недостаточно прав в очереди APP.")
			},
		},
		{name: "workflow fails", args: []string{"APP"}, setup: workflowFails},
		{name: "workflow fails under a selection", args: []string{"APP", "--json", "workflows"}, setup: workflowFails},
		{
			name: "several parts fail",
			args: []string{"APP"},
			setup: func(m *mockContextClient) {
				m.componentsErr = newAPIError(http.StatusForbidden, "components denied")
				m.localFieldsErr = newAPIError(http.StatusNotFound, "local fields not found")
				m.globalFieldsErr = newAPIError(http.StatusInternalServerError, "global fields unavailable")
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			mock := newAPPMock()
			tt.setup(mock)

			stdout, _, err := runContextCmd(t, mock, tt.args...)
			if err != nil {
				t.Fatalf("a failed part must not fail the command, got: %v", err)
			}

			var doc struct {
				Incomplete []incompletePart `json:"incomplete"`
			}
			if err := json.Unmarshal([]byte(stdout), &doc); err != nil {
				t.Fatalf("output is not JSON: %v\n%s", err, stdout)
			}
			if len(doc.Incomplete) == 0 {
				t.Errorf("incomplete does not name the failed part:\n%s", stdout)
			}
		})
	}
}

func TestQueueContextJSONIsTheDocument(t *testing.T) {
	workflowErr := newAPIError(http.StatusNotFound, "Workflow W100 not found")
	componentsErr := newAPIError(http.StatusForbidden, "components denied")

	failing := newAPPMock()
	failing.queue = twoWorkflowQueue()
	failing.workflowErrs = map[string]error{"W100": workflowErr}
	failing.componentsErr = componentsErr

	failed := appResults()
	failed.workflows, failed.failedWorkflow, failed.workflowsErr = nil, "W100", workflowErr
	failed.components, failed.componentsErr = nil, componentsErr

	queueFieldsErr := newAPIError(http.StatusForbidden, "queue fields denied")
	localFieldsErr := newAPIError(http.StatusNotFound, "local fields not found")
	globalErr := newAPIError(http.StatusInternalServerError, "global fields unavailable")

	failingFields := newAPPMock()
	failingFields.queueFieldsErr = queueFieldsErr
	failingFields.localFieldsErr = localFieldsErr
	failingFields.globalFieldsErr = globalErr

	failedFields := appResults()
	failedFields.queueFields, failedFields.queueFieldsErr = nil, queueFieldsErr
	failedFields.localFields, failedFields.localFieldsErr = nil, localFieldsErr
	failedFields.globalFields, failedFields.globalErr = nil, globalErr

	tests := []struct {
		name    string
		mock    *mockContextClient
		results contextResults
		fields  []string
	}{
		{name: "whole document", mock: newAPPMock(), results: appResults()},
		{
			name:    "failed parts under a selection",
			mock:    failing,
			results: failed,
			fields:  []string{"statuses", "components", "requiredFields"},
		},
		{name: "failed field parts", mock: failingFields, results: failedFields},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			args := []string{"APP"}
			if tt.fields != nil {
				args = append(args, "--json", strings.Join(tt.fields, ","))
			}

			stdout, _, err := runContextCmd(t, tt.mock, args...)
			if err != nil {
				t.Fatalf("unexpected error: %v", err)
			}

			want := marshalDocument(t, contextDocument(tt.mock.queue, "APP", tt.results, tt.fields))
			if stdout != want+"\n" {
				t.Errorf("stdout is not the queue context document\ngot:  %s\nwant: %s", stdout, want)
			}
		})
	}
}

func TestQueueContextSelectionIgnoresCase(t *testing.T) {
	mock := newAPPMock()

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

func TestQueueContextJQ(t *testing.T) {
	mock := newAPPMock()

	stdout, _, err := runContextCmd(t, mock, "APP", "--jq", ".localFields[].id")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if got, want := strings.TrimSpace(stdout), "5d0e4f1a2b3c4d5e6f708192--size"; got != want {
		t.Errorf("jq output = %q, want %q", got, want)
	}
}

func TestQueueContextQuiet(t *testing.T) {
	mock := newAPPMock()

	stdout, _, err := runContextCmd(t, mock, "APP", "--quiet")

	assertExitCode(t, err, ytrerrors.ExitUserError)
	if stdout != "" {
		t.Errorf("stdout must be empty, got:\n%s", stdout)
	}
	assertCalls(t, mock)
}

func TestQueueContextUnknownQueue(t *testing.T) {
	mock := newAPPMock()
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
	mock := newAPPMock()

	stdout, _, err := runContextCmd(t, mock, "")

	assertExitCode(t, err, ytrerrors.ExitUserError)
	if stdout != "" {
		t.Errorf("stdout must be empty, got:\n%s", stdout)
	}
	assertCalls(t, mock)
}

func TestQueueContextInvalidField(t *testing.T) {
	mock := newAPPMock()

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
	mock := newAPPMock()

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
// fields completion reads off the command.
func TestQueueContextFieldListsAgree(t *testing.T) {
	cmd := newContextCmd()

	docType := reflect.TypeFor[queueContext]()
	tags := make([]string, 0, docType.NumField())
	for i := range docType.NumField() {
		tags = append(tags, strings.Split(docType.Field(i).Tag.Get("json"), ",")[0])
	}
	if !slices.Equal(tags, QueueContextFields) {
		t.Errorf("json tags %q do not match QueueContextFields %q", tags, QueueContextFields)
	}

	if want := "JSON FIELDS\n  " + strings.Join(QueueContextFields, ", "); !strings.HasSuffix(cmd.Long, want) {
		t.Errorf("JSON FIELDS block does not list the parts; want %q at the end of:\n%s", want, cmd.Long)
	}

	if got, ok := runner.Fields(cmd); !ok || !slices.Equal(got, QueueContextFields) {
		t.Errorf("command fields = %q (set %v), want %q", got, ok, QueueContextFields)
	}
}
