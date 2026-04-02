package priority

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

// mockPriorityLister implements priorityLister for testing.
type mockPriorityLister struct {
	priorities []*tracker.Priority
	resp       *tracker.Response
	err        error
}

func (m *mockPriorityLister) List(
	_ context.Context, _ *tracker.PriorityListOptions,
) ([]*tracker.Priority, *tracker.Response, error) {
	if m.err != nil {
		return nil, nil, m.err
	}
	return m.priorities, m.resp, nil
}

func setupListCmd(t *testing.T, mock *mockPriorityLister, args []string) (string, error) {
	t.Helper()

	origLister := newPriorityLister
	newPriorityLister = func(_ *config.ResolvedAuth) priorityLister { return mock }
	t.Cleanup(func() { newPriorityLister = origLister })

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
		mock  *mockPriorityLister
		args  []string
		setup func()
		check func(t *testing.T, out string, err error)
	}{
		{
			name: "table output",
			mock: &mockPriorityLister{
				priorities: []*tracker.Priority{
					{
						ID:   testutil.FlexStringPtr("1"),
						Key:  testutil.StrPtr("critical"),
						Name: testutil.StrPtr("Critical"),
					},
					{
						ID:   testutil.FlexStringPtr("2"),
						Key:  testutil.StrPtr("normal"),
						Name: testutil.StrPtr("Normal"),
					},
				},
				resp: &tracker.Response{},
			},
			args: nil,
			check: func(t *testing.T, out string, err error) {
				t.Helper()
				if err != nil {
					t.Fatalf("unexpected error: %v", err)
				}
				for _, want := range []string{"ID", "KEY", "NAME", "1", "critical", "Critical", "2", "normal", "Normal"} {
					if !strings.Contains(out, want) {
						t.Errorf("table output missing %q; got:\n%s", want, out)
					}
				}
			},
		},
		{
			name: "json output",
			mock: &mockPriorityLister{
				priorities: []*tracker.Priority{
					{
						ID:   testutil.FlexStringPtr("3"),
						Key:  testutil.StrPtr("low"),
						Name: testutil.StrPtr("Low"),
					},
				},
				resp: &tracker.Response{},
			},
			args:  nil,
			setup: func() { output.JSONFields = PriorityListFields },
			check: func(t *testing.T, out string, err error) {
				t.Helper()
				if err != nil {
					t.Fatalf("unexpected error: %v", err)
				}
				var items []map[string]any
				if err := json.Unmarshal([]byte(out), &items); err != nil {
					t.Fatalf("invalid JSON: %v\nraw: %s", err, out)
				}
				if len(items) != 1 {
					t.Fatalf("expected 1 item, got %d", len(items))
				}
				if items[0]["id"] != "3" {
					t.Errorf("expected id=3, got %v", items[0]["id"])
				}
				if items[0]["key"] != "low" {
					t.Errorf("expected key=low, got %v", items[0]["key"])
				}
				if items[0]["name"] != "Low" {
					t.Errorf("expected name=Low, got %v", items[0]["name"])
				}
			},
		},
		{
			name: "quiet output",
			mock: &mockPriorityLister{
				priorities: []*tracker.Priority{
					{Key: testutil.StrPtr("critical")},
					{Key: testutil.StrPtr("normal")},
				},
				resp: &tracker.Response{},
			},
			args:  nil,
			setup: func() { output.QuietFlag = true },
			check: func(t *testing.T, out string, err error) {
				t.Helper()
				if err != nil {
					t.Fatalf("unexpected error: %v", err)
				}
				lines := strings.Split(strings.TrimSpace(out), "\n")
				if len(lines) != 2 {
					t.Fatalf("expected 2 lines, got %d: %v", len(lines), lines)
				}
				if lines[0] != "critical" || lines[1] != "normal" {
					t.Errorf("expected critical, normal; got %v", lines)
				}
			},
		},
		{
			name: "empty result",
			mock: &mockPriorityLister{
				priorities: []*tracker.Priority{},
				resp:       &tracker.Response{},
			},
			args: nil,
			check: func(t *testing.T, out string, err error) {
				t.Helper()
				if err != nil {
					t.Fatalf("unexpected error: %v", err)
				}
				if !strings.Contains(out, "No priorities found") {
					t.Errorf("expected 'No priorities found', got: %s", out)
				}
			},
		},
		{
			name: "api error",
			mock: &mockPriorityLister{
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
		{
			name: "jq filter",
			mock: &mockPriorityLister{
				priorities: []*tracker.Priority{
					{
						ID:   testutil.FlexStringPtr("1"),
						Key:  testutil.StrPtr("critical"),
						Name: testutil.StrPtr("Critical"),
					},
				},
				resp: &tracker.Response{},
			},
			args:  nil,
			setup: func() { output.JQFilter = ".[0].key" },
			check: func(t *testing.T, out string, err error) {
				t.Helper()
				if err != nil {
					t.Fatalf("unexpected error: %v", err)
				}
				trimmed := strings.TrimSpace(out)
				if trimmed != "critical" {
					t.Errorf("expected 'critical', got %q", trimmed)
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
