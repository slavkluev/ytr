package cmd

import (
	"encoding/json"
	"net/http"
	"path/filepath"
	"strings"
	"testing"

	"github.com/slavkluev/ytr/internal/faketracker"
)

func TestOutputOptionsDoNotLeakIntoTheNextRun(t *testing.T) {
	t.Parallel()

	exchanges := faketracker.Load(t, filepath.Join(fixtureDir, "status-list.json"))

	first := runCLI(t, exchanges, "status", "list", "--json", "id")
	if first.Code != 0 || !json.Valid([]byte(first.Stdout)) {
		t.Fatalf("first run: exit = %d, stdout = %q, want 0 and a JSON document", first.Code, first.Stdout)
	}

	second := runCLI(t, exchanges, "status", "list")
	if second.Code != 0 {
		t.Fatalf("second run: exit = %d, stderr = %q, want 0", second.Code, second.Stderr)
	}

	var page struct {
		Items []map[string]any `json:"items"`
	}
	if err := json.Unmarshal([]byte(second.Stdout), &page); err != nil || len(page.Items) == 0 {
		t.Fatalf("second run stdout = %q, want the envelope of a list of statuses (%v)", second.Stdout, err)
	}
	if _, ok := page.Items[0]["name"]; !ok {
		t.Errorf("second run item = %v, want every field, not the first run's id alone", page.Items[0])
	}
}

func TestDebugLinesPrecedeTheOneErrorDocument(t *testing.T) {
	t.Parallel()

	res := runCLI(t, []faketracker.Exchange{{
		Method: http.MethodGet,
		Path:   "/v3/statuses",
		Status: http.StatusInternalServerError,
		Header: http.Header{"Content-Type": {"application/json"}},
		Body:   []byte(`{"errorMessages":["Internal server error"],"errors":{}}`),
	}}, "status", "list", "--debug")

	if res.Code == 0 {
		t.Errorf("exit = 0, want non-zero")
	}
	if res.Stdout != "" {
		t.Errorf("stdout = %q, want empty", res.Stdout)
	}
	for _, want := range []string{"[debug] request GET /v3/statuses", "[debug] api_error status=500"} {
		if !strings.Contains(res.Stderr, want) {
			t.Errorf("stderr = %q, want it to hold %q", res.Stderr, want)
		}
	}

	lines := strings.Split(strings.TrimSuffix(res.Stderr, "\n"), "\n")
	for _, line := range lines[:len(lines)-1] {
		if !strings.HasPrefix(line, "[debug] ") {
			t.Errorf("stderr line %q precedes the error document but is not a debug line", line)
		}
	}
	decodeOneJSONError(t, "ytr status list --debug", lines[len(lines)-1]+"\n")
}
