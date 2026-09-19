package field

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"reflect"
	"strings"
	"testing"

	"github.com/slavkluev/go-yandex-tracker/tracker"

	"github.com/slavkluev/ytr/internal/config"
	"github.com/slavkluev/ytr/internal/output"
	"github.com/slavkluev/ytr/internal/testutil"
)

// mockFieldGetter implements fieldGetter for testing.
type mockFieldGetter struct {
	field        *tracker.Field
	err          error
	gotFieldID   string
	gotQueueKey  string
	calledGlobal bool
	calledLocal  bool
}

func (m *mockFieldGetter) Get(_ context.Context, fieldID string) (*tracker.Field, *tracker.Response, error) {
	m.calledGlobal = true
	m.gotFieldID = fieldID
	if m.err != nil {
		return nil, nil, m.err
	}
	return m.field, nil, nil
}

func (m *mockFieldGetter) GetLocal(
	_ context.Context, queueKey, fieldKey string,
) (*tracker.Field, *tracker.Response, error) {
	m.calledLocal = true
	m.gotQueueKey = queueKey
	m.gotFieldID = fieldKey
	if m.err != nil {
		return nil, nil, m.err
	}
	return m.field, nil, nil
}

func setupGetCmd(t *testing.T, mock *mockFieldGetter, args []string) (string, error) {
	t.Helper()

	origGetter := newFieldGetter
	newFieldGetter = func(_ *config.ResolvedAuth) fieldGetter { return mock }
	t.Cleanup(func() { newFieldGetter = origGetter })

	buf := &bytes.Buffer{}
	cmd := newGetCmd()
	cmd.SetOut(buf)
	cmd.SetErr(buf)
	cmd.PersistentFlags().String("token", "test-token", "")
	cmd.PersistentFlags().String("org-id", "test-org", "")
	cmd.PersistentFlags().String("org-type", "360", "")
	cmd.SetArgs(args)
	err := cmd.Execute()
	return buf.String(), err
}

func TestGet(t *testing.T) {
	tests := []struct {
		name  string
		mock  *mockFieldGetter
		args  []string
		setup func()
		check func(t *testing.T, mock *mockFieldGetter, out string, err error)
	}{
		{
			name: "global detail table output",
			mock: &mockFieldGetter{
				field: &tracker.Field{
					Key:      testutil.StrPtr("summary"),
					Name:     testutil.StrPtr("Summary"),
					Type:     testutil.StrPtr("standard"),
					Schema:   &tracker.FieldSchema{Type: testutil.StrPtr("string"), Required: testutil.BoolPtr(true)},
					Readonly: testutil.BoolPtr(false),
					Category: &tracker.FieldCategory{Display: testutil.StrPtr("System")},
				},
			},
			args: []string{"summary"},
			check: func(t *testing.T, mock *mockFieldGetter, out string, err error) {
				t.Helper()
				if err != nil {
					t.Fatalf("unexpected error: %v", err)
				}
				if !mock.calledGlobal {
					t.Error("expected global Get to be called")
				}
				for _, want := range []string{
					"Key:", "summary", "Name:", "Summary",
					"Type:", "standard", "Schema:", "string (required)",
					"Readonly:", "no", "Category:", "System",
				} {
					if !strings.Contains(out, want) {
						t.Errorf("detail output missing %q; got:\n%s", want, out)
					}
				}
			},
		},
		{
			name: "local detail with --queue",
			mock: &mockFieldGetter{
				field: &tracker.Field{
					Key:      testutil.StrPtr("local-field"),
					Name:     testutil.StrPtr("Local Field"),
					Type:     testutil.StrPtr("local"),
					Schema:   &tracker.FieldSchema{Type: testutil.StrPtr("string")},
					Readonly: testutil.BoolPtr(false),
					Queue:    &tracker.Queue{Key: testutil.StrPtr("PROJ")},
				},
			},
			args: []string{"local-field", "--queue", "PROJ"},
			check: func(t *testing.T, mock *mockFieldGetter, out string, err error) {
				t.Helper()
				if err != nil {
					t.Fatalf("unexpected error: %v", err)
				}
				if !mock.calledLocal {
					t.Error("expected local GetLocal to be called")
				}
				if mock.gotQueueKey != "PROJ" {
					t.Errorf("expected queue key PROJ, got %q", mock.gotQueueKey)
				}
				if !strings.Contains(out, "Queue:") {
					t.Errorf("output missing Queue field; got:\n%s", out)
				}
				if !strings.Contains(out, "PROJ") {
					t.Errorf("output missing PROJ; got:\n%s", out)
				}
			},
		},
		{
			name: "json output",
			mock: &mockFieldGetter{
				field: &tracker.Field{
					Key:  testutil.StrPtr("summary"),
					Name: testutil.StrPtr("Summary"),
					Type: testutil.StrPtr("standard"),
					Schema: &tracker.FieldSchema{
						Type:     testutil.StrPtr("string"),
						Required: testutil.BoolPtr(true),
					},
					Readonly:    testutil.BoolPtr(false),
					Category:    &tracker.FieldCategory{Display: testutil.StrPtr("System")},
					Description: testutil.StrPtr("Issue summary"),
				},
			},
			args:  []string{"summary"},
			setup: func() { output.JSONFields = FieldGetFields },
			check: func(t *testing.T, _ *mockFieldGetter, out string, err error) {
				t.Helper()
				if err != nil {
					t.Fatalf("unexpected error: %v", err)
				}
				var result map[string]any
				if jsonErr := json.Unmarshal([]byte(out), &result); jsonErr != nil {
					t.Fatalf("invalid JSON: %v\nraw: %s", jsonErr, out)
				}
				if result["key"] != "summary" {
					t.Errorf("expected key=summary, got %v", result["key"])
				}
				if result["schema"] != "string" {
					t.Errorf("expected schema=string, got %v", result["schema"])
				}
				if result["required"] != true {
					t.Errorf("expected required=true, got %v", result["required"])
				}
				if result["readonly"] != false {
					t.Errorf("expected readonly=false, got %v", result["readonly"])
				}
				if result["description"] != "Issue summary" {
					t.Errorf("expected description='Issue summary', got %v", result["description"])
				}
			},
		},
		{
			name: "quiet output",
			mock: &mockFieldGetter{
				field: &tracker.Field{
					Key: testutil.StrPtr("summary"),
				},
			},
			args:  []string{"summary"},
			setup: func() { output.QuietFlag = true },
			check: func(t *testing.T, _ *mockFieldGetter, out string, err error) {
				t.Helper()
				if err != nil {
					t.Fatalf("unexpected error: %v", err)
				}
				trimmed := strings.TrimSpace(out)
				if trimmed != "summary" {
					t.Errorf("expected 'summary', got %q", trimmed)
				}
			},
		},
		{
			name: "with options",
			mock: &mockFieldGetter{
				field: &tracker.Field{
					Key:             testutil.StrPtr("issueType"),
					Name:            testutil.StrPtr("Issue Type"),
					Type:            testutil.StrPtr("standard"),
					Schema:          &tracker.FieldSchema{Type: testutil.StrPtr("string")},
					Readonly:        testutil.BoolPtr(false),
					OptionsProvider: &tracker.OptionsProvider{Values: []any{"bug", "task", "story"}},
				},
			},
			args: []string{"issueType"},
			check: func(t *testing.T, _ *mockFieldGetter, out string, err error) {
				t.Helper()
				if err != nil {
					t.Fatalf("unexpected error: %v", err)
				}
				if !strings.Contains(out, "Options:") {
					t.Errorf("output missing Options field; got:\n%s", out)
				}
				if !strings.Contains(out, "bug, task, story") {
					t.Errorf("output missing option values; got:\n%s", out)
				}
			},
		},
		{
			name: "without options",
			mock: &mockFieldGetter{
				field: &tracker.Field{
					Key:      testutil.StrPtr("summary"),
					Name:     testutil.StrPtr("Summary"),
					Type:     testutil.StrPtr("standard"),
					Schema:   &tracker.FieldSchema{Type: testutil.StrPtr("string")},
					Readonly: testutil.BoolPtr(false),
				},
			},
			args: []string{"summary"},
			check: func(t *testing.T, _ *mockFieldGetter, out string, err error) {
				t.Helper()
				if err != nil {
					t.Fatalf("unexpected error: %v", err)
				}
				if strings.Contains(out, "Options:") {
					t.Errorf("output should not contain Options field; got:\n%s", out)
				}
			},
		},
		{
			name: "api error",
			mock: &mockFieldGetter{
				err: errors.New("connection refused"),
			},
			args: []string{"summary"},
			check: func(t *testing.T, _ *mockFieldGetter, _ string, err error) {
				t.Helper()
				if err == nil {
					t.Fatal("expected error, got nil")
				}
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			testutil.ResetOutputFlags(t)
			if tt.setup != nil {
				tt.setup()
			}
			out, err := setupGetCmd(t, tt.mock, tt.args)
			tt.check(t, tt.mock, out, err)
		})
	}
}

// localSizeDetailField returns a queue-local field fixture carrying the full
// field id that the Tracker API assigns to local fields.
func localSizeDetailField() *tracker.Field {
	return &tracker.Field{
		ID:       testutil.FlexStringPtr("66fd07bba913292094b4403c--size"),
		Key:      testutil.StrPtr("size"),
		Name:     testutil.StrPtr("Size"),
		Type:     testutil.StrPtr("local"),
		Schema:   &tracker.FieldSchema{Type: testutil.StrPtr("string")},
		Readonly: testutil.BoolPtr(false),
		Queue:    &tracker.Queue{Key: testutil.StrPtr("PROJ")},
	}
}

// tagsArrayField returns a field whose schema is an array of strings.
func tagsArrayField(required bool) *tracker.Field {
	return &tracker.Field{
		Key:  testutil.StrPtr("tags"),
		Name: testutil.StrPtr("Tags"),
		Type: testutil.StrPtr("standard"),
		Schema: &tracker.FieldSchema{
			Type:     testutil.StrPtr("array"),
			Items:    testutil.StrPtr("string"),
			Required: testutil.BoolPtr(required),
		},
		Readonly: testutil.BoolPtr(false),
	}
}

// decodeDetail parses field get JSON output into a generic map.
func decodeDetail(t *testing.T, out string) map[string]any {
	t.Helper()

	var result map[string]any
	if err := json.Unmarshal([]byte(out), &result); err != nil {
		t.Fatalf("invalid JSON: %v\nraw: %s", err, out)
	}
	return result
}

func TestGetCardShowsFullFieldID(t *testing.T) {
	testutil.ResetOutputFlags(t)

	mock := &mockFieldGetter{field: localSizeDetailField()}
	out, err := setupGetCmd(t, mock, []string{"size", "--queue", "PROJ"})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	for _, want := range []string{"ID:", "66fd07bba913292094b4403c--size"} {
		if !strings.Contains(out, want) {
			t.Errorf("detail output missing %q; got:\n%s", want, out)
		}
	}
}

func TestGetJSONIncludesFullFieldID(t *testing.T) {
	testutil.ResetOutputFlags(t)
	output.JSONFields = []string{"id", "key"}

	mock := &mockFieldGetter{field: localSizeDetailField()}
	out, err := setupGetCmd(t, mock, []string{"size", "--queue", "PROJ"})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	result := decodeDetail(t, out)
	if result["id"] != "66fd07bba913292094b4403c--size" {
		t.Errorf("expected full field id, got %v", result["id"])
	}
}

func TestGetCardSpellsOutArrayElementType(t *testing.T) {
	testutil.ResetOutputFlags(t)

	mock := &mockFieldGetter{field: tagsArrayField(true)}
	out, err := setupGetCmd(t, mock, []string{"tags"})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if !strings.Contains(out, "array of string (required)") {
		t.Errorf("detail output missing array element type; got:\n%s", out)
	}
}

func TestGetJSONIncludesArrayElementType(t *testing.T) {
	testutil.ResetOutputFlags(t)
	output.JSONFields = []string{"key", "schema", "items"}

	mock := &mockFieldGetter{field: tagsArrayField(false)}
	out, err := setupGetCmd(t, mock, []string{"tags"})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	result := decodeDetail(t, out)
	if result["items"] != "string" {
		t.Errorf("expected items=string, got %v", result["items"])
	}
}

func TestGetJSONKeepsStringOptions(t *testing.T) {
	testutil.ResetOutputFlags(t)
	output.JSONFields = []string{"key", "options"}

	field := localSizeDetailField()
	field.OptionsProvider = &tracker.OptionsProvider{Values: []any{"S", "M"}}
	mock := &mockFieldGetter{field: field}
	out, err := setupGetCmd(t, mock, []string{"size", "--queue", "PROJ"})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	result := decodeDetail(t, out)
	if want := []any{"S", "M"}; !reflect.DeepEqual(result["options"], want) {
		t.Errorf("expected options %v, got %#v", want, result["options"])
	}
}

func TestGetJSONKeepsNumericOptions(t *testing.T) {
	testutil.ResetOutputFlags(t)
	output.JSONFields = []string{"key", "options"}

	mock := &mockFieldGetter{field: decodeFieldFixture(t, possibleSpamJSON)}
	out, err := setupGetCmd(t, mock, []string{"possibleSpam"})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	result := decodeDetail(t, out)
	if want := []any{float64(0), float64(1)}; !reflect.DeepEqual(result["options"], want) {
		t.Errorf("expected numeric options [0 1], got %#v", result["options"])
	}
}

func TestGetCardShowsNumericOptions(t *testing.T) {
	testutil.ResetOutputFlags(t)

	mock := &mockFieldGetter{field: decodeFieldFixture(t, possibleSpamJSON)}
	out, err := setupGetCmd(t, mock, []string{"possibleSpam"})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if !strings.Contains(out, "Options:  0, 1\n") {
		t.Errorf("detail output missing numeric options; got:\n%s", out)
	}
}

func TestGetJSONPerQueueOptions(t *testing.T) {
	testutil.ResetOutputFlags(t)
	output.JSONFields = []string{"key", "options", "queueOptions", "defaultOptions"}

	mock := &mockFieldGetter{field: decodeFieldFixture(t, perQueueFieldJSON)}
	out, err := setupGetCmd(t, mock, []string{"stand"})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	result := decodeDetail(t, out)
	if _, present := result["options"]; present {
		t.Errorf("expected options to be omitted, got %v", result["options"])
	}
	wantQueue := map[string]any{
		"DIRECT": []any{"Not specified", "Test", "Developer", "Beta", "Production", "Trunk"},
	}
	if !reflect.DeepEqual(result["queueOptions"], wantQueue) {
		t.Errorf("expected queueOptions %v, got %#v", wantQueue, result["queueOptions"])
	}
	wantDefaults := []any{"Not specified", "Test", "Developer", "Beta", "Production"}
	if !reflect.DeepEqual(result["defaultOptions"], wantDefaults) {
		t.Errorf("expected defaultOptions %v, got %#v", wantDefaults, result["defaultOptions"])
	}
}

func TestGetCardShowsPerQueueOptions(t *testing.T) {
	testutil.ResetOutputFlags(t)

	mock := &mockFieldGetter{field: decodeFieldFixture(t, perQueueFieldJSON)}
	out, err := setupGetCmd(t, mock, []string{"stand"})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	for _, want := range []string{
		"Options (DIRECT):  Not specified, Test, Developer, Beta, Production, Trunk\n",
		"Default options:  Not specified, Test, Developer, Beta, Production\n",
	} {
		if !strings.Contains(out, want) {
			t.Errorf("detail output missing %q; got:\n%s", want, out)
		}
	}
	if strings.Contains(out, "Options:") {
		t.Errorf("detail output should not have a flat Options row; got:\n%s", out)
	}
}

func TestGetJSONOmitsOptionsWhenProviderHasNoValues(t *testing.T) {
	testutil.ResetOutputFlags(t)
	output.JSONFields = []string{"key", "options", "queueOptions", "defaultOptions"}

	mock := &mockFieldGetter{field: decodeFieldFixture(t, teamFieldJSON)}
	out, err := setupGetCmd(t, mock, []string{"team"})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	result := decodeDetail(t, out)
	for _, key := range []string{"options", "queueOptions", "defaultOptions"} {
		if _, present := result[key]; present {
			t.Errorf("expected %s to be omitted, got %v", key, result[key])
		}
	}
}

func TestGetCardOmitsOptionsWhenProviderHasNoValues(t *testing.T) {
	testutil.ResetOutputFlags(t)

	mock := &mockFieldGetter{field: decodeFieldFixture(t, teamFieldJSON)}
	out, err := setupGetCmd(t, mock, []string{"team"})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if strings.Contains(out, "ptions") {
		t.Errorf("detail output should have no options rows; got:\n%s", out)
	}
}

func TestGetPassesThroughOptionDecodeError(t *testing.T) {
	testutil.ResetOutputFlags(t)

	decodeErr := optionDecodeError(t)
	mock := &mockFieldGetter{err: decodeErr}
	_, err := setupGetCmd(t, mock, []string{"size"})
	if err == nil {
		t.Fatal("expected error, got nil")
	}
	if !strings.Contains(err.Error(), decodeErr.Error()) {
		t.Errorf("expected error to carry %q, got %q", decodeErr, err)
	}
}

func TestGetCardOrdersPerQueueOptionsByQueueKey(t *testing.T) {
	testutil.ResetOutputFlags(t)

	field := localSizeDetailField()
	field.OptionsProvider = &tracker.OptionsProvider{
		QueueValues: map[string][]any{
			"ZETA":  {"Z1"},
			"ALPHA": {"A1"},
		},
	}
	mock := &mockFieldGetter{field: field}
	out, err := setupGetCmd(t, mock, []string{"size", "--queue", "PROJ"})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	alpha := strings.Index(out, "Options (ALPHA):")
	zeta := strings.Index(out, "Options (ZETA):")
	if alpha < 0 || zeta < 0 {
		t.Fatalf("detail output missing a per-queue options row; got:\n%s", out)
	}
	if alpha > zeta {
		t.Errorf("expected Options (ALPHA) before Options (ZETA); got:\n%s", out)
	}
}
