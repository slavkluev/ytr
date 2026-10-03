package cmd

import (
	"bytes"
	"encoding/json"
	"net/http"
	"os"
	"path/filepath"
	"reflect"
	"slices"
	"strings"
	"testing"

	"github.com/spf13/cobra"

	"github.com/slavkluev/ytr/internal/api"
	"github.com/slavkluev/ytr/internal/config"
	"github.com/slavkluev/ytr/internal/faketracker"
	"github.com/slavkluev/ytr/internal/output"
	"github.com/slavkluev/ytr/internal/testutil"
)

const fixtureDir = "testdata/fixtures"

// The commands AGENTS.md allows against the real Tracker: a leaf with one of
// these names, or one of these whole paths.
var (
	readOnlyLeafNames = []string{"list", "view", "get", "changelog", "myself"}
	readOnlyLeafPaths = []string{"ytr bulk status", "ytr auth status"}
)

// TestRecordFixtures runs each invocation against the real Tracker and writes
// what came back as a fixture. It sends real requests, so it runs only under
// YTR_RECORD=1, and recordFixture lets only read-only commands through.
func TestRecordFixtures(t *testing.T) {
	if os.Getenv("YTR_RECORD") != "1" {
		t.Skip("set YTR_RECORD=1 to record fixtures from the real Tracker")
	}

	auth, err := config.ResolveAuth("", "", "")
	if err != nil {
		t.Fatalf("resolving credentials for the real Tracker: %v", err)
	}
	secrets := []faketracker.Secret{{Name: "token", Value: auth.Token}, {Name: "org ID", Value: auth.OrgID}}

	rows := []struct {
		fixture string
		args    []string
	}{
		{"status-list.json", []string{"status", "list"}},
		{"priority-list.json", []string{"priority", "list"}},
		{"resolution-list.json", []string{"resolution", "list"}},
		{"issuetype-list.json", []string{"issuetype", "list"}},
	}

	for _, row := range rows {
		t.Run(strings.Join(row.args, " "), func(t *testing.T) {
			recordFixture(t, http.DefaultTransport, filepath.Join(fixtureDir, row.fixture), secrets, row.args...)
		})
	}
}

// recordFixture runs a read-only invocation with credentials resolved the way
// ytr resolves them, sends its requests through base, and saves the scrubbed
// exchanges to path only when the invocation succeeded.
func recordFixture(t *testing.T, base http.RoundTripper, path string, secrets []faketracker.Secret, args ...string) {
	t.Helper()
	testutil.ResetOutputFlags(t)
	output.SetTTY(false)

	leaf, _, err := newRootCmd().Find(args)
	if err != nil || !isReadOnlyLeaf(leaf) {
		t.Fatalf("refusing to record ytr %s: only AGENTS.md's read-only commands may reach the real Tracker",
			strings.Join(args, " "))
	}

	rec := faketracker.NewRecorder(base)

	var out, errOut bytes.Buffer
	output.SetDebugWriter(&errOut)
	code := execute(api.WithTransport(t.Context(), rec), newRootCmd(), args, &out, &errOut)
	if code != 0 {
		t.Fatalf("ytr %s exited %d, nothing recorded: %s", strings.Join(args, " "), code, errOut.String())
	}

	if err := rec.Save(path, secrets...); err != nil {
		t.Fatal(err)
	}
}

func isReadOnlyLeaf(cmd *cobra.Command) bool {
	if !cmd.Runnable() || cmd.HasSubCommands() {
		return false
	}

	return slices.Contains(readOnlyLeafNames, cmd.Name()) || slices.Contains(readOnlyLeafPaths, cmd.CommandPath())
}

func TestRecordAllowsOnlyReadOnlyLeaves(t *testing.T) {
	cases := []struct {
		args []string
		want bool
	}{
		{[]string{"status", "list"}, true},
		{[]string{"issue", "view"}, true},
		{[]string{"field", "get"}, true},
		{[]string{"issue", "changelog"}, true},
		{[]string{"user", "myself"}, true},
		{[]string{"bulk", "status"}, true},
		{[]string{"auth", "status"}, true},
		{[]string{"status"}, false},
		{[]string{"issue", "create"}, false},
		{[]string{"issue", "update"}, false},
		{[]string{"issue", "transition"}, false},
		{[]string{"comment", "edit"}, false},
		{[]string{"worklog", "delete"}, false},
		{[]string{"bulk", "move"}, false},
		{[]string{"auth", "login"}, false},
		{[]string{"auth", "logout"}, false},
	}

	for _, c := range cases {
		leaf, _, err := newRootCmd().Find(c.args)
		if err != nil {
			t.Fatalf("Find(%q): %v", c.args, err)
		}
		if got := isReadOnlyLeaf(leaf); got != c.want {
			t.Errorf("isReadOnlyLeaf(ytr %s) = %v, want %v", strings.Join(c.args, " "), got, c.want)
		}
	}
}

// The record path, run against a fake instead of the real Tracker, so record
// mode cannot rot between the rare runs that reach production.
func TestRecordFixtureWritesWhatReplayServes(t *testing.T) {
	t.Setenv("YTR_CONFIG_DIR", t.TempDir())
	t.Setenv("YTR_TOKEN", "y0_record-token")
	t.Setenv("YTR_ORG_ID", "bpf-record-org")
	t.Setenv("YTR_ORG_TYPE", "cloud")

	upstream := faketracker.New(t, []faketracker.Exchange{{
		Method: http.MethodGet,
		Path:   "/v3/statuses",
		Status: http.StatusOK,
		Header: http.Header{"Content-Type": {"application/json"}, "X-Request-Id": {"req-1"}},
		Body:   []byte(`[{"self":"https://api.tracker.yandex.net/v3/statuses/1","id":1,"key":"open","name":"Open"}]`),
	}})
	path := filepath.Join(t.TempDir(), "status-list.json")
	secrets := []faketracker.Secret{
		{Name: "token", Value: "y0_record-token"},
		{Name: "org ID", Value: "bpf-record-org"},
	}

	recordFixture(t, upstream, path, secrets, "status", "list")

	if got := upstream.Requests(); len(got) != 1 || got[0].Method != http.MethodGet {
		t.Fatalf("upstream requests = %+v, want one GET", got)
	}

	res := runCLI(t, faketracker.Load(t, path), "status", "list", "--json", "id,key,name")
	if res.Code != 0 {
		t.Fatalf("replay exit = %d, stderr %q", res.Code, res.Stderr)
	}

	var items []map[string]string
	if err := json.Unmarshal([]byte(res.Stdout), &items); err != nil {
		t.Fatalf("replay stdout is not a JSON array: %v\n%s", err, res.Stdout)
	}
	want := []map[string]string{{"id": "1", "key": "open", "name": "Open"}}
	if !reflect.DeepEqual(items, want) {
		t.Errorf("replay items = %v, want %v", items, want)
	}
}
