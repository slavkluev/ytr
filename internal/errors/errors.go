package errors

import (
	"encoding/json"
	"fmt"
	"strings"
)

// Machine-readable error codes used in ExitError.Code and JSON output.
const (
	CodeUserError    = "user_error"
	CodeAuthError    = "auth_error"
	CodeNotFound     = "not_found"
	CodeRateLimited  = "rate_limited"
	CodeInvalidField = "invalid_field"
)

// ExitError is an error with a semantic exit code, machine-readable code,
// human-readable message, and an optional recovery suggestion.
// It implements the error interface and carries all information needed
// for both JSON and human error formatting.
type ExitError struct {
	// ExitCode is the semantic exit code (0-130) for the process.
	ExitCode int

	// Code is the machine-readable error code (e.g., "auth_error", "not_found").
	Code string

	// Message is the human-readable error message, always in English per D-05.
	Message string

	// Suggestion is a copy-paste-ready recovery hint per D-04.
	// May be empty if no specific recovery action is available.
	Suggestion string
}

// Error returns the human-readable error message.
func (e *ExitError) Error() string {
	return e.Message
}

// JSONError returns the JSON representation of the error for --json mode.
// The suggestion field is omitted when empty.
func (e *ExitError) JSONError() ([]byte, error) {
	return json.Marshal(struct {
		Code       string `json:"code"`
		Message    string `json:"message"`
		Suggestion string `json:"suggestion,omitempty"`
	}{
		Code:       e.Code,
		Message:    e.Message,
		Suggestion: e.Suggestion,
	})
}

// NewUserError creates an ExitError for invalid input or general user mistakes.
func NewUserError(message, suggestion string) *ExitError {
	return &ExitError{
		ExitCode:   ExitUserError,
		Code:       CodeUserError,
		Message:    message,
		Suggestion: suggestion,
	}
}

// NewAuthError creates an ExitError for authentication or authorization failures.
func NewAuthError(message, suggestion string) *ExitError {
	return &ExitError{
		ExitCode:   ExitAuthError,
		Code:       CodeAuthError,
		Message:    message,
		Suggestion: suggestion,
	}
}

// NewNotFoundError creates an ExitError for resources that do not exist.
func NewNotFoundError(message, suggestion string) *ExitError {
	return &ExitError{
		ExitCode:   ExitNotFound,
		Code:       CodeNotFound,
		Message:    message,
		Suggestion: suggestion,
	}
}

// NewRateLimitedError creates an ExitError for API rate limit exceeded.
func NewRateLimitedError(message, suggestion string) *ExitError {
	return &ExitError{
		ExitCode:   ExitRateLimited,
		Code:       CodeRateLimited,
		Message:    message,
		Suggestion: suggestion,
	}
}

// InvalidFieldError extends ExitError with field-specific validation details.
// Used when a user requests a JSON field name that does not exist.
type InvalidFieldError struct {
	ExitError

	InvalidField string   `json:"invalidField"`
	ValidFields  []string `json:"validFields"`
}

// JSONError returns JSON with invalid_field code, the bad field, and valid field list.
func (e *InvalidFieldError) JSONError() ([]byte, error) {
	return json.Marshal(struct {
		Code         string   `json:"code"`
		Message      string   `json:"message"`
		InvalidField string   `json:"invalidField"`
		ValidFields  []string `json:"validFields"`
		Suggestion   string   `json:"suggestion"`
	}{
		Code:         CodeInvalidField,
		Message:      e.Message,
		InvalidField: e.InvalidField,
		ValidFields:  e.ValidFields,
		Suggestion:   e.Suggestion,
	})
}

// NewInvalidFieldError creates an error for an unrecognized JSON field name.
func NewInvalidFieldError(field string, validFields []string) *InvalidFieldError {
	return &InvalidFieldError{
		ExitError: ExitError{
			ExitCode:   ExitUserError,
			Code:       CodeInvalidField,
			Message:    fmt.Sprintf("unknown field: %q", field),
			Suggestion: "Valid fields: " + strings.Join(validFields, ", "),
		},
		InvalidField: field,
		ValidFields:  validFields,
	}
}
