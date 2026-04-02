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

// mockComponentCreator implements componentCreator for testing.
type mockComponentCreator struct {
	component *tracker.Component
	err       error
	gotReq    *tracker.ComponentRequest
}

func (m *mockComponentCreator) Create(
	_ context.Context,
	req *tracker.ComponentRequest,
) (*tracker.Component, *tracker.Response, error) {
	m.gotReq = req
	if m.err != nil {
		return nil, nil, m.err
	}
	return m.component, &tracker.Response{}, nil
}

func setupCreateCmd(t *testing.T, mock *mockComponentCreator, args []string) (string, error) {
	t.Helper()

	origCreator := newComponentCreator
	newComponentCreator = func(_ *config.ResolvedAuth) componentCreator { return mock }
	t.Cleanup(func() { newComponentCreator = origCreator })

	buf := &bytes.Buffer{}
	cmd := newCreateCmd()
	cmd.SetOut(buf)
	cmd.SetErr(buf)
	cmd.PersistentFlags().String("token", "test-token", "")
	cmd.PersistentFlags().String("org-id", "test-org", "")
	cmd.PersistentFlags().String("org-type", "360", "")
	cmd.SetArgs(args)
	err := cmd.Execute()
	return buf.String(), err
}

func makeCreatedComponent() *tracker.Component {
	return &tracker.Component{
		ID:   testutil.FlexStringPtr("10"),
		Name: testutil.StrPtr("Backend"),
		Queue: &tracker.Queue{
			Key: testutil.StrPtr("PROJ"),
		},
		Lead: &tracker.User{
			Display: testutil.StrPtr("John Doe"),
		},
		Description: testutil.StrPtr("Backend services"),
		AssignAuto:  testutil.BoolPtr(true),
	}
}

func TestCreate(t *testing.T) {
	tests := []struct {
		name      string
		mock      *mockComponentCreator
		args      []string
		setup     func()
		wantOut   string
		wantErr   string
		jsonCheck func(t *testing.T, out string)
	}{
		{
			name:    "table output with required flags",
			mock:    &mockComponentCreator{component: makeCreatedComponent()},
			args:    []string{"--name", "Backend", "--queue", "PROJ"},
			wantOut: "Component 10 created",
		},
		{
			name: "all flags",
			mock: &mockComponentCreator{component: makeCreatedComponent()},
			args: []string{
				"--name", "Backend", "--queue", "PROJ",
				"--description", "Backend services",
				"--lead", "12345",
				"--assign-auto",
			},
			wantOut: "Component 10 created",
		},
		{
			name:    "from-json input",
			mock:    &mockComponentCreator{component: makeCreatedComponent()},
			args:    []string{"--from-json", `{"name":"Backend","queue":"PROJ"}`},
			wantOut: "Component 10 created",
		},
		{
			name:    "mutual exclusion error",
			mock:    &mockComponentCreator{},
			args:    []string{"--name", "Test", "--from-json", `{"name":"Test"}`},
			wantErr: "cannot use individual flags and --from-json together",
		},
		{
			name:    "missing required name",
			mock:    &mockComponentCreator{},
			args:    []string{"--queue", "PROJ"},
			wantErr: "--name and --queue are required",
		},
		{
			name:    "missing required queue",
			mock:    &mockComponentCreator{},
			args:    []string{"--name", "Test"},
			wantErr: "--name and --queue are required",
		},
		{
			name:    "missing both required flags",
			mock:    &mockComponentCreator{},
			args:    nil,
			wantErr: "--name and --queue are required",
		},
		{
			name:  "json output",
			mock:  &mockComponentCreator{component: makeCreatedComponent()},
			args:  []string{"--name", "Backend", "--queue", "PROJ"},
			setup: func() { output.JSONFields = ComponentListFields },
			jsonCheck: func(t *testing.T, out string) {
				t.Helper()
				var item map[string]any
				if err := json.Unmarshal([]byte(out), &item); err != nil {
					t.Fatalf("invalid JSON: %v\nraw: %s", err, out)
				}
				if item["id"] != "10" {
					t.Errorf("expected id=10, got %v", item["id"])
				}
				if item["name"] != "Backend" {
					t.Errorf("expected name=Backend, got %v", item["name"])
				}
				if item["queue"] != "PROJ" {
					t.Errorf("expected queue=PROJ, got %v", item["queue"])
				}
			},
		},
		{
			name:    "quiet output",
			mock:    &mockComponentCreator{component: makeCreatedComponent()},
			args:    []string{"--name", "Backend", "--queue", "PROJ"},
			setup:   func() { output.QuietFlag = true },
			wantOut: "10",
		},
		{
			name:    "api error",
			mock:    &mockComponentCreator{err: errors.New("connection refused")},
			args:    []string{"--name", "Test", "--queue", "PROJ"},
			wantErr: "API request failed",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			testutil.ResetOutputFlags(t)
			if tt.setup != nil {
				tt.setup()
			}

			out, err := setupCreateCmd(t, tt.mock, tt.args)

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

			if tt.wantOut != "" && !strings.Contains(out, tt.wantOut) {
				t.Errorf("expected output containing %q, got: %s", tt.wantOut, out)
			}
		})
	}
}

func TestCreateRequestCapture(t *testing.T) {
	testutil.ResetOutputFlags(t)

	mock := &mockComponentCreator{component: makeCreatedComponent()}
	_, err := setupCreateCmd(t, mock, []string{"--name", "Backend", "--queue", "PROJ"})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if mock.gotReq == nil {
		t.Fatal("expected request, got nil")
	}
	if mock.gotReq.Name == nil || *mock.gotReq.Name != "Backend" {
		t.Errorf("expected name=Backend, got %v", mock.gotReq.Name)
	}
	if mock.gotReq.Queue == nil || *mock.gotReq.Queue != "PROJ" {
		t.Errorf("expected queue=PROJ, got %v", mock.gotReq.Queue)
	}
	// Optional flags should not be set.
	if mock.gotReq.Description != nil {
		t.Errorf("expected description=nil, got %v", mock.gotReq.Description)
	}
	if mock.gotReq.Lead != nil {
		t.Errorf("expected lead=nil, got %v", mock.gotReq.Lead)
	}
	if mock.gotReq.AssignAuto != nil {
		t.Errorf("expected assignAuto=nil, got %v", mock.gotReq.AssignAuto)
	}
}

func TestCreateAllFlags(t *testing.T) {
	testutil.ResetOutputFlags(t)

	mock := &mockComponentCreator{component: makeCreatedComponent()}
	_, err := setupCreateCmd(t, mock, []string{
		"--name", "Backend", "--queue", "PROJ",
		"--description", "Backend services",
		"--lead", "12345",
		"--assign-auto",
	})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if mock.gotReq == nil {
		t.Fatal("expected request, got nil")
	}
	if mock.gotReq.Description == nil || *mock.gotReq.Description != "Backend services" {
		t.Errorf("expected description='Backend services', got %v", mock.gotReq.Description)
	}
	if mock.gotReq.Lead == nil || *mock.gotReq.Lead != "12345" {
		t.Errorf("expected lead=12345, got %v", mock.gotReq.Lead)
	}
	if mock.gotReq.AssignAuto == nil || !*mock.gotReq.AssignAuto {
		t.Errorf("expected assignAuto=true, got %v", mock.gotReq.AssignAuto)
	}
}

func TestCreateFromJSONRequest(t *testing.T) {
	testutil.ResetOutputFlags(t)

	mock := &mockComponentCreator{component: makeCreatedComponent()}
	_, err := setupCreateCmd(t, mock, []string{"--from-json", `{"name":"Backend","queue":"PROJ"}`})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if mock.gotReq == nil {
		t.Fatal("expected request, got nil")
	}
	if mock.gotReq.Name == nil || *mock.gotReq.Name != "Backend" {
		t.Errorf("expected name=Backend, got %v", mock.gotReq.Name)
	}
	if mock.gotReq.Queue == nil || *mock.gotReq.Queue != "PROJ" {
		t.Errorf("expected queue=PROJ, got %v", mock.gotReq.Queue)
	}
}
