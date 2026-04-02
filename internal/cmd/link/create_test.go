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

// mockLinkCreator implements linkCreator for testing.
type mockLinkCreator struct {
	link        *tracker.IssueLink
	err         error
	gotIssueKey string
	gotReq      *tracker.LinkRequest
}

func (m *mockLinkCreator) CreateLink(
	_ context.Context,
	issueKey string,
	req *tracker.LinkRequest,
) (*tracker.IssueLink, *tracker.Response, error) {
	m.gotIssueKey = issueKey
	m.gotReq = req
	if m.err != nil {
		return nil, nil, m.err
	}
	return m.link, &tracker.Response{}, nil
}

func makeCreatedLink() *tracker.IssueLink {
	return &tracker.IssueLink{
		ID:        testutil.FlexStringPtr("555"),
		Direction: testutil.StrPtr("outward"),
		Type: &tracker.IssueLinkType{
			ID:      testutil.FlexStringPtr("depends"),
			Inward:  testutil.StrPtr("depends on"),
			Outward: testutil.StrPtr("is dependency for"),
		},
		Object: &tracker.Issue{
			Key:     testutil.StrPtr("PROJ-456"),
			Summary: testutil.StrPtr("Target issue"),
		},
	}
}

func setupCreateCmd(t *testing.T, mock *mockLinkCreator, args []string) (string, error) {
	t.Helper()

	origCreator := newLinkCreator
	newLinkCreator = func(_ *config.ResolvedAuth) linkCreator {
		return mock
	}
	t.Cleanup(func() { newLinkCreator = origCreator })

	buf := &bytes.Buffer{}
	cmd := newCreateCmd()
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

func TestCreate(t *testing.T) {
	tests := []struct {
		name      string
		mock      *mockLinkCreator
		args      []string
		setup     func()
		wantOut   string
		wantErr   string
		jsonCheck func(t *testing.T, out string)
		reqCheck  func(t *testing.T, mock *mockLinkCreator)
	}{
		{
			name:    "table output",
			mock:    &mockLinkCreator{link: makeCreatedLink()},
			args:    []string{"PROJ-123", "--type", "depends on", "--issue", "PROJ-456"},
			wantOut: "Link 555 created on PROJ-123",
		},
		{
			name: "json output",
			mock: &mockLinkCreator{link: makeCreatedLink()},
			args: []string{"PROJ-123", "--type", "depends on", "--issue", "PROJ-456"},
			setup: func() {
				output.JSONFields = LinkListFields
			},
			jsonCheck: func(t *testing.T, out string) {
				t.Helper()
				var item map[string]any
				if err := json.Unmarshal([]byte(out), &item); err != nil {
					t.Fatalf("invalid JSON: %v\nraw: %s", err, out)
				}
				if item["id"] != "555" {
					t.Errorf("expected id=555, got %v", item["id"])
				}
				if item["type"] != "is dependency for" {
					t.Errorf("expected type='is dependency for', got %v", item["type"])
				}
				if item["issue"] != "PROJ-456" {
					t.Errorf("expected issue=PROJ-456, got %v", item["issue"])
				}
			},
		},
		{
			name: "quiet output",
			mock: &mockLinkCreator{link: makeCreatedLink()},
			args: []string{"PROJ-123", "--type", "depends on", "--issue", "PROJ-456"},
			setup: func() {
				output.QuietFlag = true
			},
			wantOut: "555",
		},
		{
			name:    "from-json",
			mock:    &mockLinkCreator{link: makeCreatedLink()},
			args:    []string{"PROJ-123", "--from-json", `{"relationship":"relates","issue":"PROJ-456"}`},
			wantOut: "Link 555 created on PROJ-123",
			reqCheck: func(t *testing.T, mock *mockLinkCreator) {
				t.Helper()
				if mock.gotReq == nil {
					t.Fatal("expected request, got nil")
				}
				if mock.gotReq.Relationship == nil || *mock.gotReq.Relationship != "relates" {
					t.Errorf("expected relationship=relates, got %v", mock.gotReq.Relationship)
				}
				if mock.gotReq.Issue == nil || *mock.gotReq.Issue != "PROJ-456" {
					t.Errorf("expected issue=PROJ-456, got %v", mock.gotReq.Issue)
				}
			},
		},
		{
			name:    "mutual exclusion --type and --from-json",
			mock:    &mockLinkCreator{},
			args:    []string{"PROJ-123", "--type", "relates", "--from-json", `{"relationship":"relates"}`},
			wantErr: "cannot use --type/--issue and --from-json together",
		},
		{
			name:    "mutual exclusion --issue and --from-json",
			mock:    &mockLinkCreator{},
			args:    []string{"PROJ-123", "--issue", "PROJ-456", "--from-json", `{"relationship":"relates"}`},
			wantErr: "cannot use --type/--issue and --from-json together",
		},
		{
			name:    "missing --type",
			mock:    &mockLinkCreator{},
			args:    []string{"PROJ-123", "--issue", "PROJ-456"},
			wantErr: "both --type and --issue are required",
		},
		{
			name:    "missing --issue",
			mock:    &mockLinkCreator{},
			args:    []string{"PROJ-123", "--type", "relates"},
			wantErr: "both --type and --issue are required",
		},
		{
			name:    "missing both flags",
			mock:    &mockLinkCreator{},
			args:    []string{"PROJ-123"},
			wantErr: "both --type and --issue are required",
		},
		{
			name:    "invalid issue key",
			mock:    &mockLinkCreator{},
			args:    []string{"bad-key", "--type", "relates", "--issue", "PROJ-456"},
			wantErr: "invalid issue key",
		},
		{
			name:    "invalid target issue key",
			mock:    &mockLinkCreator{},
			args:    []string{"PROJ-123", "--type", "relates", "--issue", "bad-key"},
			wantErr: "invalid issue key",
		},
		{
			name:    "api error",
			mock:    &mockLinkCreator{err: errors.New("connection refused")},
			args:    []string{"PROJ-123", "--type", "relates", "--issue", "PROJ-456"},
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

			if tt.reqCheck != nil {
				tt.reqCheck(t, tt.mock)
			}

			if tt.wantOut != "" && !strings.Contains(out, tt.wantOut) {
				t.Errorf("expected output containing %q, got: %s", tt.wantOut, out)
			}
		})
	}
}

func TestCreateRequestCapture(t *testing.T) {
	testutil.ResetOutputFlags(t)

	mock := &mockLinkCreator{link: makeCreatedLink()}
	_, err := setupCreateCmd(t, mock, []string{"PROJ-123", "--type", "depends on", "--issue", "PROJ-456"})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if mock.gotIssueKey != "PROJ-123" {
		t.Errorf("expected issueKey=PROJ-123, got %q", mock.gotIssueKey)
	}
	if mock.gotReq == nil {
		t.Fatal("expected request, got nil")
	}
	if mock.gotReq.Relationship == nil || *mock.gotReq.Relationship != "depends on" {
		t.Errorf("expected relationship='depends on', got %v", mock.gotReq.Relationship)
	}
	if mock.gotReq.Issue == nil || *mock.gotReq.Issue != "PROJ-456" {
		t.Errorf("expected issue=PROJ-456, got %v", mock.gotReq.Issue)
	}
}

func TestCreateRegistered(t *testing.T) {
	cmd := NewCmd()

	found := false
	for _, sub := range cmd.Commands() {
		if sub.Name() == "create" {
			found = true
			break
		}
	}
	if !found {
		t.Error("'create' not registered as subcommand of 'link'")
	}
}
