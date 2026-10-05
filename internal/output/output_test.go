package output_test

import (
	"bytes"
	"encoding/json"
	"strings"
	"testing"

	ytrerrors "github.com/slavkluev/ytr/internal/errors"
	"github.com/slavkluev/ytr/internal/output"
)

func TestPrintJSON(t *testing.T) {
	var buf bytes.Buffer
	data := struct {
		Name  string `json:"name"`
		Count int    `json:"count"`
	}{
		Name:  "test",
		Count: 42,
	}

	err := output.PrintJSON(&buf, data)
	if err != nil {
		t.Fatalf("PrintJSON() returned error: %v", err)
	}

	// Verify output is valid JSON
	var result map[string]interface{}
	if unmarshalErr := json.Unmarshal(buf.Bytes(), &result); unmarshalErr != nil {
		t.Fatalf("PrintJSON() produced invalid JSON: %v\nOutput: %q", unmarshalErr, buf.String())
	}

	if result["name"] != "test" {
		t.Errorf("name = %v, want %q", result["name"], "test")
	}
	if result["count"] != float64(42) {
		t.Errorf("count = %v, want 42", result["count"])
	}

	// Verify trailing newline
	if !strings.HasSuffix(buf.String(), "\n") {
		t.Error("PrintJSON() output should end with newline")
	}
}

func TestIsJSON_WithFields(t *testing.T) {
	opts := output.Options{JSONFields: []string{"key"}}

	if !opts.IsJSON() {
		t.Error("IsJSON() = false, want true when JSONFields is non-empty")
	}
}

func TestIsJSON_Empty(t *testing.T) {
	opts := output.Options{}

	if opts.IsJSON() {
		t.Error("IsJSON() = true, want false when JSONFields is nil and JQFilter is empty")
	}
}

func TestIsJSON_JQOnly(t *testing.T) {
	opts := output.Options{JQFilter: ".key"}

	if !opts.IsJSON() {
		t.Error("IsJSON() = false, want true when JQFilter is set")
	}
}

func TestHasFieldSelection(t *testing.T) {
	opts := output.Options{JSONFields: []string{"key"}}

	if !opts.HasFieldSelection() {
		t.Error("HasFieldSelection() = false, want true when JSONFields is non-empty")
	}

	opts.JSONFields = nil
	if opts.HasFieldSelection() {
		t.Error("HasFieldSelection() = true, want false when JSONFields is nil")
	}
}

func TestDebugf(t *testing.T) {
	var buf bytes.Buffer
	opts := output.Options{DebugOut: &buf}

	opts.Debugf("hidden")
	if buf.Len() != 0 {
		t.Fatalf("Debugf() wrote output while disabled: %q", buf.String())
	}

	opts.Debug = true
	opts.Debugf("request %s", "ok")

	if got := buf.String(); got != "[debug] request ok\n" {
		t.Errorf("Debugf() = %q, want %q", got, "[debug] request ok\n")
	}
}

func TestSanitizeDebugString(t *testing.T) {
	for _, tc := range []struct {
		name  string
		input string
		want  string
	}{
		{
			name:  "bearer token",
			input: "Authorization: Bearer secret-token-value",
			want:  "Authorization: Bearer <redacted>",
		},
		{
			name:  "natural token wording",
			input: "tracker returned invalid token abc123xyz",
			want:  "tracker returned invalid token <redacted>",
		},
		{
			name:  "query style pair",
			input: "access_token=abc123",
			want:  "access_token=<redacted>",
		},
	} {
		t.Run(tc.name, func(t *testing.T) {
			if got := output.SanitizeDebugString(tc.input); got != tc.want {
				t.Errorf("SanitizeDebugString() = %q, want %q", got, tc.want)
			}
		})
	}
}

func TestHandleInvocationError_Nil(t *testing.T) {
	opts := output.Options{}

	var buf bytes.Buffer
	code := opts.HandleInvocationError(&buf, nil, nil)

	if code != 0 {
		t.Errorf("HandleInvocationError(nil) = %d, want 0", code)
	}
}

func TestHandleInvocationError_ExitError(t *testing.T) {
	var buf bytes.Buffer
	opts := output.Options{}

	err := ytrerrors.NewAuthError("auth failed", "login again")
	code := opts.HandleInvocationError(&buf, err, nil)

	if code != 3 {
		t.Errorf("HandleInvocationError(AuthError) = %d, want 3", code)
	}

	// Should contain the error message in output
	if !strings.Contains(buf.String(), "auth failed") {
		t.Errorf("HandleInvocationError() output = %q, should contain %q", buf.String(), "auth failed")
	}
}

func TestHandleInvocationError_ExitError_JSON(t *testing.T) {
	var buf bytes.Buffer
	opts := output.Options{JSONFields: []string{"key"}}

	err := ytrerrors.NewNotFoundError("issue not found", "check the key")
	code := opts.HandleInvocationError(&buf, err, nil)

	if code != 4 {
		t.Errorf("HandleInvocationError(NotFoundError) = %d, want 4", code)
	}

	// Should be valid JSON
	var result map[string]string
	if unmarshalErr := json.Unmarshal(buf.Bytes(), &result); unmarshalErr != nil {
		t.Fatalf("HandleInvocationError JSON output is invalid: %v\nOutput: %q", unmarshalErr, buf.String())
	}

	if result["code"] != "not_found" {
		t.Errorf("JSON code = %q, want %q", result["code"], "not_found")
	}
}

func TestWantsFieldHint(t *testing.T) {
	tests := []struct {
		name            string
		jsonFlagChanged bool
		jsonFields      []string
		jqFilter        string
		want            bool
	}{
		{"no flags", false, nil, "", false},
		{"bare --json= (empty slice, flag changed)", true, nil, "", true},
		{"--json with fields", true, []string{"key"}, "", false},
		{"--jq only (no --json)", false, nil, ".x", false},
		{"--json fields + --jq", true, []string{"key"}, ".x", false},
		{"--json= + --jq (jq wins, no hint)", true, nil, ".x", false},
		{"fields set directly without flag (e.g. tests)", false, []string{"key"}, "", false},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			opts := output.Options{JSONFields: tc.jsonFields, JQFilter: tc.jqFilter}

			if got := opts.WantsFieldHint(tc.jsonFlagChanged); got != tc.want {
				t.Errorf("WantsFieldHint(%v) = %v, want %v", tc.jsonFlagChanged, got, tc.want)
			}
		})
	}
}

func TestHandleInvocationError_InvalidFieldError_JSON(t *testing.T) {
	var buf bytes.Buffer
	opts := output.Options{JSONFields: []string{"key"}}

	err := ytrerrors.NewInvalidFieldError("bogus", []string{"key", "summary"})
	code := opts.HandleInvocationError(&buf, err, nil)

	if code != 1 {
		t.Errorf("HandleInvocationError(InvalidFieldError) = %d, want 1", code)
	}

	var result map[string]any
	if unmarshalErr := json.Unmarshal(buf.Bytes(), &result); unmarshalErr != nil {
		t.Fatalf("HandleInvocationError JSON output is invalid: %v\nOutput: %q", unmarshalErr, buf.String())
	}

	// The structured invalid_field payload must survive — not be flattened to
	// the generic user_error shape.
	if result["code"] != "invalid_field" {
		t.Errorf("JSON code = %v, want %q", result["code"], "invalid_field")
	}
	if result["invalidField"] != "bogus" {
		t.Errorf("JSON invalidField = %v, want %q", result["invalidField"], "bogus")
	}
	valid, ok := result["validFields"].([]any)
	if !ok || len(valid) != 2 {
		t.Errorf("JSON validFields = %v, want [key summary]", result["validFields"])
	}
}

func TestHandleInvocationError_InvalidFieldError_Human(t *testing.T) {
	var buf bytes.Buffer
	opts := output.Options{}

	err := ytrerrors.NewInvalidFieldError("bogus", []string{"key", "summary"})
	code := opts.HandleInvocationError(&buf, err, nil)

	if code != 1 {
		t.Errorf("HandleInvocationError(InvalidFieldError) = %d, want 1", code)
	}

	out := buf.String()
	if !strings.Contains(out, "unknown field") {
		t.Errorf("human output = %q, should contain %q", out, "unknown field")
	}
	// The "Valid fields: …" recovery hint must be shown.
	if !strings.Contains(out, "Valid fields:") || !strings.Contains(out, "summary") {
		t.Errorf("human output = %q, should contain valid-fields hint", out)
	}
}

// TestHandleInvocationError_JSONErrorSharesStderrWithDebug pins the one stream the JSON
// error document may take. Debug diagnostics used to push it onto stdout so
// they could keep stderr to themselves; the document now sits among them,
// because stdout has to stay empty for a run that failed.
func TestHandleInvocationError_JSONErrorSharesStderrWithDebug(t *testing.T) {
	var stderrBuf bytes.Buffer

	opts := output.Options{JSONFields: []string{"key"}, Debug: true, DebugOut: &stderrBuf}

	opts.Debugf("transport error")

	err := ytrerrors.NewNotFoundError("issue not found", "check the key")
	code := opts.HandleInvocationError(&stderrBuf, err, nil)

	if code != 4 {
		t.Errorf("HandleInvocationError(NotFoundError) = %d, want 4", code)
	}

	lines := strings.Split(strings.TrimSuffix(stderrBuf.String(), "\n"), "\n")
	if len(lines) != 2 {
		t.Fatalf("stderr = %q, want the debug line followed by one error document", stderrBuf.String())
	}
	if lines[0] != "[debug] transport error" {
		t.Errorf("stderr line 1 = %q, want the debug line", lines[0])
	}

	var result map[string]string
	if unmarshalErr := json.Unmarshal([]byte(lines[1]), &result); unmarshalErr != nil {
		t.Fatalf("JSON error output is invalid: %v\nOutput: %q", unmarshalErr, lines[1])
	}

	if result["code"] != "not_found" {
		t.Errorf("JSON code = %q, want %q", result["code"], "not_found")
	}
}

// TestHandleInvocationError_BulkFailedError_JSON checks that an error type which adds
// fields to ExitError renders its own document. handleError matches the
// JSONError interface rather than naming concrete types, so this is what keeps
// a new type from being flattened to the generic user_error shape.
func TestHandleInvocationError_BulkFailedError_JSON(t *testing.T) {
	var buf bytes.Buffer
	opts := output.Options{JSONFields: []string{"id"}}

	err := ytrerrors.NewBulkFailedError("op-1", "Operation FAILED", 7, 3)
	code := opts.HandleInvocationError(&buf, err, nil)

	if code != ytrerrors.ExitUserError {
		t.Errorf("HandleInvocationError(BulkFailedError) = %d, want %d", code, ytrerrors.ExitUserError)
	}

	var result map[string]any
	if unmarshalErr := json.Unmarshal(buf.Bytes(), &result); unmarshalErr != nil {
		t.Fatalf("HandleInvocationError JSON output is invalid: %v\nOutput: %q", unmarshalErr, buf.String())
	}

	if result["code"] != ytrerrors.CodeBulkFailed {
		t.Errorf("JSON code = %v, want %q", result["code"], ytrerrors.CodeBulkFailed)
	}
	if result["operationId"] != "op-1" {
		t.Errorf("JSON operationId = %v, want %q", result["operationId"], "op-1")
	}
	if result["statusText"] != "Operation FAILED" {
		t.Errorf("JSON statusText = %v, want %q", result["statusText"], "Operation FAILED")
	}
	if result["totalIssues"] != float64(7) {
		t.Errorf("JSON totalIssues = %v, want 7", result["totalIssues"])
	}
	if result["totalCompletedIssues"] != float64(3) {
		t.Errorf("JSON totalCompletedIssues = %v, want 3", result["totalCompletedIssues"])
	}
	if result["suggestion"] != "ytr bulk status op-1" {
		t.Errorf("JSON suggestion = %v, want a runnable ytr bulk status", result["suggestion"])
	}
}

// TestHandleInvocationError_BulkFailedError_Human checks the counts reach a human too:
// human output shows only the message and the suggestion, so the message has
// to carry what the JSON document holds in its own fields.
func TestHandleInvocationError_BulkFailedError_Human(t *testing.T) {
	var buf bytes.Buffer
	opts := output.Options{}

	err := ytrerrors.NewBulkFailedError("op-1", "Operation FAILED", 7, 3)
	code := opts.HandleInvocationError(&buf, err, nil)

	if code != ytrerrors.ExitUserError {
		t.Errorf("HandleInvocationError(BulkFailedError) = %d, want %d", code, ytrerrors.ExitUserError)
	}

	out := buf.String()
	for _, want := range []string{"Operation FAILED", "3 of 7", "ytr bulk status op-1"} {
		if !strings.Contains(out, want) {
			t.Errorf("human output = %q, should contain %q", out, want)
		}
	}
}

func TestHandleInvocationError_GenericError(t *testing.T) {
	var buf bytes.Buffer
	opts := output.Options{}

	genericErr := bytes.ErrTooLarge
	code := opts.HandleInvocationError(&buf, genericErr, nil)

	if code != ytrerrors.ExitUserError {
		t.Errorf("HandleInvocationError(generic) = %d, want %d", code, ytrerrors.ExitUserError)
	}
}

func TestHandleInvocationError_GenericError_JSON(t *testing.T) {
	var buf bytes.Buffer
	opts := output.Options{JSONFields: []string{"key"}}

	genericErr := bytes.ErrTooLarge
	code := opts.HandleInvocationError(&buf, genericErr, nil)

	if code != ytrerrors.ExitUserError {
		t.Errorf("HandleInvocationError(generic JSON) = %d, want %d", code, ytrerrors.ExitUserError)
	}

	var result map[string]string
	if unmarshalErr := json.Unmarshal(buf.Bytes(), &result); unmarshalErr != nil {
		t.Fatalf("HandleInvocationError generic JSON output is invalid: %v\nOutput: %q", unmarshalErr, buf.String())
	}

	if result["code"] != "user_error" {
		t.Errorf("JSON code = %q, want %q", result["code"], "user_error")
	}
	if result["message"] != bytes.ErrTooLarge.Error() {
		t.Errorf("JSON message = %q, want %q", result["message"], bytes.ErrTooLarge.Error())
	}
}

func TestPrintJSONIsOneLine(t *testing.T) {
	var buf bytes.Buffer
	data := map[string]any{"key": "APP-1", "labels": []string{"a", "b"}}
	if err := output.PrintJSON(&buf, data); err != nil {
		t.Fatalf("PrintJSON() returned error: %v", err)
	}

	if got, want := buf.String(), `{"key":"APP-1","labels":["a","b"]}`+"\n"; got != want {
		t.Errorf("PrintJSON() = %q, want %q", got, want)
	}
}

func TestHandleInvocationErrorJSONIsOneLine(t *testing.T) {
	opts := output.Options{JSONFields: []string{"key"}}

	var buf bytes.Buffer
	code := opts.HandleInvocationError(&buf, ytrerrors.NewUserError("bad input", "try again"), nil)
	if code != ytrerrors.ExitUserError {
		t.Errorf("exit code = %d, want %d", code, ytrerrors.ExitUserError)
	}
	if got := buf.String(); strings.Count(got, "\n") != 1 {
		t.Errorf("JSON error must be one line, got %q", got)
	}
}

func TestHandleInvocationErrorReadsJSONFromRawArgs(t *testing.T) {
	opts := output.Options{}

	// pflag stopped at --nosuchflag, so --json never reached JSONFields.
	var buf bytes.Buffer
	args := []string{"issue", "list", "--nosuchflag", "--json", "key"}
	code := opts.HandleInvocationError(&buf, ytrerrors.NewUserError("unknown flag: --nosuchflag", "try --help"), args)

	if code != ytrerrors.ExitUserError {
		t.Errorf("exit code = %d, want %d", code, ytrerrors.ExitUserError)
	}

	var doc map[string]string
	if err := json.Unmarshal(buf.Bytes(), &doc); err != nil {
		t.Fatalf("output is not a JSON document (%v): %q", err, buf.String())
	}
	if doc["message"] != "unknown flag: --nosuchflag" {
		t.Errorf("message = %q, want the flag error", doc["message"])
	}
}

func TestHandleInvocationErrorKeepsTextWithoutJSONArgs(t *testing.T) {
	opts := output.Options{}

	var buf bytes.Buffer
	args := []string{"issue", "list", "--nosuchflag"}
	opts.HandleInvocationError(&buf, ytrerrors.NewUserError("unknown flag: --nosuchflag", "try --help"), args)

	if got, want := buf.String(), "Error: unknown flag: --nosuchflag\ntry --help\n"; got != want {
		t.Errorf("output = %q, want %q", got, want)
	}
}

func TestHandleInvocationErrorIgnoresRawArgsOnSuccess(t *testing.T) {
	opts := output.Options{}

	var buf bytes.Buffer
	if code := opts.HandleInvocationError(&buf, nil, []string{"--json", "key"}); code != ytrerrors.ExitSuccess {
		t.Errorf("exit code = %d, want %d", code, ytrerrors.ExitSuccess)
	}
	if buf.Len() != 0 {
		t.Errorf("output = %q, want empty", buf.String())
	}
}

func TestHandleInvocationErrorRawArgForms(t *testing.T) {
	cases := []struct {
		name   string
		args   []string
		isJSON bool
	}{
		{"space separated json", []string{"issue", "--x", "--json", "key"}, true},
		{"equals form json", []string{"issue", "--x", "--json=key"}, true},
		{"space separated jq", []string{"issue", "--x", "--jq", ".key"}, true},
		{"equals form jq", []string{"issue", "--x", "--jq=.key"}, true},
		{"no json at all", []string{"issue", "--x"}, false},
		{"empty equals form stays on the field-hint path", []string{"issue", "--x", "--json="}, false},
		{"json with no value", []string{"issue", "--x", "--json"}, false},
		{"json followed by another flag", []string{"issue", "--x", "--json", "--debug"}, false},
		{"json after a bare double dash is a value", []string{"issue", "--x", "--", "--json", "key"}, false},
	}

	for _, c := range cases {
		var opts output.Options

		var buf bytes.Buffer
		opts.HandleInvocationError(&buf, ytrerrors.NewUserError("bad invocation", "try --help"), c.args)

		gotJSON := strings.HasPrefix(buf.String(), "{")
		if gotJSON != c.isJSON {
			t.Errorf("%s: rendered JSON = %v, want %v (output %q)", c.name, gotJSON, c.isJSON, buf.String())
		}
	}
}

func TestFromContextWithoutOptionsTurnsEverythingOff(t *testing.T) {
	opts := output.FromContext(t.Context())

	if opts.IsJSON() || opts.Debug {
		t.Errorf("FromContext(no options) = %+v, want no JSON and no debug", opts)
	}
}

func TestFromContextReturnsTheCarriedOptions(t *testing.T) {
	carried := &output.Options{JSONFields: []string{"key"}}

	got := output.FromContext(output.NewContext(t.Context(), carried))
	if got != carried {
		t.Fatalf("FromContext() = %p, want the carried %p", got, carried)
	}

	got.JSONFields = []string{"summary"}
	if carried.JSONFields[0] != "summary" {
		t.Errorf("a change through FromContext did not reach the carried options: %v", carried.JSONFields)
	}
}
