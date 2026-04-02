package status

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

// mockStatusLister implements statusLister for testing.
type mockStatusLister struct {
	statuses []*tracker.Status
	resp     *tracker.Response
	err      error
}

func (m *mockStatusLister) List(_ context.Context) ([]*tracker.Status, *tracker.Response, error) {
	if m.err != nil {
		return nil, nil, m.err
	}
	return m.statuses, m.resp, nil
}

func setupListCmd(t *testing.T, mock *mockStatusLister, args []string) (string, error) {
	t.Helper()

	origLister := newStatusLister
	newStatusLister = func(_ *config.ResolvedAuth) statusLister { return mock }
	t.Cleanup(func() { newStatusLister = origLister })

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
		mock  *mockStatusLister
		args  []string
		setup func()
		check func(t *testing.T, out string, err error)
	}{
		{
			name: "table output",
			mock: &mockStatusLister{
				statuses: []*tracker.Status{
					{ID: testutil.FlexStringPtr("1"), Key: testutil.StrPtr("open"), Name: testutil.StrPtr("Open")},
					{ID: testutil.FlexStringPtr("2"), Key: testutil.StrPtr("closed"), Name: testutil.StrPtr("Closed")},
				},
				resp: &tracker.Response{},
			},
			args: nil,
			check: func(t *testing.T, out string, err error) {
				t.Helper()
				if err != nil {
					t.Fatalf("unexpected error: %v", err)
				}
				for _, want := range []string{"ID", "KEY", "NAME", "1", "open", "Open", "2", "closed", "Closed"} {
					if !strings.Contains(out, want) {
						t.Errorf("table output missing %q; got:\n%s", want, out)
					}
				}
			},
		},
		{
			name: "json output",
			mock: &mockStatusLister{
				statuses: []*tracker.Status{
					{ID: testutil.FlexStringPtr("1"), Key: testutil.StrPtr("open"), Name: testutil.StrPtr("Open")},
				},
				resp: &tracker.Response{},
			},
			args:  nil,
			setup: func() { output.JSONFields = StatusListFields },
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
				if items[0]["id"] != "1" {
					t.Errorf("expected id=1, got %v", items[0]["id"])
				}
				if items[0]["key"] != "open" {
					t.Errorf("expected key=open, got %v", items[0]["key"])
				}
				if items[0]["name"] != "Open" {
					t.Errorf("expected name=Open, got %v", items[0]["name"])
				}
			},
		},
		{
			name: "quiet output",
			mock: &mockStatusLister{
				statuses: []*tracker.Status{
					{Key: testutil.StrPtr("open")},
					{Key: testutil.StrPtr("closed")},
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
				if lines[0] != "open" || lines[1] != "closed" {
					t.Errorf("expected open, closed; got %v", lines)
				}
			},
		},
		{
			name: "empty result",
			mock: &mockStatusLister{
				statuses: []*tracker.Status{},
				resp:     &tracker.Response{},
			},
			args: nil,
			check: func(t *testing.T, out string, err error) {
				t.Helper()
				if err != nil {
					t.Fatalf("unexpected error: %v", err)
				}
				if !strings.Contains(out, "No statuses found") {
					t.Errorf("expected 'No statuses found', got: %s", out)
				}
			},
		},
		{
			name: "api error",
			mock: &mockStatusLister{
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
			mock: &mockStatusLister{
				statuses: []*tracker.Status{
					{ID: testutil.FlexStringPtr("1"), Key: testutil.StrPtr("open"), Name: testutil.StrPtr("Open")},
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
				if trimmed != "open" {
					t.Errorf("expected 'open', got %q", trimmed)
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
