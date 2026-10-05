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
	code := opts.HandleInvocationError(&buf, nil)

	if code != 0 {
		t.Errorf("HandleInvocationError(nil) = %d, want 0", code)
	}
	if buf.Len() != 0 {
		t.Errorf("HandleInvocationError(nil) wrote %q, want nothing", buf.String())
	}
}

// TestHandleInvocationError_ExitError runs with no output flag set: the
// document does not wait for --json or --jq.
func TestHandleInvocationError_ExitError(t *testing.T) {
	var buf bytes.Buffer
	opts := output.Options{}

	err := ytrerrors.NewAuthError("auth failed", "login again")
	code := opts.HandleInvocationError(&buf, err)

	if code != 3 {
		t.Errorf("HandleInvocationError(AuthError) = %d, want 3", code)
	}

	want := `{"code":"auth_error","message":"auth failed","suggestion":"login again"}` + "\n"
	if got := buf.String(); got != want {
		t.Errorf("HandleInvocationError() output = %q, want %q", got, want)
	}
}

func TestHandleInvocationError_ExitError_NoSuggestion(t *testing.T) {
	var buf bytes.Buffer
	opts := output.Options{JSONFields: []string{"key"}}

	err := ytrerrors.NewNotFoundError("issue not found", "")
	code := opts.HandleInvocationError(&buf, err)

	if code != 4 {
		t.Errorf("HandleInvocationError(NotFoundError) = %d, want 4", code)
	}

	want := `{"code":"not_found","message":"issue not found"}` + "\n"
	if got := buf.String(); got != want {
		t.Errorf("HandleInvocationError() output = %q, want %q", got, want)
	}
}

func TestHasEmptySelection(t *testing.T) {
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
		{"--json= + --jq (still selects no field)", true, nil, ".x", true},
		{"fields set directly without flag (e.g. tests)", false, []string{"key"}, "", false},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			opts := output.Options{JSONFields: tc.jsonFields, JQFilter: tc.jqFilter}

			if got := opts.HasEmptySelection(tc.jsonFlagChanged); got != tc.want {
				t.Errorf("HasEmptySelection(%v) = %v, want %v", tc.jsonFlagChanged, got, tc.want)
			}
		})
	}
}

func TestHandleInvocationError_InvalidFieldError(t *testing.T) {
	var buf bytes.Buffer
	opts := output.Options{}

	err := ytrerrors.NewInvalidFieldError("bogus", []string{"key", "summary"})
	code := opts.HandleInvocationError(&buf, err)

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

// TestHandleInvocationError_JSONErrorSharesStderrWithDebug pins the one stream the JSON
// error document may take. Debug diagnostics used to push it onto stdout so
// they could keep stderr to themselves; the document now sits among them,
// because stdout has to stay empty for a run that failed.
func TestHandleInvocationError_JSONErrorSharesStderrWithDebug(t *testing.T) {
	var stderrBuf bytes.Buffer

	opts := output.Options{Debug: true, DebugOut: &stderrBuf}

	opts.Debugf("transport error")

	err := ytrerrors.NewNotFoundError("issue not found", "check the key")
	code := opts.HandleInvocationError(&stderrBuf, err)

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

// TestHandleInvocationError_BulkFailedError checks that an error type which adds
// fields to ExitError renders its own document. handleError matches the
// JSONError interface rather than naming concrete types, so this is what keeps
// a new type from being flattened to the generic user_error shape.
func TestHandleInvocationError_BulkFailedError(t *testing.T) {
	var buf bytes.Buffer
	opts := output.Options{}

	err := ytrerrors.NewBulkFailedError("op-1", "Operation FAILED", 7, 3)
	code := opts.HandleInvocationError(&buf, err)

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

func TestHandleInvocationError_GenericError(t *testing.T) {
	var buf bytes.Buffer
	opts := output.Options{}

	genericErr := bytes.ErrTooLarge
	code := opts.HandleInvocationError(&buf, genericErr)

	if code != ytrerrors.ExitUserError {
		t.Errorf("HandleInvocationError(generic) = %d, want %d", code, ytrerrors.ExitUserError)
	}

	var result map[string]string
	if unmarshalErr := json.Unmarshal(buf.Bytes(), &result); unmarshalErr != nil {
		t.Fatalf("HandleInvocationError generic output is invalid JSON: %v\nOutput: %q", unmarshalErr, buf.String())
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
	opts := output.Options{}

	var buf bytes.Buffer
	code := opts.HandleInvocationError(&buf, ytrerrors.NewUserError("bad input", "try again"))
	if code != ytrerrors.ExitUserError {
		t.Errorf("exit code = %d, want %d", code, ytrerrors.ExitUserError)
	}
	if got := buf.String(); strings.Count(got, "\n") != 1 {
		t.Errorf("JSON error must be one line, got %q", got)
	}
}

func TestFromContextWithoutOptionsTurnsEverythingOff(t *testing.T) {
	opts := output.FromContext(t.Context())

	if opts.HasFieldSelection() || opts.JQFilter != "" || opts.Debug {
		t.Errorf("FromContext(no options) = %+v, want no field selection, no jq filter and no debug", opts)
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
