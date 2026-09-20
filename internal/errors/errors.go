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
	CodeBulkFailed   = "bulk_failed"
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

	// Message is the human-readable error message, always in English.
	Message string

	// Suggestion is a copy-paste-ready recovery hint.
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

	// InvalidField is the single offending field name. Empty when several
	// fields were rejected at once; see InvalidFields.
	InvalidField string `json:"invalidField,omitempty"`

	// InvalidFields holds every offending field name when more than one was
	// rejected, as happens for a JSON request body.
	InvalidFields []string `json:"invalidFields,omitempty"`

	ValidFields []string `json:"validFields"`
}

// Unwrap exposes the embedded ExitError so errors.As/Is can traverse the
// chain. InvalidFieldError embeds ExitError by value, which on its own does
// not satisfy errors.As(err, **ExitError); Unwrap makes the relationship
// explicit. The field-specific JSON comes from JSONError below, which
// handleError finds through an interface rather than by matching this type.
func (e *InvalidFieldError) Unwrap() error {
	return &e.ExitError
}

// JSONError returns JSON with invalid_field code, the bad field, and valid field list.
func (e *InvalidFieldError) JSONError() ([]byte, error) {
	return json.Marshal(struct {
		Code          string   `json:"code"`
		Message       string   `json:"message"`
		InvalidField  string   `json:"invalidField,omitempty"`
		InvalidFields []string `json:"invalidFields,omitempty"`
		ValidFields   []string `json:"validFields"`
		Suggestion    string   `json:"suggestion"`
	}{
		Code:          CodeInvalidField,
		Message:       e.Message,
		InvalidField:  e.InvalidField,
		InvalidFields: e.InvalidFields,
		ValidFields:   e.ValidFields,
		Suggestion:    e.Suggestion,
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

// NewUnknownFieldsError creates an error for JSON input carrying keys the
// request body has no field for. Unlike NewInvalidFieldError it reports every
// offending key at once, so a single run names all of them.
func NewUnknownFieldsError(fields, validFields []string) *InvalidFieldError {
	return &InvalidFieldError{
		ExitError: ExitError{
			ExitCode:   ExitUserError,
			Code:       CodeInvalidField,
			Message:    "unknown fields in JSON input: " + strings.Join(fields, ", "),
			Suggestion: "Valid fields: " + strings.Join(validFields, ", "),
		},
		InvalidFields: fields,
		ValidFields:   validFields,
	}
}

// BulkFailedError extends ExitError with what a bulk operation reported when
// it finished in the FAILED state. A failed operation writes nothing to
// stdout, so these counts are the only place an agent learns how much of the
// change landed.
type BulkFailedError struct {
	ExitError

	// OperationID is the bulk change the counts belong to.
	OperationID string `json:"operationId"`

	// StatusText is the API's own description of the failure, empty when it
	// gave none.
	StatusText string `json:"statusText"`

	// TotalIssues and TotalCompletedIssues are the operation's final counts.
	TotalIssues          int `json:"totalIssues"`
	TotalCompletedIssues int `json:"totalCompletedIssues"`
}

// Unwrap exposes the embedded ExitError so errors.As/Is can traverse the
// chain, for the same reason InvalidFieldError does.
func (e *BulkFailedError) Unwrap() error {
	return &e.ExitError
}

// JSONError returns JSON with the bulk_failed code and the operation's counts.
func (e *BulkFailedError) JSONError() ([]byte, error) {
	return json.Marshal(struct {
		Code                 string `json:"code"`
		Message              string `json:"message"`
		OperationID          string `json:"operationId"`
		StatusText           string `json:"statusText"`
		TotalIssues          int    `json:"totalIssues"`
		TotalCompletedIssues int    `json:"totalCompletedIssues"`
		Suggestion           string `json:"suggestion"`
	}{
		Code:                 CodeBulkFailed,
		Message:              e.Message,
		OperationID:          e.OperationID,
		StatusText:           e.StatusText,
		TotalIssues:          e.TotalIssues,
		TotalCompletedIssues: e.TotalCompletedIssues,
		Suggestion:           e.Suggestion,
	})
}

// NewBulkFailedError creates an error for a bulk operation that reached the
// FAILED state. The message repeats the counts so human output, which shows
// only the message and the suggestion, says as much as the JSON document.
func NewBulkFailedError(
	operationID, statusText string,
	totalIssues, totalCompletedIssues int,
) *BulkFailedError {
	message := fmt.Sprintf("bulk operation %s failed", operationID)
	if statusText != "" {
		message += ": " + statusText
	}
	message += fmt.Sprintf(
		" (%d of %d issues completed)", totalCompletedIssues, totalIssues,
	)

	return &BulkFailedError{
		ExitError: ExitError{
			ExitCode:   ExitUserError,
			Code:       CodeBulkFailed,
			Message:    message,
			Suggestion: "ytr bulk status " + operationID,
		},
		OperationID:          operationID,
		StatusText:           statusText,
		TotalIssues:          totalIssues,
		TotalCompletedIssues: totalCompletedIssues,
	}
}
