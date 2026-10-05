package cmd

import (
	"encoding/json"
	"net/http"
	"net/url"
	"reflect"
	"slices"
	"strings"
	"testing"

	ytrerrors "github.com/slavkluev/ytr/internal/errors"
	"github.com/slavkluev/ytr/internal/faketracker"
)

// leafRow is one invocation of a leaf and what it must produce. Every
// exchange must be requested and nothing else, so a row with no exchanges
// sends no request at all.
type leafRow struct {
	name      string
	args      []string
	exchanges []faketracker.Exchange
	code      int

	// The run reads stdin, env and config as runAgainst describes. It is
	// signed in by harnessAuth unless signedOut is set, which leaves it only
	// the credentials env and config give it.
	stdin     string
	env       map[string]string
	config    string
	signedOut bool

	// stdout is compared byte for byte, unless json or holds is set: json
	// wants stdout to be one line that decodes to the same JSON value, and
	// holds wants stdout to contain each of its strings.
	stdout string
	json   string
	holds  []string

	// stderr must hold each of these; with none, stderr must be empty.
	stderr []string

	// body, when set, is the JSON the last request must carry as its body.
	body string

	check func(t *testing.T, res cliResult)
}

func runLeafRows(t *testing.T, rows []leafRow) {
	t.Helper()

	for _, row := range rows {
		t.Run(row.name, func(t *testing.T) {
			t.Parallel()

			argv := row.args
			if !row.signedOut {
				argv = slices.Concat(harnessAuth, row.args)
			}
			in := cliInput{stdin: row.stdin, env: row.env, config: row.config}
			res := runAgainst(t, in, faketracker.New(t, row.exchanges), argv)

			if res.Code != row.code {
				t.Errorf("exit = %d, want %d (stderr: %s)", res.Code, row.code, res.Stderr)
			}
			if len(res.Requests) != len(row.exchanges) {
				t.Errorf("requests = %+v, want %d", res.Requests, len(row.exchanges))
			}
			if row.body != "" {
				if len(res.Requests) == 0 {
					t.Errorf("no request was sent, want one with body %s", row.body)
				} else {
					assertSameJSONAs(t, "request body", res.Requests[len(res.Requests)-1].Body, row.body)
				}
			}

			switch {
			case row.json != "":
				assertOneLine(t, res.Stdout)
				assertSameJSONAs(t, "stdout", res.Stdout, row.json)
			case len(row.holds) > 0:
				for _, want := range row.holds {
					if !strings.Contains(res.Stdout, want) {
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

// assertOneLine wants stdout to be one line ending in a newline, as compact
// JSON is.
func assertOneLine(t *testing.T, stdout string) {
	t.Helper()

	if strings.Count(stdout, "\n") != 1 || !strings.HasSuffix(stdout, "\n") {
		t.Errorf("stdout = %q, want one line", stdout)
	}
}

func assertSameJSONAs(t *testing.T, label, got, want string) {
	t.Helper()

	var gotValue, wantValue any
	if err := json.Unmarshal([]byte(got), &gotValue); err != nil {
		t.Fatalf("%s is not one JSON document: %v\n%s", label, err, got)
	}
	if err := json.Unmarshal([]byte(want), &wantValue); err != nil {
		t.Fatalf("the row's JSON does not parse: %v\n%s", err, want)
	}
	if !reflect.DeepEqual(gotValue, wantValue) {
		t.Errorf("%s = %s,\nwant %s", label, got, want)
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

// withQuery is ex answering only a request that carries query.
func withQuery(ex faketracker.Exchange, query url.Values) faketracker.Exchange {
	ex.Query = query

	return ex
}

// trackerWrite answers a write the way Tracker does: status, and body as JSON
// unless it is empty, as the body of a 204 is.
func trackerWrite(method, path string, status int, body string) faketracker.Exchange {
	ex := faketracker.Exchange{Method: method, Path: path, Status: status}
	if body != "" {
		ex.Header = http.Header{"Content-Type": {"application/json"}}
		ex.Body = []byte(body)
	}

	return ex
}

func trackerPOST(path, body string) faketracker.Exchange {
	return trackerWrite(http.MethodPost, path, http.StatusCreated, body)
}

func trackerPATCH(path, body string) faketracker.Exchange {
	return trackerWrite(http.MethodPatch, path, http.StatusOK, body)
}

func trackerDELETE(path string) faketracker.Exchange {
	return trackerWrite(http.MethodDelete, path, http.StatusNoContent, "")
}

func trackerNotFound(path string) faketracker.Exchange {
	return trackerNotFoundOn(http.MethodGet, path, "Object not found")
}

// trackerNotFoundOn answers method on path with a 404 and Tracker's error body
// carrying message.
func trackerNotFoundOn(method, path, message string) faketracker.Exchange {
	return trackerError(method, path, http.StatusNotFound, message)
}

// trackerError answers method on path with status and Tracker's error body
// carrying message.
func trackerError(method, path string, status int, message string) faketracker.Exchange {
	body, _ := json.Marshal(
		map[string]any{
			"errorMessages": []string{message},
			"errors":        map[string]string{},
			"statusCode":    status,
		},
	)

	return trackerWrite(method, path, status, string(body))
}

// assertOneErrorDocument wants stderr to be exactly one JSON error document
// carrying message.
func assertOneErrorDocument(message string) func(*testing.T, cliResult) {
	return func(t *testing.T, res cliResult) {
		t.Helper()

		doc := decodeOneJSONErrorCoded(t, "stderr", res.Stderr, ytrerrors.CodeUserError)
		if doc.Message != message {
			t.Errorf("message = %q, want the server's text %q", doc.Message, message)
		}
	}
}

// notFoundRow runs args against a 404 on path, and wants stdout empty and the
// server's text in the one JSON error document.
func notFoundRow(path string, args ...string) leafRow {
	return failureRow(trackerNotFound(path), args...)
}

// failureRow runs args against ex, an exchange that answers with a 404 and
// Tracker's error body, and wants stdout empty and the server's text in the
// one JSON error document, with the not-found exit code.
func failureRow(ex faketracker.Exchange, args ...string) leafRow {
	var answer struct {
		ErrorMessages []string `json:"errorMessages"`
	}
	_ = json.Unmarshal(ex.Body, &answer)

	return leafRow{
		name: "Tracker 404", args: args, exchanges: []faketracker.Exchange{ex},
		code: ytrerrors.ExitNotFound, stderr: []string{`"code":"` + ytrerrors.CodeNotFound + `"`},
		check: func(t *testing.T, res cliResult) {
			t.Helper()

			doc := decodeOneJSONErrorCoded(t, "stderr", res.Stderr, ytrerrors.CodeNotFound)
			if want := strings.Join(answer.ErrorMessages, "; "); doc.Message != want {
				t.Errorf("message = %q, want the server's text %q", doc.Message, want)
			}
		},
	}
}

func named(name string, row leafRow) leafRow {
	row.name = name

	return row
}

// fieldHintRow wants --json= on the leaf at path, given args, refused before
// any request with a document that names its fields in order.
func fieldHintRow(path string, args []string, fields ...string) leafRow {
	return leafRow{
		name: "Field hint", args: slices.Concat(strings.Fields(path), args, []string{"--json="}),
		code:   ytrerrors.ExitUserError,
		stderr: []string{noFieldsDocument(fields)},
	}
}

// noFieldsDocument is what --json= writes to stderr on a leaf whose fields are
// fields.
func noFieldsDocument(fields []string) string {
	valid, _ := json.Marshal(fields)

	return `{"code":"invalid_field","message":"no fields specified","validFields":` + string(valid) +
		`,"suggestion":"Valid fields: ` + strings.Join(fields, ", ") + `"}` + "\n"
}

// helpRow wants the leaf's --help to hold tail: the end of its description, then
// its JSON FIELDS section.
func helpRow(path, tail string) leafRow {
	return leafRow{name: "Help", args: append(strings.Fields(path), "--help"), holds: []string{tail}}
}

// deleteRows are the output rows of the delete leaf args run, which Tracker
// answers with answer: what it prints for id with no flag, under --json and
// under --jq.
func deleteRows(args []string, answer faketracker.Exchange, id string) []leafRow {
	with := func(extra ...string) []string { return slices.Concat(args, extra) }
	sent := []faketracker.Exchange{answer}

	return []leafRow{
		{name: "Delete", args: args, exchanges: sent, json: `{"id": "` + id + `", "deleted": true}`},
		{name: "Delete JSON", args: with("--json", "id"), exchanges: sent, json: `{"id": "` + id + `"}`},
		{name: "Delete jq", args: with("--jq", ".deleted"), exchanges: sent, stdout: "true\n"},
	}
}
