package errors_test

import (
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	"testing"

	ytrerrors "github.com/slavkluev/ytr/internal/errors"
)

func TestExitCodes(t *testing.T) {
	tests := []struct {
		name     string
		constant int
		want     int
	}{
		{"ExitSuccess", ytrerrors.ExitSuccess, 0},
		{"ExitUserError", ytrerrors.ExitUserError, 1},
		{"ExitAuthError", ytrerrors.ExitAuthError, 3},
		{"ExitNotFound", ytrerrors.ExitNotFound, 4},
		{"ExitRateLimited", ytrerrors.ExitRateLimited, 5},
		{"ExitInterrupted", ytrerrors.ExitInterrupted, 130},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if tt.constant != tt.want {
				t.Errorf("%s = %d, want %d", tt.name, tt.constant, tt.want)
			}
		})
	}
}

func TestExitError_Error(t *testing.T) {
	err := &ytrerrors.ExitError{
		ExitCode: 1,
		Code:     "user_error",
		Message:  "something went wrong",
	}

	if got := err.Error(); got != "something went wrong" {
		t.Errorf("Error() = %q, want %q", got, "something went wrong")
	}
}

func TestExitError_JSONError(t *testing.T) {
	t.Run("with suggestion", func(t *testing.T) {
		err := &ytrerrors.ExitError{
			ExitCode:   3,
			Code:       "auth_error",
			Message:    "not authenticated",
			Suggestion: "Run: ytr auth login",
		}

		data, jsonErr := err.JSONError()
		if jsonErr != nil {
			t.Fatalf("JSONError() returned error: %v", jsonErr)
		}

		var result map[string]string
		if unmarshalErr := json.Unmarshal(data, &result); unmarshalErr != nil {
			t.Fatalf("JSONError() produced invalid JSON: %v", unmarshalErr)
		}

		if result["code"] != "auth_error" {
			t.Errorf("code = %q, want %q", result["code"], "auth_error")
		}
		if result["message"] != "not authenticated" {
			t.Errorf("message = %q, want %q", result["message"], "not authenticated")
		}
		if result["suggestion"] != "Run: ytr auth login" {
			t.Errorf("suggestion = %q, want %q", result["suggestion"], "Run: ytr auth login")
		}
	})

	t.Run("without suggestion omits field", func(t *testing.T) {
		err := &ytrerrors.ExitError{
			ExitCode: 1,
			Code:     "user_error",
			Message:  "bad input",
		}

		data, jsonErr := err.JSONError()
		if jsonErr != nil {
			t.Fatalf("JSONError() returned error: %v", jsonErr)
		}

		var result map[string]interface{}
		if unmarshalErr := json.Unmarshal(data, &result); unmarshalErr != nil {
			t.Fatalf("JSONError() produced invalid JSON: %v", unmarshalErr)
		}

		if _, exists := result["suggestion"]; exists {
			t.Errorf("suggestion field should be omitted when empty, got %v", result["suggestion"])
		}
	})
}

func TestNewAuthError(t *testing.T) {
	err := ytrerrors.NewAuthError("not authenticated", "Run: ytr auth login")

	if err.ExitCode != 3 {
		t.Errorf("ExitCode = %d, want 3", err.ExitCode)
	}
	if err.Code != "auth_error" {
		t.Errorf("Code = %q, want %q", err.Code, "auth_error")
	}
	if err.Message != "not authenticated" {
		t.Errorf("Message = %q, want %q", err.Message, "not authenticated")
	}
	if err.Suggestion != "Run: ytr auth login" {
		t.Errorf("Suggestion = %q, want %q", err.Suggestion, "Run: ytr auth login")
	}
}

func TestNewNotFoundError(t *testing.T) {
	err := ytrerrors.NewNotFoundError("issue not found", "Check the issue key")

	if err.ExitCode != 4 {
		t.Errorf("ExitCode = %d, want 4", err.ExitCode)
	}
	if err.Code != "not_found" {
		t.Errorf("Code = %q, want %q", err.Code, "not_found")
	}
	if err.Message != "issue not found" {
		t.Errorf("Message = %q, want %q", err.Message, "issue not found")
	}
	if err.Suggestion != "Check the issue key" {
		t.Errorf("Suggestion = %q, want %q", err.Suggestion, "Check the issue key")
	}
}

func TestNewUserError(t *testing.T) {
	err := ytrerrors.NewUserError("invalid flag", "Use --help for usage")

	if err.ExitCode != 1 {
		t.Errorf("ExitCode = %d, want 1", err.ExitCode)
	}
	if err.Code != "user_error" {
		t.Errorf("Code = %q, want %q", err.Code, "user_error")
	}
	if err.Message != "invalid flag" {
		t.Errorf("Message = %q, want %q", err.Message, "invalid flag")
	}
	if err.Suggestion != "Use --help for usage" {
		t.Errorf("Suggestion = %q, want %q", err.Suggestion, "Use --help for usage")
	}
}

func TestNewRateLimitedError(t *testing.T) {
	err := ytrerrors.NewRateLimitedError("rate limit exceeded", "Wait and retry")

	if err.ExitCode != 5 {
		t.Errorf("ExitCode = %d, want 5", err.ExitCode)
	}
	if err.Code != "rate_limited" {
		t.Errorf("Code = %q, want %q", err.Code, "rate_limited")
	}
	if err.Message != "rate limit exceeded" {
		t.Errorf("Message = %q, want %q", err.Message, "rate limit exceeded")
	}
	if err.Suggestion != "Wait and retry" {
		t.Errorf("Suggestion = %q, want %q", err.Suggestion, "Wait and retry")
	}
}

func TestErrorsAs(t *testing.T) {
	original := ytrerrors.NewAuthError("auth failed", "login again")
	wrapped := fmt.Errorf("command failed: %w", original)

	var target *ytrerrors.ExitError
	if !errors.As(wrapped, &target) {
		t.Fatal("errors.As failed to extract ExitError from wrapped error")
	}

	if target.ExitCode != 3 {
		t.Errorf("ExitCode = %d, want 3", target.ExitCode)
	}
	if target.Code != "auth_error" {
		t.Errorf("Code = %q, want %q", target.Code, "auth_error")
	}
	if target.Message != "auth failed" {
		t.Errorf("Message = %q, want %q", target.Message, "auth failed")
	}
}

func TestPrintHuman_NoColors(t *testing.T) {
	var buf bytes.Buffer
	err := &ytrerrors.ExitError{
		ExitCode:   3,
		Code:       "auth_error",
		Message:    "not authenticated",
		Suggestion: "Run: ytr auth login",
	}

	ytrerrors.PrintHuman(&buf, err, false)

	want := "Error: not authenticated\nRun: ytr auth login\n"
	if got := buf.String(); got != want {
		t.Errorf("PrintHuman() = %q, want %q", got, want)
	}
}

func TestPrintHuman_WithColors(t *testing.T) {
	var bufColor bytes.Buffer
	var bufPlain bytes.Buffer
	err := &ytrerrors.ExitError{
		ExitCode: 1,
		Code:     "user_error",
		Message:  "bad input",
	}

	ytrerrors.PrintHuman(&bufColor, err, true)
	ytrerrors.PrintHuman(&bufPlain, err, false)

	colorOut := bufColor.String()
	plainOut := bufPlain.String()

	if len(colorOut) == 0 {
		t.Fatal("PrintHuman with colors produced empty output")
	}

	// Colored output should differ from plain output (contains ANSI codes)
	if colorOut == plainOut {
		t.Errorf("PrintHuman with colors should produce different output than without colors")
	}

	// Colored output should still contain the message
	if !bytes.Contains(bufColor.Bytes(), []byte("bad input")) {
		t.Error("PrintHuman with colors should contain the error message")
	}
}

func TestPrintHuman_NoSuggestion(t *testing.T) {
	var buf bytes.Buffer
	err := &ytrerrors.ExitError{
		ExitCode: 1,
		Code:     "user_error",
		Message:  "bad input",
	}

	ytrerrors.PrintHuman(&buf, err, false)

	want := "Error: bad input\n"
	if got := buf.String(); got != want {
		t.Errorf("PrintHuman() = %q, want %q", got, want)
	}
}
