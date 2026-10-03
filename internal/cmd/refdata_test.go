package cmd

import (
	"encoding/json"
	"net/http"
	"path/filepath"
	"slices"
	"strings"
	"testing"

	"github.com/slavkluev/ytr/internal/faketracker"
)

type refdataItem struct {
	ID   string `json:"id"`
	Key  string `json:"key"`
	Name string `json:"name"`
}

func TestRefdataListPrintsTheRecordedItems(t *testing.T) {
	rows := []struct {
		args    []string
		path    string
		fixture string
	}{
		{[]string{"status", "list"}, "/v3/statuses", "status-list.json"},
		{[]string{"priority", "list"}, "/v3/priorities", "priority-list.json"},
		{[]string{"resolution", "list"}, "/v3/resolutions", "resolution-list.json"},
		{[]string{"issuetype", "list"}, "/v3/issuetypes", "issuetype-list.json"},
	}

	for _, row := range rows {
		t.Run(strings.Join(row.args, " "), func(t *testing.T) {
			exchanges := faketracker.Load(t, filepath.Join(fixtureDir, row.fixture))

			res := runCLI(t, exchanges, slices.Concat(row.args, []string{"--json", "id,key,name"})...)

			if res.Code != 0 || res.Stderr != "" {
				t.Fatalf("exit = %d, stderr = %q, want 0 and empty", res.Code, res.Stderr)
			}
			if len(res.Requests) != 1 || res.Requests[0].Method != http.MethodGet || res.Requests[0].Path != row.path {
				t.Errorf("requests = %+v, want only GET %s", res.Requests, row.path)
			}

			var items []refdataItem
			if err := json.Unmarshal([]byte(res.Stdout), &items); err != nil {
				t.Fatalf("stdout is not a JSON array of items: %v\n%s", err, res.Stdout)
			}

			if want := fixtureItems(t, exchanges[0].Body); !slices.Equal(items, want) {
				t.Errorf("items = %+v,\nwant the fixture's %+v", items, want)
			}
			for _, item := range items {
				if item.Key == "" || item.Name == "" {
					t.Errorf("item %+v has an empty key or name", item)
				}
			}
		})
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

func TestRefdataListTrackerErrorLeavesStdoutEmpty(t *testing.T) {
	res := runCLI(t, []faketracker.Exchange{{
		Method: http.MethodGet,
		Path:   "/v3/statuses",
		Status: http.StatusInternalServerError,
		Header: http.Header{"Content-Type": {"application/json"}},
		Body:   []byte(`{"errorMessages":["Internal server error"],"errors":{}}`),
	}}, "status", "list", "--json", "id")

	if res.Code == 0 {
		t.Errorf("exit = 0, want non-zero")
	}
	if res.Stdout != "" {
		t.Errorf("stdout = %q, want empty", res.Stdout)
	}

	doc := decodeOneJSONError(t, "ytr status list --json id", res.Stderr)
	if !strings.Contains(doc.Message, "Internal server error") {
		t.Errorf("message = %q, want the server's text", doc.Message)
	}
}
