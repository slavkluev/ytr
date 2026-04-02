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

// mockComponentGetter implements componentGetter for testing.
type mockComponentGetter struct {
	component *tracker.Component
	err       error
	gotID     string
}

func (m *mockComponentGetter) Get(
	_ context.Context,
	componentID string,
) (*tracker.Component, *tracker.Response, error) {
	m.gotID = componentID
	if m.err != nil {
		return nil, nil, m.err
	}
	return m.component, nil, nil
}

func setupGetCmd(t *testing.T, mock *mockComponentGetter, args []string) (string, error) {
	t.Helper()

	origGetter := newComponentGetter
	newComponentGetter = func(_ *config.ResolvedAuth) componentGetter { return mock }
	t.Cleanup(func() { newComponentGetter = origGetter })

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
		mock  *mockComponentGetter
		args  []string
		setup func()
		check func(t *testing.T, mock *mockComponentGetter, out string, err error)
	}{
		{
			name: "detail card output",
			mock: &mockComponentGetter{
				component: &tracker.Component{
					ID:          testutil.FlexStringPtr("42"),
					Name:        testutil.StrPtr("Backend"),
					Queue:       &tracker.Queue{Key: testutil.StrPtr("PROJ")},
					Lead:        &tracker.User{Display: testutil.StrPtr("John Doe")},
					Description: testutil.StrPtr("Backend services"),
					AssignAuto:  testutil.BoolPtr(true),
				},
			},
			args: []string{"42"},
			check: func(t *testing.T, mock *mockComponentGetter, out string, err error) {
				t.Helper()
				if err != nil {
					t.Fatalf("unexpected error: %v", err)
				}
				if mock.gotID != "42" {
					t.Errorf("expected componentID=42, got %q", mock.gotID)
				}
				for _, want := range []string{
					"ID:", "42",
					"Name:", "Backend",
					"Queue:", "PROJ",
					"Lead:", "John Doe",
					"Description:", "Backend services",
					"AssignAuto:", "yes",
				} {
					if !strings.Contains(out, want) {
						t.Errorf("detail output missing %q; got:\n%s", want, out)
					}
				}
			},
		},
		{
			name: "detail card without description",
			mock: &mockComponentGetter{
				component: &tracker.Component{
					ID:         testutil.FlexStringPtr("10"),
					Name:       testutil.StrPtr("Simple"),
					Queue:      &tracker.Queue{Key: testutil.StrPtr("TEST")},
					AssignAuto: testutil.BoolPtr(false),
				},
			},
			args: []string{"10"},
			check: func(t *testing.T, _ *mockComponentGetter, out string, err error) {
				t.Helper()
				if err != nil {
					t.Fatalf("unexpected error: %v", err)
				}
				if strings.Contains(out, "Description:") {
					t.Errorf("output should not contain Description for empty value; got:\n%s", out)
				}
				if !strings.Contains(out, "AssignAuto:") || !strings.Contains(out, "no") {
					t.Errorf("expected AssignAuto: no; got:\n%s", out)
				}
			},
		},
		{
			name: "json output",
			mock: &mockComponentGetter{
				component: &tracker.Component{
					ID:          testutil.FlexStringPtr("42"),
					Name:        testutil.StrPtr("Backend"),
					Queue:       &tracker.Queue{Key: testutil.StrPtr("PROJ")},
					Lead:        &tracker.User{Display: testutil.StrPtr("John Doe")},
					Description: testutil.StrPtr("Backend services"),
					AssignAuto:  testutil.BoolPtr(true),
				},
			},
			args:  []string{"42"},
			setup: func() { output.JSONFields = ComponentGetFields },
			check: func(t *testing.T, _ *mockComponentGetter, out string, err error) {
				t.Helper()
				if err != nil {
					t.Fatalf("unexpected error: %v", err)
				}
				var result map[string]any
				if jsonErr := json.Unmarshal([]byte(out), &result); jsonErr != nil {
					t.Fatalf("invalid JSON: %v\nraw: %s", jsonErr, out)
				}
				if result["id"] != "42" {
					t.Errorf("expected id=42, got %v", result["id"])
				}
				if result["name"] != "Backend" {
					t.Errorf("expected name=Backend, got %v", result["name"])
				}
				if result["queue"] != "PROJ" {
					t.Errorf("expected queue=PROJ, got %v", result["queue"])
				}
				if result["lead"] != "John Doe" {
					t.Errorf("expected lead=John Doe, got %v", result["lead"])
				}
				if result["assignAuto"] != true {
					t.Errorf("expected assignAuto=true, got %v", result["assignAuto"])
				}
			},
		},
		{
			name: "quiet output",
			mock: &mockComponentGetter{
				component: &tracker.Component{
					ID: testutil.FlexStringPtr("42"),
				},
			},
			args:  []string{"42"},
			setup: func() { output.QuietFlag = true },
			check: func(t *testing.T, _ *mockComponentGetter, out string, err error) {
				t.Helper()
				if err != nil {
					t.Fatalf("unexpected error: %v", err)
				}
				trimmed := strings.TrimSpace(out)
				if trimmed != "42" {
					t.Errorf("expected '42', got %q", trimmed)
				}
			},
		},
		{
			name: "invalid component id",
			mock: &mockComponentGetter{},
			args: []string{"abc"},
			check: func(t *testing.T, _ *mockComponentGetter, _ string, err error) {
				t.Helper()
				if err == nil {
					t.Fatal("expected error, got nil")
				}
				if !strings.Contains(err.Error(), "invalid component ID") {
					t.Errorf("expected 'invalid component ID' error, got: %v", err)
				}
			},
		},
		{
			name: "nil queue and lead in response",
			mock: &mockComponentGetter{
				component: &tracker.Component{
					ID:   testutil.FlexStringPtr("99"),
					Name: testutil.StrPtr("Orphan"),
				},
			},
			args: []string{"99"},
			check: func(t *testing.T, _ *mockComponentGetter, out string, err error) {
				t.Helper()
				if err != nil {
					t.Fatalf("unexpected error: %v", err)
				}
				if !strings.Contains(out, "Queue:") && !strings.Contains(out, "-") {
					t.Errorf("expected '-' for nil queue; got:\n%s", out)
				}
				if !strings.Contains(out, "Lead:") && !strings.Contains(out, "-") {
					t.Errorf("expected '-' for nil lead; got:\n%s", out)
				}
			},
		},
		{
			name: "api error",
			mock: &mockComponentGetter{
				err: errors.New("connection refused"),
			},
			args: []string{"42"},
			check: func(t *testing.T, _ *mockComponentGetter, _ string, err error) {
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
