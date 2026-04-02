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

// mockComponentDeleter implements componentDeleter for testing.
type mockComponentDeleter struct {
	err   error
	gotID string
}

func (m *mockComponentDeleter) Delete(_ context.Context, componentID string) (*tracker.Response, error) {
	m.gotID = componentID
	if m.err != nil {
		return nil, m.err
	}
	return &tracker.Response{}, nil
}

func setupDeleteCmd(t *testing.T, mock *mockComponentDeleter, args []string) (string, error) {
	t.Helper()

	origDeleter := newComponentDeleter
	newComponentDeleter = func(_ *config.ResolvedAuth) componentDeleter { return mock }
	t.Cleanup(func() { newComponentDeleter = origDeleter })

	buf := &bytes.Buffer{}
	cmd := newDeleteCmd()
	cmd.SetOut(buf)
	cmd.SetErr(buf)
	cmd.PersistentFlags().String("token", "test-token", "")
	cmd.PersistentFlags().String("org-id", "test-org", "")
	cmd.PersistentFlags().String("org-type", "360", "")
	cmd.SetArgs(args)
	err := cmd.Execute()
	return buf.String(), err
}

func TestDelete(t *testing.T) {
	tests := []struct {
		name      string
		mock      *mockComponentDeleter
		args      []string
		setup     func()
		wantOut   string
		wantErr   string
		jsonCheck func(t *testing.T, out string)
	}{
		{
			name:    "table output",
			mock:    &mockComponentDeleter{},
			args:    []string{"42"},
			wantOut: "Component 42 deleted\n",
		},
		{
			name:  "json output",
			mock:  &mockComponentDeleter{},
			args:  []string{"42"},
			setup: func() { output.JSONFields = []string{"id"} },
			jsonCheck: func(t *testing.T, out string) {
				t.Helper()
				var result map[string]any
				if err := json.Unmarshal([]byte(out), &result); err != nil {
					t.Fatalf("invalid JSON: %v\nraw: %s", err, out)
				}
				if result["id"] != "42" {
					t.Errorf("expected id=42, got %v", result["id"])
				}
				if result["deleted"] != true {
					t.Errorf("expected deleted=true, got %v", result["deleted"])
				}
			},
		},
		{
			name:    "quiet output",
			mock:    &mockComponentDeleter{},
			args:    []string{"42"},
			setup:   func() { output.QuietFlag = true },
			wantOut: "42",
		},
		{
			name:    "invalid component id",
			mock:    &mockComponentDeleter{},
			args:    []string{"abc"},
			wantErr: "invalid component ID",
		},
		{
			name:    "api error",
			mock:    &mockComponentDeleter{err: errors.New("connection refused")},
			args:    []string{"42"},
			wantErr: "API request failed",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			testutil.ResetOutputFlags(t)
			if tt.setup != nil {
				tt.setup()
			}

			out, err := setupDeleteCmd(t, tt.mock, tt.args)

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

			trimmed := strings.TrimSpace(out)
			wantTrimmed := strings.TrimSpace(tt.wantOut)
			if wantTrimmed != "" && !strings.Contains(trimmed, wantTrimmed) {
				t.Errorf("expected output containing %q, got: %q", wantTrimmed, trimmed)
			}
		})
	}
}

func TestDeleteRequestCapture(t *testing.T) {
	testutil.ResetOutputFlags(t)

	mock := &mockComponentDeleter{}

	_, err := setupDeleteCmd(t, mock, []string{"42"})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if mock.gotID != "42" {
		t.Errorf("expected componentID=42, got %q", mock.gotID)
	}
}
