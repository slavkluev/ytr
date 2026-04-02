package link

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

// mockLinkLister implements linkLister for testing.
type mockLinkLister struct {
	links       []*tracker.IssueLink
	err         error
	gotIssueKey string
}

func (m *mockLinkLister) GetLinks(
	_ context.Context,
	issueKey string,
) ([]*tracker.IssueLink, *tracker.Response, error) {
	m.gotIssueKey = issueKey
	if m.err != nil {
		return nil, nil, m.err
	}
	return m.links, &tracker.Response{}, nil
}

func makeLinks() []*tracker.IssueLink {
	return []*tracker.IssueLink{
		{
			ID:        testutil.FlexStringPtr("101"),
			Direction: testutil.StrPtr("inward"),
			Type: &tracker.IssueLinkType{
				ID:      testutil.FlexStringPtr("depends"),
				Inward:  testutil.StrPtr("depends on"),
				Outward: testutil.StrPtr("is dependency for"),
			},
			Object: &tracker.Issue{
				Key:     testutil.StrPtr("PROJ-456"),
				Summary: testutil.StrPtr("Setup database"),
			},
		},
		{
			ID:        testutil.FlexStringPtr("202"),
			Direction: testutil.StrPtr("outward"),
			Type: &tracker.IssueLinkType{
				ID:      testutil.FlexStringPtr("relates"),
				Inward:  testutil.StrPtr("is related to"),
				Outward: testutil.StrPtr("relates to"),
			},
			Object: &tracker.Issue{
				Key:     testutil.StrPtr("PROJ-789"),
				Summary: testutil.StrPtr("Add tests"),
			},
		},
	}
}

func setupListCmd(t *testing.T, mock *mockLinkLister, args []string) (string, error) {
	t.Helper()

	origLister := newLinkLister
	newLinkLister = func(_ *config.ResolvedAuth) linkLister {
		return mock
	}
	t.Cleanup(func() { newLinkLister = origLister })

	buf := &bytes.Buffer{}
	cmd := newListCmd()
	cmd.SetOut(buf)
	cmd.SetErr(buf)

	// Simulate root persistent flags for auth.
	cmd.PersistentFlags().String("token", "test-token", "")
	cmd.PersistentFlags().String("org-id", "test-org", "")
	cmd.PersistentFlags().String("org-type", "360", "")

	cmd.SetArgs(args)
	err := cmd.Execute()
	return buf.String(), err
}

func TestList(t *testing.T) {
	tests := []struct {
		name      string
		mock      *mockLinkLister
		args      []string
		setup     func()
		wantOut   string
		wantErr   string
		jsonCheck func(t *testing.T, out string)
	}{
		{
			name:    "table output",
			mock:    &mockLinkLister{links: makeLinks()},
			args:    []string{"PROJ-123"},
			wantOut: "depends on",
		},
		{
			name:    "table output has headers",
			mock:    &mockLinkLister{links: makeLinks()},
			args:    []string{"PROJ-123"},
			wantOut: "ID",
		},
		{
			name: "json output",
			mock: &mockLinkLister{links: makeLinks()},
			args: []string{"PROJ-123"},
			setup: func() {
				output.JSONFields = LinkListFields
			},
			jsonCheck: func(t *testing.T, out string) {
				t.Helper()
				var items []map[string]any
				if err := json.Unmarshal([]byte(out), &items); err != nil {
					t.Fatalf("invalid JSON: %v\nraw: %s", err, out)
				}
				if len(items) != 2 {
					t.Fatalf("expected 2 items, got %d", len(items))
				}
				if items[0]["id"] != "101" {
					t.Errorf("expected id=101, got %v", items[0]["id"])
				}
				if items[0]["type"] != "depends on" {
					t.Errorf("expected type='depends on', got %v", items[0]["type"])
				}
				if items[0]["issue"] != "PROJ-456" {
					t.Errorf("expected issue=PROJ-456, got %v", items[0]["issue"])
				}
				if items[0]["summary"] != "Setup database" {
					t.Errorf("expected summary='Setup database', got %v", items[0]["summary"])
				}
				// Outward link should show outward type.
				if items[1]["type"] != "relates to" {
					t.Errorf("expected type='relates to', got %v", items[1]["type"])
				}
			},
		},
		{
			name: "quiet output",
			mock: &mockLinkLister{links: makeLinks()},
			args: []string{"PROJ-123"},
			setup: func() {
				output.QuietFlag = true
			},
			wantOut: "101",
		},
		{
			name: "jq filter",
			mock: &mockLinkLister{links: makeLinks()},
			args: []string{"PROJ-123"},
			setup: func() {
				output.JQFilter = ".[0].type"
			},
			wantOut: "depends on",
		},
		{
			name:    "empty list",
			mock:    &mockLinkLister{links: []*tracker.IssueLink{}},
			args:    []string{"PROJ-123"},
			wantOut: "No links found",
		},
		{
			name:    "invalid issue key",
			mock:    &mockLinkLister{},
			args:    []string{"bad-key"},
			wantErr: "invalid issue key",
		},
		{
			name:    "api error",
			mock:    &mockLinkLister{err: errors.New("connection refused")},
			args:    []string{"PROJ-123"},
			wantErr: "API request failed",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			testutil.ResetOutputFlags(t)
			if tt.setup != nil {
				tt.setup()
			}

			out, err := setupListCmd(t, tt.mock, tt.args)

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

func TestListDirectionAware(t *testing.T) {
	testutil.ResetOutputFlags(t)

	mock := &mockLinkLister{links: makeLinks()}

	out, err := setupListCmd(t, mock, []string{"PROJ-123"})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	// Inward link should show "depends on".
	if !strings.Contains(out, "depends on") {
		t.Errorf("expected inward type 'depends on' in table, got:\n%s", out)
	}
	// Outward link should show "relates to".
	if !strings.Contains(out, "relates to") {
		t.Errorf("expected outward type 'relates to' in table, got:\n%s", out)
	}
}

func TestListRequestCapture(t *testing.T) {
	testutil.ResetOutputFlags(t)

	mock := &mockLinkLister{links: []*tracker.IssueLink{}}
	_, err := setupListCmd(t, mock, []string{"PROJ-123"})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if mock.gotIssueKey != "PROJ-123" {
		t.Errorf("expected issueKey=PROJ-123, got %q", mock.gotIssueKey)
	}
}

func TestListRegistered(t *testing.T) {
	cmd := NewCmd()

	found := false
	for _, sub := range cmd.Commands() {
		if sub.Name() == "list" {
			found = true
			break
		}
	}
	if !found {
		t.Error("'list' not registered as subcommand of 'link'")
	}
}
