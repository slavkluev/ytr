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

func TestPrintJSON_NoANSI(t *testing.T) {
	var buf bytes.Buffer
	data := map[string]string{"key": "value"}

	err := output.PrintJSON(&buf, data)
	if err != nil {
		t.Fatalf("PrintJSON() returned error: %v", err)
	}

	out := buf.String()
	// Check for ANSI escape sequences
	if strings.Contains(out, "\x1b") || strings.Contains(out, "\033") {
		t.Errorf("PrintJSON() output contains ANSI escape codes: %q", out)
	}
}

func TestPrintQuiet(t *testing.T) {
	var buf bytes.Buffer
	output.PrintQuiet(&buf, "value1", "value2")

	want := "value1\nvalue2\n"
	if got := buf.String(); got != want {
		t.Errorf("PrintQuiet() = %q, want %q", got, want)
	}
}

func TestMode(t *testing.T) {
	tests := []struct {
		name       string
		jsonFields []string
		quietFlag  bool
		want       output.OutputMode
	}{
		{"default is table", nil, false, output.ModeTable},
		{"json fields", []string{"key"}, false, output.ModeJSON},
		{"quiet flag", nil, true, output.ModeQuiet},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			output.JSONFields = tt.jsonFields
			output.QuietFlag = tt.quietFlag
			defer output.ResetFlags()

			if got := output.Mode(); got != tt.want {
				t.Errorf("Mode() = %v, want %v", got, tt.want)
			}
		})
	}
}

func TestMode_JSONFields(t *testing.T) {
	output.JSONFields = []string{"key", "status"}
	defer output.ResetFlags()

	if got := output.Mode(); got != output.ModeJSON {
		t.Errorf("Mode() with JSONFields = %v, want ModeJSON", got)
	}
}

func TestIsJSON_WithFields(t *testing.T) {
	output.JSONFields = []string{"key"}
	defer output.ResetFlags()

	if !output.IsJSON() {
		t.Error("IsJSON() = false, want true when JSONFields is non-empty")
	}
}

func TestIsJSON_Empty(t *testing.T) {
	output.JSONFields = nil
	output.JQFilter = ""
	defer output.ResetFlags()

	if output.IsJSON() {
		t.Error("IsJSON() = true, want false when JSONFields is nil and JQFilter is empty")
	}
}

func TestIsJSON_JQOnly(t *testing.T) {
	output.JSONFields = nil
	output.JQFilter = ".key"
	defer output.ResetFlags()

	if !output.IsJSON() {
		t.Error("IsJSON() = false, want true when JQFilter is set")
	}
}

func TestHasFieldSelection(t *testing.T) {
	output.JSONFields = []string{"key"}
	defer output.ResetFlags()

	if !output.HasFieldSelection() {
		t.Error("HasFieldSelection() = false, want true when JSONFields is non-empty")
	}

	output.JSONFields = nil
	if output.HasFieldSelection() {
		t.Error("HasFieldSelection() = true, want false when JSONFields is nil")
	}
}

func TestIsQuiet(t *testing.T) {
	output.QuietFlag = true
	defer output.ResetFlags()

	if !output.IsQuiet() {
		t.Error("IsQuiet() = false, want true when QuietFlag is true")
	}

	output.QuietFlag = false
	if output.IsQuiet() {
		t.Error("IsQuiet() = true, want false when QuietFlag is false")
	}
}

func TestDebugf(t *testing.T) {
	var buf bytes.Buffer
	output.SetDebugWriter(&buf)
	defer output.ResetFlags()

	output.Debugf("hidden")
	if buf.Len() != 0 {
		t.Fatalf("Debugf() wrote output while disabled: %q", buf.String())
	}

	output.DebugFlag = true
	output.Debugf("request %s", "ok")

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

func TestHandleError_Nil(t *testing.T) {
	var buf bytes.Buffer
	code := output.HandleError(&buf, nil)

	if code != 0 {
		t.Errorf("HandleError(nil) = %d, want 0", code)
	}
}

func TestHandleError_ExitError(t *testing.T) {
	var buf bytes.Buffer
	output.JSONFields = nil
	defer output.ResetFlags()

	err := ytrerrors.NewAuthError("auth failed", "login again")
	code := output.HandleError(&buf, err)

	if code != 3 {
		t.Errorf("HandleError(AuthError) = %d, want 3", code)
	}

	// Should contain the error message in output
	if !strings.Contains(buf.String(), "auth failed") {
		t.Errorf("HandleError() output = %q, should contain %q", buf.String(), "auth failed")
	}
}

func TestHandleError_ExitError_JSON(t *testing.T) {
	var buf bytes.Buffer
	output.JSONFields = []string{"key"}
	defer output.ResetFlags()

	err := ytrerrors.NewNotFoundError("issue not found", "check the key")
	code := output.HandleError(&buf, err)

	if code != 4 {
		t.Errorf("HandleError(NotFoundError) = %d, want 4", code)
	}

	// Should be valid JSON
	var result map[string]string
	if unmarshalErr := json.Unmarshal(buf.Bytes(), &result); unmarshalErr != nil {
		t.Fatalf("HandleError JSON output is invalid: %v\nOutput: %q", unmarshalErr, buf.String())
	}

	if result["code"] != "not_found" {
		t.Errorf("JSON code = %q, want %q", result["code"], "not_found")
	}
}

func TestHandleError_ExitError_JSON_DebugIsolation(t *testing.T) {
	var stderrBuf bytes.Buffer
	var jsonBuf bytes.Buffer

	output.JSONFields = []string{"key"}
	output.DebugFlag = true
	output.SetDebugWriter(&stderrBuf)
	output.SetJSONErrorWriter(&jsonBuf)
	defer output.ResetFlags()

	output.Debugf("transport error")

	err := ytrerrors.NewNotFoundError("issue not found", "check the key")
	code := output.HandleError(&stderrBuf, err)

	if code != 4 {
		t.Errorf("HandleError(NotFoundError) = %d, want 4", code)
	}

	if !strings.Contains(stderrBuf.String(), "[debug] transport error") {
		t.Fatalf("stderr debug output = %q, want debug line", stderrBuf.String())
	}

	if strings.Contains(stderrBuf.String(), `"code":"not_found"`) {
		t.Fatalf("stderr should not contain JSON error payload: %q", stderrBuf.String())
	}

	var result map[string]string
	if unmarshalErr := json.Unmarshal(jsonBuf.Bytes(), &result); unmarshalErr != nil {
		t.Fatalf("JSON error output is invalid: %v\nOutput: %q", unmarshalErr, jsonBuf.String())
	}

	if result["code"] != "not_found" {
		t.Errorf("JSON code = %q, want %q", result["code"], "not_found")
	}
}

func TestHandleError_GenericError(t *testing.T) {
	var buf bytes.Buffer
	output.JSONFields = nil
	defer output.ResetFlags()

	genericErr := bytes.ErrTooLarge
	code := output.HandleError(&buf, genericErr)

	if code != ytrerrors.ExitUserError {
		t.Errorf("HandleError(generic) = %d, want %d", code, ytrerrors.ExitUserError)
	}
}

func TestHandleError_GenericError_JSON(t *testing.T) {
	var buf bytes.Buffer
	output.JSONFields = []string{"key"}
	defer output.ResetFlags()

	genericErr := bytes.ErrTooLarge
	code := output.HandleError(&buf, genericErr)

	if code != ytrerrors.ExitUserError {
		t.Errorf("HandleError(generic JSON) = %d, want %d", code, ytrerrors.ExitUserError)
	}

	var result map[string]string
	if unmarshalErr := json.Unmarshal(buf.Bytes(), &result); unmarshalErr != nil {
		t.Fatalf("HandleError generic JSON output is invalid: %v\nOutput: %q", unmarshalErr, buf.String())
	}

	if result["code"] != "user_error" {
		t.Errorf("JSON code = %q, want %q", result["code"], "user_error")
	}
	if result["message"] != bytes.ErrTooLarge.Error() {
		t.Errorf("JSON message = %q, want %q", result["message"], bytes.ErrTooLarge.Error())
	}
}
