package component

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

// mockComponentEditor implements componentEditor for testing.
type mockComponentEditor struct {
	component *tracker.Component
	err       error
	gotID     string
	gotReq    *tracker.ComponentRequest
}

func (m *mockComponentEditor) Edit(
	_ context.Context,
	componentID string,
	req *tracker.ComponentRequest,
) (*tracker.Component, *tracker.Response, error) {
	m.gotID = componentID
	m.gotReq = req
	if m.err != nil {
		return nil, nil, m.err
	}
	return m.component, &tracker.Response{}, nil
}

func setupEditCmd(t *testing.T, mock *mockComponentEditor, args []string) (string, error) {
	t.Helper()

	origEditor := newComponentEditor
	newComponentEditor = func(_ *config.ResolvedAuth) componentEditor { return mock }
	t.Cleanup(func() { newComponentEditor = origEditor })

	buf := &bytes.Buffer{}
	cmd := newEditCmd()
	cmd.SetOut(buf)
	cmd.SetErr(buf)
	cmd.PersistentFlags().String("token", "test-token", "")
	cmd.PersistentFlags().String("org-id", "test-org", "")
	cmd.PersistentFlags().String("org-type", "360", "")
	cmd.SetArgs(args)
	err := cmd.Execute()
	return buf.String(), err
}

func makeEditedComponent() *tracker.Component {
	return &tracker.Component{
		ID:   testutil.FlexStringPtr("42"),
		Name: testutil.StrPtr("Updated Backend"),
		Queue: &tracker.Queue{
			Key: testutil.StrPtr("PROJ"),
		},
		Lead: &tracker.User{
			Display: testutil.StrPtr("Jane Smith"),
		},
		Description: testutil.StrPtr("Updated description"),
		AssignAuto:  testutil.BoolPtr(true),
	}
}

func TestEdit(t *testing.T) {
	tests := []struct {
		name      string
		mock      *mockComponentEditor
		args      []string
		setup     func()
		wantOut   string
		wantErr   string
		jsonCheck func(t *testing.T, out string)
		reqCheck  func(t *testing.T, mock *mockComponentEditor)
	}{
		{
			name:    "update name only",
			mock:    &mockComponentEditor{component: makeEditedComponent()},
			args:    []string{"42", "--name", "Updated Backend"},
			wantOut: "Component 42 updated",
			reqCheck: func(t *testing.T, mock *mockComponentEditor) {
				t.Helper()
				if mock.gotID != "42" {
					t.Errorf("expected componentID=42, got %q", mock.gotID)
				}
				if mock.gotReq == nil {
					t.Fatal("expected request, got nil")
				}
				if mock.gotReq.Name == nil || *mock.gotReq.Name != "Updated Backend" {
					t.Errorf("expected name='Updated Backend', got %v", mock.gotReq.Name)
				}
				// Other fields should not be set.
				if mock.gotReq.Queue != nil {
					t.Errorf("expected queue=nil, got %v", mock.gotReq.Queue)
				}
				if mock.gotReq.Lead != nil {
					t.Errorf("expected lead=nil, got %v", mock.gotReq.Lead)
				}
			},
		},
		{
			name:    "update lead only",
			mock:    &mockComponentEditor{component: makeEditedComponent()},
			args:    []string{"42", "--lead", "67890"},
			wantOut: "Component 42 updated",
			reqCheck: func(t *testing.T, mock *mockComponentEditor) {
				t.Helper()
				if mock.gotReq.Lead == nil || *mock.gotReq.Lead != "67890" {
					t.Errorf("expected lead=67890, got %v", mock.gotReq.Lead)
				}
				if mock.gotReq.Name != nil {
					t.Errorf("expected name=nil, got %v", mock.gotReq.Name)
				}
			},
		},
		{
			name:    "update assign-auto",
			mock:    &mockComponentEditor{component: makeEditedComponent()},
			args:    []string{"42", "--assign-auto"},
			wantOut: "Component 42 updated",
			reqCheck: func(t *testing.T, mock *mockComponentEditor) {
				t.Helper()
				if mock.gotReq.AssignAuto == nil || !*mock.gotReq.AssignAuto {
					t.Errorf("expected assignAuto=true, got %v", mock.gotReq.AssignAuto)
				}
			},
		},
		{
			name:    "from-json input",
			mock:    &mockComponentEditor{component: makeEditedComponent()},
			args:    []string{"42", "--from-json", `{"name":"Updated","description":"New desc"}`},
			wantOut: "Component 42 updated",
			reqCheck: func(t *testing.T, mock *mockComponentEditor) {
				t.Helper()
				if mock.gotReq == nil {
					t.Fatal("expected request, got nil")
				}
				if mock.gotReq.Name == nil || *mock.gotReq.Name != "Updated" {
					t.Errorf("expected name=Updated, got %v", mock.gotReq.Name)
				}
				if mock.gotReq.Description == nil || *mock.gotReq.Description != "New desc" {
					t.Errorf("expected description='New desc', got %v", mock.gotReq.Description)
				}
			},
		},
		{
			name:    "mutual exclusion error",
			mock:    &mockComponentEditor{},
			args:    []string{"42", "--name", "Test", "--from-json", `{"name":"Test"}`},
			wantErr: "cannot use individual flags and --from-json together",
		},
		{
			name:    "no flags error",
			mock:    &mockComponentEditor{},
			args:    []string{"42"},
			wantErr: "at least one of --name, --queue, --description, --lead, --assign-auto, or --from-json is required",
		},
		{
			name:    "invalid component id",
			mock:    &mockComponentEditor{},
			args:    []string{"abc", "--name", "Test"},
			wantErr: "invalid component ID",
		},
		{
			name:  "json output",
			mock:  &mockComponentEditor{component: makeEditedComponent()},
			args:  []string{"42", "--name", "Updated Backend"},
			setup: func() { output.JSONFields = ComponentListFields },
			jsonCheck: func(t *testing.T, out string) {
				t.Helper()
				var item map[string]any
				if err := json.Unmarshal([]byte(out), &item); err != nil {
					t.Fatalf("invalid JSON: %v\nraw: %s", err, out)
				}
				if item["id"] != "42" {
					t.Errorf("expected id=42, got %v", item["id"])
				}
				if item["name"] != "Updated Backend" {
					t.Errorf("expected name='Updated Backend', got %v", item["name"])
				}
			},
		},
		{
			name:    "quiet output",
			mock:    &mockComponentEditor{component: makeEditedComponent()},
			args:    []string{"42", "--name", "Updated"},
			setup:   func() { output.QuietFlag = true },
			wantOut: "42",
		},
		{
			name:    "api error",
			mock:    &mockComponentEditor{err: errors.New("connection refused")},
			args:    []string{"42", "--name", "Test"},
			wantErr: "API request failed",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			testutil.ResetOutputFlags(t)
			if tt.setup != nil {
				tt.setup()
			}

			out, err := setupEditCmd(t, tt.mock, tt.args)

			if tt.wantErr != "" {
				if err == nil {
					t.Fatalf("expected error containing %q, got nil", tt.wantErr)
				}
				if !strings.Contains(err.Error(), tt.wantErr) {
					t.Errorf("expected error containing %q, got: %v", tt.wantErr, err)
				}
				return
			}

			if err != nil {
				t.Fatalf("unexpected error: %v", err)
			}

			if tt.jsonCheck != nil {
				tt.jsonCheck(t, out)
				return
			}

			if tt.reqCheck != nil {
				tt.reqCheck(t, tt.mock)
			}

			if tt.wantOut != "" && !strings.Contains(out, tt.wantOut) {
				t.Errorf("expected output containing %q, got: %s", tt.wantOut, out)
			}
		})
	}
}
