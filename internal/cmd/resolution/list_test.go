package resolution

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

// mockResolutionLister implements resolutionLister for testing.
type mockResolutionLister struct {
	resolutions []*tracker.Resolution
	resp        *tracker.Response
	err         error
}

func (m *mockResolutionLister) List(_ context.Context) ([]*tracker.Resolution, *tracker.Response, error) {
	if m.err != nil {
		return nil, nil, m.err
	}
	return m.resolutions, m.resp, nil
}

func setupListCmd(t *testing.T, mock *mockResolutionLister, args []string) (string, error) {
	t.Helper()

	origLister := newResolutionLister
	newResolutionLister = func(_ *config.ResolvedAuth) resolutionLister { return mock }
	t.Cleanup(func() { newResolutionLister = origLister })

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
		mock  *mockResolutionLister
		args  []string
		setup func()
		check func(t *testing.T, out string, err error)
	}{
		{
			name: "table output",
			mock: &mockResolutionLister{
				resolutions: []*tracker.Resolution{
					{
						ID:   testutil.FlexStringPtr("1"),
						Key:  testutil.StrPtr("fixed"),
						Name: testutil.StrPtr("Fixed"),
					},
					{
						ID:   testutil.FlexStringPtr("2"),
						Key:  testutil.StrPtr("wontFix"),
						Name: testutil.StrPtr("Won't Fix"),
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
				for _, want := range []string{"ID", "KEY", "NAME", "1", "fixed", "Fixed", "2", "wontFix", "Won't Fix"} {
					if !strings.Contains(out, want) {
						t.Errorf("table output missing %q; got:\n%s", want, out)
					}
				}
			},
		},
		{
			name: "json output",
			mock: &mockResolutionLister{
				resolutions: []*tracker.Resolution{
					{
						ID:   testutil.FlexStringPtr("1"),
						Key:  testutil.StrPtr("fixed"),
						Name: testutil.StrPtr("Fixed"),
					},
				},
				resp: &tracker.Response{},
			},
			args:  nil,
			setup: func() { output.JSONFields = ResolutionListFields },
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
				if items[0]["key"] != "fixed" {
					t.Errorf("expected key=fixed, got %v", items[0]["key"])
				}
				if items[0]["name"] != "Fixed" {
					t.Errorf("expected name=Fixed, got %v", items[0]["name"])
				}
			},
		},
		{
			name: "quiet output",
			mock: &mockResolutionLister{
				resolutions: []*tracker.Resolution{
					{Key: testutil.StrPtr("fixed")},
					{Key: testutil.StrPtr("wontFix")},
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
				if lines[0] != "fixed" || lines[1] != "wontFix" {
					t.Errorf("expected fixed, wontFix; got %v", lines)
				}
			},
		},
		{
			name: "empty result",
			mock: &mockResolutionLister{
				resolutions: []*tracker.Resolution{},
				resp:        &tracker.Response{},
			},
			args: nil,
			check: func(t *testing.T, out string, err error) {
				t.Helper()
				if err != nil {
					t.Fatalf("unexpected error: %v", err)
				}
				if !strings.Contains(out, "No resolutions found") {
					t.Errorf("expected 'No resolutions found', got: %s", out)
				}
			},
		},
		{
			name: "api error",
			mock: &mockResolutionLister{
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
			mock: &mockResolutionLister{
				resolutions: []*tracker.Resolution{
					{
						ID:   testutil.FlexStringPtr("1"),
						Key:  testutil.StrPtr("fixed"),
						Name: testutil.StrPtr("Fixed"),
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
				if trimmed != "fixed" {
					t.Errorf("expected 'fixed', got %q", trimmed)
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
