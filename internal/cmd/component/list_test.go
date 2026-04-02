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

// mockComponentLister implements componentLister for testing.
type mockComponentLister struct {
	components []*tracker.Component
	err        error
}

func (m *mockComponentLister) List(_ context.Context) ([]*tracker.Component, *tracker.Response, error) {
	if m.err != nil {
		return nil, nil, m.err
	}
	return m.components, nil, nil
}

func setupListCmd(t *testing.T, mock *mockComponentLister, args []string) (string, error) {
	t.Helper()

	origLister := newComponentLister
	newComponentLister = func(_ *config.ResolvedAuth) componentLister { return mock }
	t.Cleanup(func() { newComponentLister = origLister })

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
		mock  *mockComponentLister
		args  []string
		setup func()
		check func(t *testing.T, out string, err error)
	}{
		{
			name: "table output",
			mock: &mockComponentLister{
				components: []*tracker.Component{
					{
						ID:   testutil.FlexStringPtr("1"),
						Name: testutil.StrPtr("Backend"),
						Queue: &tracker.Queue{
							Key: testutil.StrPtr("PROJ"),
						},
						Lead: &tracker.User{
							Display: testutil.StrPtr("John Doe"),
						},
					},
					{
						ID:   testutil.FlexStringPtr("2"),
						Name: testutil.StrPtr("Frontend"),
						Queue: &tracker.Queue{
							Key: testutil.StrPtr("WEB"),
						},
						Lead: &tracker.User{
							Display: testutil.StrPtr("Jane Smith"),
						},
					},
				},
			},
			args: nil,
			check: func(t *testing.T, out string, err error) {
				t.Helper()
				if err != nil {
					t.Fatalf("unexpected error: %v", err)
				}
				for _, want := range []string{
					"ID", "NAME", "QUEUE", "LEAD",
					"1", "Backend", "PROJ", "John Doe",
					"2", "Frontend", "WEB", "Jane Smith",
				} {
					if !strings.Contains(out, want) {
						t.Errorf("table output missing %q; got:\n%s", want, out)
					}
				}
			},
		},
		{
			name: "json output",
			mock: &mockComponentLister{
				components: []*tracker.Component{
					{
						ID:   testutil.FlexStringPtr("1"),
						Name: testutil.StrPtr("Backend"),
						Queue: &tracker.Queue{
							Key: testutil.StrPtr("PROJ"),
						},
						Lead: &tracker.User{
							Display: testutil.StrPtr("John Doe"),
						},
						Description: testutil.StrPtr("Backend services"),
						AssignAuto:  testutil.BoolPtr(true),
					},
				},
			},
			args:  nil,
			setup: func() { output.JSONFields = ComponentListFields },
			check: func(t *testing.T, out string, err error) {
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
				if items[0]["id"] != "1" {
					t.Errorf("expected id=1, got %v", items[0]["id"])
				}
				if items[0]["name"] != "Backend" {
					t.Errorf("expected name=Backend, got %v", items[0]["name"])
				}
				if items[0]["queue"] != "PROJ" {
					t.Errorf("expected queue=PROJ, got %v", items[0]["queue"])
				}
				if items[0]["lead"] != "John Doe" {
					t.Errorf("expected lead=John Doe, got %v", items[0]["lead"])
				}
				if items[0]["description"] != "Backend services" {
					t.Errorf("expected description='Backend services', got %v", items[0]["description"])
				}
				if items[0]["assignAuto"] != true {
					t.Errorf("expected assignAuto=true, got %v", items[0]["assignAuto"])
				}
			},
		},
		{
			name: "quiet output",
			mock: &mockComponentLister{
				components: []*tracker.Component{
					{ID: testutil.FlexStringPtr("1")},
					{ID: testutil.FlexStringPtr("2")},
					{ID: testutil.FlexStringPtr("3")},
				},
			},
			args:  nil,
			setup: func() { output.QuietFlag = true },
			check: func(t *testing.T, out string, err error) {
				t.Helper()
				if err != nil {
					t.Fatalf("unexpected error: %v", err)
				}
				lines := strings.Split(strings.TrimSpace(out), "\n")
				if len(lines) != 3 {
					t.Fatalf("expected 3 lines, got %d: %v", len(lines), lines)
				}
				if lines[0] != "1" || lines[1] != "2" || lines[2] != "3" {
					t.Errorf("expected 1, 2, 3; got %v", lines)
				}
			},
		},
		{
			name: "empty list",
			mock: &mockComponentLister{
				components: []*tracker.Component{},
			},
			args: nil,
			check: func(t *testing.T, out string, err error) {
				t.Helper()
				if err != nil {
					t.Fatalf("unexpected error: %v", err)
				}
				if !strings.Contains(out, "No components found") {
					t.Errorf("expected 'No components found', got: %s", out)
				}
			},
		},
		{
			name: "nil queue and lead",
			mock: &mockComponentLister{
				components: []*tracker.Component{
					{
						ID:    testutil.FlexStringPtr("5"),
						Name:  testutil.StrPtr("Orphan"),
						Queue: nil,
						Lead:  nil,
					},
				},
			},
			args: nil,
			check: func(t *testing.T, out string, err error) {
				t.Helper()
				if err != nil {
					t.Fatalf("unexpected error: %v", err)
				}
				if !strings.Contains(out, "Orphan") {
					t.Errorf("output missing component name; got:\n%s", out)
				}
				// Verify "-" fallback is used for nil queue and lead.
				lines := strings.Split(out, "\n")
				foundDash := false
				for _, line := range lines {
					if strings.Contains(line, "5") && strings.Contains(line, "-") {
						foundDash = true
						break
					}
				}
				if !foundDash {
					t.Errorf("expected '-' fallback for nil queue/lead; got:\n%s", out)
				}
			},
		},
		{
			name: "api error",
			mock: &mockComponentLister{
				err: errors.New("connection refused"),
			},
			args: nil,
			check: func(t *testing.T, _ string, err error) {
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
			tt.check(t, out, err)
		})
	}
}
