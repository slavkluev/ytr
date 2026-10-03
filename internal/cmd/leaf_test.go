package cmd

import (
	"encoding/json"
	"net/http"
	"reflect"
	"regexp"
	"slices"
	"strings"
	"testing"
	"time"

	ytrerrors "github.com/slavkluev/ytr/internal/errors"
	"github.com/slavkluev/ytr/internal/faketracker"
	"github.com/slavkluev/ytr/internal/output"
)

// leafRow is one invocation of a leaf and what it must produce. Every
// exchange must be requested and nothing else, so a row with no exchanges
// sends no request at all.
type leafRow struct {
	name      string
	args      []string
	term      output.Options
	exchanges []faketracker.Exchange
	code      int

	// stdout is compared byte for byte, unless json or holds is set: json
	// wants stdout to decode to the same JSON value, and holds wants stdout,
	// less its ANSI codes, to contain each of its strings.
	stdout string
	json   string
	holds  []string

	// stderr must hold each of these; with none, stderr must be empty.
	stderr []string

	check func(t *testing.T, res cliResult)
}

func runLeafRows(t *testing.T, rows []leafRow) {
	t.Helper()

	for _, row := range rows {
		t.Run(row.name, func(t *testing.T) {
			res := runCLIOn(t, row.term, row.exchanges, row.args...)

			if res.Code != row.code {
				t.Errorf("exit = %d, want %d (stderr: %s)", res.Code, row.code, res.Stderr)
			}
			if len(res.Requests) != len(row.exchanges) {
				t.Errorf("requests = %+v, want %d", res.Requests, len(row.exchanges))
			}

			switch {
			case row.json != "":
				assertSameJSON(t, res.Stdout, row.json)
			case len(row.holds) > 0:
				for _, want := range row.holds {
					if !strings.Contains(withoutANSI(res.Stdout), want) {
						t.Errorf("stdout = %q, want it to hold %q", res.Stdout, want)
					}
				}
			case res.Stdout != row.stdout:
				t.Errorf("stdout = %q,\nwant %q", res.Stdout, row.stdout)
			}

			if len(row.stderr) == 0 {
				assertEmpty(t, "stderr", res.Stderr)
			}
			for _, want := range row.stderr {
				if !strings.Contains(res.Stderr, want) {
					t.Errorf("stderr = %q, want it to hold %q", res.Stderr, want)
				}
			}

			if row.check != nil {
				row.check(t, res)
			}
		})
	}
}

func assertSameJSON(t *testing.T, got, want string) {
	t.Helper()

	var gotValue, wantValue any
	if err := json.Unmarshal([]byte(got), &gotValue); err != nil {
		t.Fatalf("stdout is not one JSON document: %v\n%s", err, got)
	}
	if err := json.Unmarshal([]byte(want), &wantValue); err != nil {
		t.Fatalf("the row's JSON does not parse: %v\n%s", err, want)
	}
	if !reflect.DeepEqual(gotValue, wantValue) {
		t.Errorf("stdout = %s,\nwant %s", got, want)
	}
}

func trackerGET(path, body string) faketracker.Exchange {
	return faketracker.Exchange{
		Method: http.MethodGet,
		Path:   path,
		Status: http.StatusOK,
		Header: http.Header{"Content-Type": {"application/json"}},
		Body:   []byte(body),
	}
}

// trackerTime formats t the way Tracker sends a timestamp.
func trackerTime(t time.Time) string {
	return t.Format("2006-01-02T15:04:05.000-0700")
}

func trackerNotFound(path string) faketracker.Exchange {
	ex := trackerGET(path, `{"errorMessages":["Object not found"],"errors":{},"statusCode":404}`)
	ex.Status = http.StatusNotFound

	return ex
}

// notFoundRow asks for one JSON field of what path answers with a 404, and
// wants stdout empty and the server's text in the one JSON error document.
func notFoundRow(path string, args ...string) leafRow {
	return leafRow{
		name: "Tracker 404", args: args, exchanges: []faketracker.Exchange{trackerNotFound(path)},
		code: ytrerrors.ExitNotFound, stderr: []string{`"code":"not_found"`},
		check: func(t *testing.T, res cliResult) {
			t.Helper()

			var doc errorDocument
			if err := json.Unmarshal([]byte(res.Stderr), &doc); err != nil || strings.Count(res.Stderr, "\n") != 1 {
				t.Fatalf("stderr = %q, want exactly one JSON document (%v)", res.Stderr, err)
			}
			if doc.Code != ytrerrors.CodeNotFound || doc.Message != "Object not found" {
				t.Errorf("error = %+v, want code %q and the server's text", doc, ytrerrors.CodeNotFound)
			}
		},
	}
}

func named(name string, row leafRow) leafRow {
	row.name = name

	return row
}

// fieldHintRow wants --json= on the leaf at path, given args, to name its
// fields in order before any request.
func fieldHintRow(path string, args []string, fields ...string) leafRow {
	hint := "Specify one or more comma-separated field names for JSON output.\n\n" +
		"Available fields for " + path + ":\n  " + strings.Join(fields, "\n  ") + "\nError: no fields specified\n"

	return leafRow{
		name: "Field hint", args: slices.Concat(strings.Fields(path), args, []string{"--json="}),
		code:   ytrerrors.ExitUserError,
		stderr: []string{hint},
	}
}

// helpRow wants the leaf's --help to carry its JSON FIELDS and SEE ALSO
// sections as one block.
func helpRow(path, sections string) leafRow {
	return leafRow{name: "Help", args: append(strings.Fields(path), "--help"), holds: []string{sections}}
}

// assertAlignedTable fails when a TTY run printed the tab-separated table
// meant for a pipe.
func assertAlignedTable(t *testing.T, res cliResult) {
	t.Helper()

	if strings.Contains(res.Stdout, "\t") {
		t.Errorf("stdout = %q, want a table aligned with spaces on a TTY", res.Stdout)
	}
}

var ansiEscape = regexp.MustCompile("\x1b\\[[0-9;]*m")

func withoutANSI(s string) string {
	return ansiEscape.ReplaceAllString(s, "")
}
