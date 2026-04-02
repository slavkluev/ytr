package field

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
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
					OptionsProvider: &tracker.OptionsProvider{Values: []string{"bug", "task", "story"}},
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
