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

// mockFieldLister implements fieldLister for testing.
type mockFieldLister struct {
	fields       []*tracker.Field
	err          error
	gotQueueKey  string
	calledGlobal bool
	calledLocal  bool
}

func (m *mockFieldLister) List(_ context.Context) ([]*tracker.Field, *tracker.Response, error) {
	m.calledGlobal = true
	if m.err != nil {
		return nil, nil, m.err
	}
	return m.fields, nil, nil
}

func (m *mockFieldLister) ListLocal(_ context.Context, queueKey string) ([]*tracker.Field, *tracker.Response, error) {
	m.calledLocal = true
	m.gotQueueKey = queueKey
	if m.err != nil {
		return nil, nil, m.err
	}
	return m.fields, nil, nil
}

func setupListCmd(t *testing.T, mock *mockFieldLister, args []string) (string, error) {
	t.Helper()

	origLister := newFieldLister
	newFieldLister = func(_ *config.ResolvedAuth) fieldLister { return mock }
	t.Cleanup(func() { newFieldLister = origLister })

	buf := &bytes.Buffer{}
	cmd := newListCmd()
	cmd.SetOut(buf)
	cmd.SetErr(buf)
	cmd.PersistentFlags().String("token", "test-token", "")
	cmd.PersistentFlags().String("org-id", "test-org", "")
	cmd.PersistentFlags().String("org-type", "360", "")
	cmd.SetArgs(args)
	err := cmd.Execute()
	return buf.String(), err
}

func TestList(t *testing.T) {
	tests := []struct {
		name  string
		mock  *mockFieldLister
		args  []string
		setup func()
		check func(t *testing.T, mock *mockFieldLister, out string, err error)
	}{
		{
			name: "global table output",
			mock: &mockFieldLister{
				fields: []*tracker.Field{
					{
						Key:      testutil.StrPtr("summary"),
						Name:     testutil.StrPtr("Summary"),
						Schema:   &tracker.FieldSchema{Type: testutil.StrPtr("string")},
						Readonly: testutil.BoolPtr(true),
					},
					{
						Key:      testutil.StrPtr("tags"),
						Name:     testutil.StrPtr("Tags"),
						Schema:   &tracker.FieldSchema{Type: testutil.StrPtr("array")},
						Readonly: testutil.BoolPtr(false),
					},
				},
			},
			args: nil,
			check: func(t *testing.T, mock *mockFieldLister, out string, err error) {
				t.Helper()
				if err != nil {
					t.Fatalf("unexpected error: %v", err)
				}
				if !mock.calledGlobal {
					t.Error("expected global List to be called")
				}
				for _, want := range []string{
					"KEY", "NAME", "SCHEMA", "READONLY",
					"summary", "Summary", "string", "yes",
					"tags", "Tags", "array", "no",
				} {
					if !strings.Contains(out, want) {
						t.Errorf("table output missing %q; got:\n%s", want, out)
					}
				}
			},
		},
		{
			name: "local table output with --queue",
			mock: &mockFieldLister{
				fields: []*tracker.Field{
					{
						Key:      testutil.StrPtr("local-field"),
						Name:     testutil.StrPtr("Local Field"),
						Schema:   &tracker.FieldSchema{Type: testutil.StrPtr("string")},
						Readonly: testutil.BoolPtr(false),
					},
				},
			},
			args: []string{"--queue", "PROJ"},
			check: func(t *testing.T, mock *mockFieldLister, out string, err error) {
				t.Helper()
				if err != nil {
					t.Fatalf("unexpected error: %v", err)
				}
				if !mock.calledLocal {
					t.Error("expected local ListLocal to be called")
				}
				if mock.gotQueueKey != "PROJ" {
					t.Errorf("expected queue key PROJ, got %q", mock.gotQueueKey)
				}
				if !strings.Contains(out, "local-field") {
					t.Errorf("output missing local-field; got:\n%s", out)
				}
			},
		},
		{
			name: "json output",
			mock: &mockFieldLister{
				fields: []*tracker.Field{
					{
						Key:      testutil.StrPtr("summary"),
						Name:     testutil.StrPtr("Summary"),
						Schema:   &tracker.FieldSchema{Type: testutil.StrPtr("string")},
						Readonly: testutil.BoolPtr(true),
					},
				},
			},
			args:  nil,
			setup: func() { output.JSONFields = FieldListFields },
			check: func(t *testing.T, _ *mockFieldLister, out string, err error) {
				t.Helper()
				if err != nil {
					t.Fatalf("unexpected error: %v", err)
				}
				var items []map[string]any
				if jsonErr := json.Unmarshal([]byte(out), &items); jsonErr != nil {
					t.Fatalf("invalid JSON: %v\nraw: %s", jsonErr, out)
				}
				if len(items) != 1 {
					t.Fatalf("expected 1 item, got %d", len(items))
				}
				if items[0]["key"] != "summary" {
					t.Errorf("expected key=summary, got %v", items[0]["key"])
				}
				if items[0]["name"] != "Summary" {
					t.Errorf("expected name=Summary, got %v", items[0]["name"])
				}
				if items[0]["schema"] != "string" {
					t.Errorf("expected schema=string, got %v", items[0]["schema"])
				}
				if items[0]["readonly"] != true {
					t.Errorf("expected readonly=true, got %v", items[0]["readonly"])
				}
			},
		},
		{
			name: "quiet output",
			mock: &mockFieldLister{
				fields: []*tracker.Field{
					{Key: testutil.StrPtr("summary")},
					{Key: testutil.StrPtr("description")},
				},
			},
			args:  nil,
			setup: func() { output.QuietFlag = true },
			check: func(t *testing.T, _ *mockFieldLister, out string, err error) {
				t.Helper()
				if err != nil {
					t.Fatalf("unexpected error: %v", err)
				}
				lines := strings.Split(strings.TrimSpace(out), "\n")
				if len(lines) != 2 {
					t.Fatalf("expected 2 lines, got %d: %v", len(lines), lines)
				}
				if lines[0] != "summary" || lines[1] != "description" {
					t.Errorf("expected summary, description; got %v", lines)
				}
			},
		},
		{
			name: "empty list",
			mock: &mockFieldLister{
				fields: []*tracker.Field{},
			},
			args: nil,
			check: func(t *testing.T, _ *mockFieldLister, out string, err error) {
				t.Helper()
				if err != nil {
					t.Fatalf("unexpected error: %v", err)
				}
				if !strings.Contains(out, "No fields found") {
					t.Errorf("expected 'No fields found', got: %s", out)
				}
			},
		},
		{
			name: "api error",
			mock: &mockFieldLister{
				err: errors.New("connection refused"),
			},
			args: nil,
			check: func(t *testing.T, _ *mockFieldLister, _ string, err error) {
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
			out, err := setupListCmd(t, tt.mock, tt.args)
			tt.check(t, tt.mock, out, err)
		})
	}
}
