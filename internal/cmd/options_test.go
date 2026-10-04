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
	if !strings.HasPrefix(second.Stdout, "ID\tKEY\tNAME\n") {
		t.Errorf("second run stdout = %q, want TSV under an ID, KEY, NAME header", second.Stdout)
	}
}

func TestQuietPrintsOneStatusKeyPerLine(t *testing.T) {
	t.Parallel()

	exchanges := faketracker.Load(t, filepath.Join(fixtureDir, "status-list.json"))

	res := runCLI(t, exchanges, "status", "list", "--quiet")
	if res.Code != 0 {
		t.Fatalf("exit = %d, stderr = %q, want 0", res.Code, res.Stderr)
	}
	if strings.Contains(res.Stdout, "ID\tKEY\tNAME") {
		t.Errorf("stdout = %q, want no table header under --quiet", res.Stdout)
	}

	var keys []string
	for _, page := range exchanges {
		for _, item := range fixtureItems(t, page.Body) {
			keys = append(keys, item.Key)
		}
	}
	if got := strings.Split(strings.TrimSuffix(res.Stdout, "\n"), "\n"); !slices.Equal(got, keys) {
		t.Errorf("stdout lines = %q, want the fixture's keys %q", got, keys)
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
	}}, "status", "list", "--debug", "--json", "id")

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
	decodeOneJSONError(t, "ytr status list --debug --json id", lines[len(lines)-1])
}

func TestRunCLIIgnoresTheColorEnvironment(t *testing.T) {
	t.Setenv("NO_COLOR", "")
	t.Setenv("CLICOLOR_FORCE", "1")

	res := runCLI(t, nil, "status", "lst")

	if res.Code == 0 {
		t.Errorf("exit = 0, want non-zero for an unknown subcommand")
	}
	if strings.Contains(res.Stdout+res.Stderr, "\x1b") {
		t.Errorf("stdout %q, stderr %q carry ANSI codes, want none", res.Stdout, res.Stderr)
	}
}
