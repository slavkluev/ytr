package cmd

import (
	"encoding/json"
	"net/http"
	"path/filepath"
	"reflect"
	"regexp"
	"slices"
	"strconv"
	"strings"
	"testing"

	ytrerrors "github.com/slavkluev/ytr/internal/errors"
	"github.com/slavkluev/ytr/internal/faketracker"
)

type refdataItem struct {
	ID   string `json:"id"`
	Key  string `json:"key"`
	Name string `json:"name"`
}

// refdataLeaf is one of the four reference-data list commands, which share
// every behavior and differ only in their nouns. Its fixture holds the
// leaf's items over its pages, one GET request per page Tracker returned.
type refdataLeaf struct {
	noun    string
	short   string
	path    string
	fixture string
	empty   string
	items   int
	pages   int
}

// refdataRow is one behavior every refdata leaf must show, run as prefix, the
// leaf's noun, then args. A nil exchanges means the run must send no request
// at all.
type refdataRow struct {
	name      string
	prefix    []string
	args      []string
	exchanges func(t *testing.T, leaf refdataLeaf) []faketracker.Exchange
	code      int
	check     func(t *testing.T, leaf refdataLeaf, recorded []refdataItem, res cliResult)
}

func TestRefdataList(t *testing.T) {
	leaves := []refdataLeaf{
		{"status", "List workflow statuses", "/v3/statuses", "status-list.json", "No statuses found", 106, 3},
		{"priority", "List priorities", "/v3/priorities", "priority-list.json", "No priorities found", 7, 1},
		{"resolution", "List resolutions", "/v3/resolutions", "resolution-list.json", "No resolutions found", 22, 1},
		{"issuetype", "List issue types", "/v3/issuetypes", "issuetype-list.json", "No issue types found", 29, 1},
	}

	for _, leaf := range leaves {
		pages := loadRefdataFixture(t, leaf)
		var recorded []refdataItem
		for _, page := range pages {
			recorded = append(recorded, fixtureItems(t, page.Body)...)
		}
		if len(recorded) != leaf.items || len(pages) != leaf.pages {
			t.Fatalf("%s fixture holds %d items over %d requests, want %d over %d",
				leaf.fixture, len(recorded), len(pages), leaf.items, leaf.pages)
		}

		for _, row := range refdataRows() {
			argv := slices.Concat(row.prefix, []string{leaf.noun}, row.args)

			t.Run(leaf.noun+"/"+row.name, func(t *testing.T) {
				var exchanges []faketracker.Exchange
				if row.exchanges != nil {
					exchanges = row.exchanges(t, leaf)
				}

				res := runCLI(t, exchanges, argv...)

				if res.Code != row.code {
					t.Errorf("exit = %d, want %d (stderr: %s)", res.Code, row.code, res.Stderr)
				}
				assertRefdataRequests(t, leaf, len(exchanges), res.Requests)
				row.check(t, leaf, recorded, res)
			})
		}
	}
}

func refdataRows() []refdataRow {
	return []refdataRow{
		{
			name: "JSON", args: []string{"list", "--json", "id,key,name"}, exchanges: loadRefdataFixture,
			check: func(t *testing.T, _ refdataLeaf, recorded []refdataItem, res cliResult) {
				t.Helper()
				assertEmpty(t, "stderr", res.Stderr)

				var items []refdataItem
				if err := json.Unmarshal([]byte(res.Stdout), &items); err != nil {
					t.Fatalf("stdout is not a JSON array of items: %v\n%s", err, res.Stdout)
				}
				if !slices.Equal(items, recorded) {
					t.Errorf("items = %+v,\nwant the fixture's %+v", items, recorded)
				}
				for _, item := range items {
					if item.Key == "" || item.Name == "" {
						t.Errorf("item %+v has an empty key or name", item)
					}
				}
			},
		},
		{
			name: "Table", args: []string{"list"}, exchanges: loadRefdataFixture,
			check: func(t *testing.T, _ refdataLeaf, recorded []refdataItem, res cliResult) {
				t.Helper()
				assertEmpty(t, "stderr", res.Stderr)

				want := []string{"ID\tKEY\tNAME"}
				for _, item := range recorded {
					want = append(want, item.ID+"\t"+item.Key+"\t"+item.Name)
				}
				assertLines(t, res.Stdout, want)
			},
		},
		{
			name: "JSON subset in any case", args: []string{"list", "--json", "ID,Key"}, exchanges: loadRefdataFixture,
			check: func(t *testing.T, _ refdataLeaf, recorded []refdataItem, res cliResult) {
				t.Helper()
				assertEmpty(t, "stderr", res.Stderr)

				want := make([]map[string]any, len(recorded))
				for i, item := range recorded {
					want[i] = map[string]any{"id": item.ID, "key": item.Key}
				}
				assertObjects(t, res.Stdout, want)
			},
		},
		{
			name: "Quiet", args: []string{"list", "--quiet"}, exchanges: loadRefdataFixture,
			check: expectRecordedKeys,
		},
		{
			name: "jq default", args: []string{"list", "--jq", ".[].key"}, exchanges: loadRefdataFixture,
			check: expectRecordedKeys,
		},
		{
			name: "jq whole document", args: []string{"list", "--jq", "."}, exchanges: loadRefdataFixture,
			check: func(t *testing.T, _ refdataLeaf, recorded []refdataItem, res cliResult) {
				t.Helper()
				assertEmpty(t, "stderr", res.Stderr)

				want := make([]map[string]any, len(recorded))
				for i, item := range recorded {
					want[i] = map[string]any{"id": item.ID, "key": item.Key, "name": item.Name}
				}
				assertObjects(t, res.Stdout, want)
			},
		},
		{
			name: "Empty", args: []string{"list"}, exchanges: emptyRefdataList,
			check: func(t *testing.T, leaf refdataLeaf, _ []refdataItem, res cliResult) {
				t.Helper()
				assertEmpty(t, "stderr", res.Stderr)
				assertLines(t, res.Stdout, []string{leaf.empty})
			},
		},
		{
			name: "Empty quiet", args: []string{"list", "--quiet"}, exchanges: emptyRefdataList,
			check: func(t *testing.T, _ refdataLeaf, _ []refdataItem, res cliResult) {
				t.Helper()
				assertEmpty(t, "stderr", res.Stderr)
				assertEmpty(t, "stdout", res.Stdout)
			},
		},
		{
			name: "Empty JSON", args: []string{"list", "--json", "id"}, exchanges: emptyRefdataList,
			check: func(t *testing.T, _ refdataLeaf, _ []refdataItem, res cliResult) {
				t.Helper()
				assertEmpty(t, "stderr", res.Stderr)

				if res.Stdout != "[]\n" {
					t.Errorf("stdout = %q, want an empty JSON array", res.Stdout)
				}
			},
		},
		{
			name: "Field hint", args: []string{"list", "--json="}, code: ytrerrors.ExitUserError,
			check: func(t *testing.T, leaf refdataLeaf, _ []refdataItem, res cliResult) {
				t.Helper()
				assertEmpty(t, "stdout", res.Stdout)

				if want := "Available fields for " + leaf.noun + " list:\n  id\n  key\n  name\n"; !strings.Contains(
					res.Stderr, want,
				) {
					t.Errorf("stderr = %q, want it to list the fields as %q", res.Stderr, want)
				}
			},
		},
		{
			name: "Bad field", args: []string{"list", "--json", "bogus"}, code: ytrerrors.ExitUserError,
			check: func(t *testing.T, _ refdataLeaf, _ []refdataItem, res cliResult) {
				t.Helper()
				assertEmpty(t, "stdout", res.Stdout)

				var doc errorDocument
				if err := json.Unmarshal([]byte(res.Stderr), &doc); err != nil ||
					strings.Count(res.Stderr, "\n") != 1 {
					t.Fatalf("stderr = %q, want exactly one JSON document (%v)", res.Stderr, err)
				}
				if doc.Code != ytrerrors.CodeInvalidField {
					t.Errorf("code = %q, want %q", doc.Code, ytrerrors.CodeInvalidField)
				}
			},
		},
		{
			name: "Tracker 500", args: []string{"list", "--json", "id"}, exchanges: refdataServerError,
			code: ytrerrors.ExitUserError,
			check: func(t *testing.T, leaf refdataLeaf, _ []refdataItem, res cliResult) {
				t.Helper()
				assertEmpty(t, "stdout", res.Stdout)

				doc := decodeOneJSONError(t, "ytr "+leaf.noun+" list --json id", res.Stderr)
				if !strings.Contains(doc.Message, "Internal server error") {
					t.Errorf("message = %q, want the server's text", doc.Message)
				}
			},
		},
		{
			name: "Completion", prefix: []string{"__complete"}, args: []string{"list", "--json", ""},
			check: func(t *testing.T, _ refdataLeaf, _ []refdataItem, res cliResult) {
				t.Helper()

				// Cobra ends the offers with a ":<directive>" line.
				offered, _, _ := strings.Cut(res.Stdout, "\n:")
				if got := strings.Split(offered, "\n"); !slices.Equal(got, []string{"id", "key", "name"}) {
					t.Errorf("completion offers %q, want id, key and name (stdout: %q)", got, res.Stdout)
				}
			},
		},
		{
			name: "Help", args: []string{"list", "--help"},
			check: func(t *testing.T, leaf refdataLeaf, _ []refdataItem, res cliResult) {
				t.Helper()

				for _, want := range []string{
					"JSON FIELDS\n  id, key, name\n",
					"ytr " + leaf.noun + " list --json id,key,name\n",
				} {
					if !strings.Contains(res.Stdout, want) {
						t.Errorf("stdout = %q, want it to hold %q", res.Stdout, want)
					}
				}
			},
		},
		{
			name: "Group help", args: []string{"--help"},
			check: func(t *testing.T, leaf refdataLeaf, _ []refdataItem, res cliResult) {
				t.Helper()

				listLine := regexp.MustCompile(`(?m)^  list +` + regexp.QuoteMeta(leaf.short) + `$`)
				if !listLine.MatchString(res.Stdout) {
					t.Errorf("stdout = %q, want the list command described as %q", res.Stdout, leaf.short)
				}
			},
		},
	}
}

func loadRefdataFixture(t *testing.T, leaf refdataLeaf) []faketracker.Exchange {
	t.Helper()

	return faketracker.Load(t, filepath.Join(fixtureDir, leaf.fixture))
}

func emptyRefdataList(_ *testing.T, leaf refdataLeaf) []faketracker.Exchange {
	return []faketracker.Exchange{refdataExchange(leaf, http.StatusOK, `[]`)}
}

func refdataServerError(_ *testing.T, leaf refdataLeaf) []faketracker.Exchange {
	return []faketracker.Exchange{refdataExchange(
		leaf, http.StatusInternalServerError, `{"errorMessages":["Internal server error"],"errors":{}}`,
	)}
}

func refdataExchange(leaf refdataLeaf, status int, body string) faketracker.Exchange {
	return faketracker.Exchange{
		Method: http.MethodGet,
		Path:   leaf.path,
		Status: status,
		Header: http.Header{"Content-Type": {"application/json"}},
		Body:   []byte(body),
	}
}

func expectRecordedKeys(t *testing.T, _ refdataLeaf, recorded []refdataItem, res cliResult) {
	t.Helper()
	assertEmpty(t, "stderr", res.Stderr)

	keys := make([]string, len(recorded))
	for i, item := range recorded {
		keys[i] = item.Key
	}
	assertLines(t, res.Stdout, keys)
}

// assertRefdataRequests wants one GET of leaf's path per page served: the
// first with no page parameter, then page=2, page=3 and so on.
func assertRefdataRequests(t *testing.T, leaf refdataLeaf, pages int, requests []faketracker.Request) {
	t.Helper()

	if len(requests) != pages {
		t.Fatalf("requests = %+v, want %d GET %s", requests, pages, leaf.path)
	}

	for i, req := range requests {
		wantPage := ""
		if i > 0 {
			wantPage = strconv.Itoa(i + 1)
		}
		if req.Method != http.MethodGet || req.Path != leaf.path || req.Query.Get("page") != wantPage {
			t.Errorf("request %d = %+v, want GET %s with page %q", i+1, req, leaf.path, wantPage)
		}
	}
}

func assertEmpty(t *testing.T, stream, got string) {
	t.Helper()

	if got != "" {
		t.Errorf("%s = %q, want empty", stream, got)
	}
}

func assertObjects(t *testing.T, stdout string, want []map[string]any) {
	t.Helper()

	var got []map[string]any
	if err := json.Unmarshal([]byte(stdout), &got); err != nil {
		t.Fatalf("stdout is not a JSON array of objects: %v\n%s", err, stdout)
	}
	if !reflect.DeepEqual(got, want) {
		t.Errorf("objects = %v,\nwant %v", got, want)
	}
}

func assertLines(t *testing.T, stdout string, want []string) {
	t.Helper()

	if got := strings.Split(strings.TrimSuffix(stdout, "\n"), "\n"); !slices.Equal(got, want) {
		t.Errorf("stdout lines = %q,\nwant %q", got, want)
	}
}

// fixtureItems reads id, key and name of each object as Tracker sent them, the
// id being a JSON string or a bare number.
func fixtureItems(t *testing.T, body json.RawMessage) []refdataItem {
	t.Helper()

	var objects []struct {
		ID   json.RawMessage `json:"id"`
		Key  string          `json:"key"`
		Name string          `json:"name"`
	}
	if err := json.Unmarshal(body, &objects); err != nil {
		t.Fatalf("fixture body is not an array of objects: %v", err)
	}

	items := make([]refdataItem, len(objects))
	for i, object := range objects {
		items[i] = refdataItem{ID: string(object.ID), Key: object.Key, Name: object.Name}

		var id string
		if err := json.Unmarshal(object.ID, &id); err == nil {
			items[i].ID = id
		}
	}

	return items
}
