package api_test

import (
	"bytes"
	"errors"
	"net/http"
	"net/url"
	"strings"
	"testing"

	"github.com/slavkluev/go-yandex-tracker/tracker"

	"github.com/slavkluev/ytr/internal/api"
	ytrerrors "github.com/slavkluev/ytr/internal/errors"
	"github.com/slavkluev/ytr/internal/output"
)

// newTrackerError creates a tracker.ErrorResponse with the given status code.
func newTrackerError(statusCode int, messages ...string) error {
	return &tracker.ErrorResponse{
		Response: &http.Response{
			StatusCode: statusCode,
			Request:    &http.Request{Method: http.MethodGet, URL: nil},
		},
		ErrorMessages: messages,
	}
}

func TestMapAPIError_Nil(t *testing.T) {
	if err := api.MapAPIError(nil); err != nil {
		t.Errorf("MapAPIError(nil) = %v, want nil", err)
	}
}

func TestMapAPIError_PreservesExitError(t *testing.T) {
	want := ytrerrors.NewAuthError("not authenticated", "Run: ytr auth login")

	got := api.MapAPIError(want)
	if !errors.Is(got, want) {
		t.Fatalf("MapAPIError() returned %p, want original %p", got, want)
	}
}

func TestMapAPIError_NotFound(t *testing.T) {
	apiErr := newTrackerError(http.StatusNotFound, "Issue not found")

	err := api.MapAPIError(apiErr)
	if err == nil {
		t.Fatal("MapAPIError() returned nil for not found error")
	}

	var exitErr *ytrerrors.ExitError
	if !errors.As(err, &exitErr) {
		t.Fatalf("error type = %T, want *ExitError", err)
	}
	if exitErr.Code != "not_found" {
		t.Errorf("Code = %q, want %q", exitErr.Code, "not_found")
	}
}

func TestMapAPIError_Forbidden(t *testing.T) {
	apiErr := newTrackerError(http.StatusForbidden, "Access denied")

	err := api.MapAPIError(apiErr)
	if err == nil {
		t.Fatal("MapAPIError() returned nil for forbidden error")
	}

	var exitErr *ytrerrors.ExitError
	if !errors.As(err, &exitErr) {
		t.Fatalf("error type = %T, want *ExitError", err)
	}
	if exitErr.Code != "auth_error" {
		t.Errorf("Code = %q, want %q", exitErr.Code, "auth_error")
	}
}

func TestMapAPIError_RateLimited(t *testing.T) {
	apiErr := newTrackerError(http.StatusTooManyRequests, "Rate limited")

	err := api.MapAPIError(apiErr)
	if err == nil {
		t.Fatal("MapAPIError() returned nil for rate limited error")
	}

	var exitErr *ytrerrors.ExitError
	if !errors.As(err, &exitErr) {
		t.Fatalf("error type = %T, want *ExitError", err)
	}
	if exitErr.Code != "rate_limited" {
		t.Errorf("Code = %q, want %q", exitErr.Code, "rate_limited")
	}
}

func TestMapAPIError_GenericError(t *testing.T) {
	genericErr := errors.New("connection refused")

	err := api.MapAPIError(genericErr)
	if err == nil {
		t.Fatal("MapAPIError() returned nil for generic error")
	}
	if !strings.Contains(err.Error(), "API request failed") {
		t.Errorf("error = %q, want to contain %q", err.Error(), "API request failed")
	}
}

func TestMapAPIError_DebugStructuredDetails(t *testing.T) {
	var buf bytes.Buffer
	output.DebugFlag = true
	output.SetDebugWriter(&buf)
	defer output.ResetFlags()

	u, err := url.Parse("https://api.tracker.yandex.net/v2/issues")
	if err != nil {
		t.Fatalf("url.Parse() returned error: %v", err)
	}

	apiErr := &tracker.ErrorResponse{
		Response: &http.Response{
			StatusCode: http.StatusBadRequest,
			Request: &http.Request{
				Method: http.MethodPost,
				URL:    u,
			},
		},
		ErrorMessages: []string{"Unknown field: token abc123"},
		Errors: map[string]string{
			"priority": "Bearer secret-value",
		},
	}

	_ = api.MapAPIError(apiErr)

	out := buf.String()
	for _, want := range []string{
		`[debug] api_error status=400 method=POST path=/v2/issues`,
		`[debug] api_error_messages messages=["Unknown field: token <redacted>"]`,
		`[debug] api_error_fields fields={"priority":"Bearer <redacted>"}`,
	} {
		if !strings.Contains(out, want) {
			t.Errorf("debug output missing %q in %q", want, out)
		}
	}

	for _, unwanted := range []string{"abc123", "secret-value"} {
		if strings.Contains(out, unwanted) {
			t.Errorf("debug output leaked sensitive value %q in %q", unwanted, out)
		}
	}
}

// newTrackerFieldError creates a tracker.ErrorResponse carrying field-level
// errors, the shape the API uses for 422 responses.
func newTrackerFieldError(statusCode int, fields map[string]string, messages ...string) error {
	return &tracker.ErrorResponse{
		Response: &http.Response{
			StatusCode: statusCode,
			Request:    &http.Request{Method: http.MethodPatch, URL: nil},
		},
		ErrorMessages: messages,
		Errors:        fields,
	}
}

func TestMapAPIError_FieldErrorsReachMessage(t *testing.T) {
	apiErr := newTrackerFieldError(http.StatusUnprocessableEntity, map[string]string{
		"66fd07bba913292094b4403c--size": "value is not allowed",
	})

	err := api.MapAPIError(apiErr)

	var exitErr *ytrerrors.ExitError
	if !errors.As(err, &exitErr) {
		t.Fatalf("error type = %T, want *ExitError", err)
	}
	want := "66fd07bba913292094b4403c--size: value is not allowed"
	if exitErr.Message != want {
		t.Errorf("Message = %q, want %q", exitErr.Message, want)
	}
}

func TestMapAPIError_FieldErrorsSortedByKey(t *testing.T) {
	want := "alpha: first; beta: second; gamma: third"

	// Map iteration order is randomized, so a single pass can pass by luck.
	for range 20 {
		apiErr := newTrackerFieldError(http.StatusUnprocessableEntity, map[string]string{
			"gamma": "third",
			"alpha": "first",
			"beta":  "second",
		})

		err := api.MapAPIError(apiErr)
		if err.Error() != want {
			t.Fatalf("Message = %q, want %q", err.Error(), want)
		}
	}
}

func TestMapAPIError_FieldErrorsJoinErrorMessages(t *testing.T) {
	apiErr := newTrackerFieldError(
		http.StatusUnprocessableEntity,
		map[string]string{"size": "value is not allowed"},
		"Validation failed",
	)

	err := api.MapAPIError(apiErr)

	want := "Validation failed; size: value is not allowed"
	if err.Error() != want {
		t.Errorf("Message = %q, want %q", err.Error(), want)
	}
}

func TestMapAPIError_ServerMessageSurvivesStatusBranches(t *testing.T) {
	tests := []struct {
		name       string
		statusCode int
		message    string
		wantCode   string
	}{
		{"not found", http.StatusNotFound, "Issue MTP-1 does not exist", ytrerrors.CodeNotFound},
		{"forbidden", http.StatusForbidden, "No write access to queue MTP", ytrerrors.CodeAuthError},
		{"rate limited", http.StatusTooManyRequests, "Too many requests, retry in 30s", ytrerrors.CodeRateLimited},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := api.MapAPIError(newTrackerError(tt.statusCode, tt.message))

			var exitErr *ytrerrors.ExitError
			if !errors.As(err, &exitErr) {
				t.Fatalf("error type = %T, want *ExitError", err)
			}
			if exitErr.Message != tt.message {
				t.Errorf("Message = %q, want %q", exitErr.Message, tt.message)
			}
			if exitErr.Code != tt.wantCode {
				t.Errorf("Code = %q, want %q", exitErr.Code, tt.wantCode)
			}
			if exitErr.Suggestion == "" {
				t.Error("Suggestion is empty, want the recovery hint preserved")
			}
		})
	}
}

func TestMapAPIError_StatusFallbacksWithoutServerMessage(t *testing.T) {
	tests := []struct {
		name       string
		statusCode int
		want       string
	}{
		{"not found", http.StatusNotFound, "resource not found"},
		{"forbidden", http.StatusForbidden, "access denied"},
		{"rate limited", http.StatusTooManyRequests, "API rate limit exceeded"},
		{"unprocessable", http.StatusUnprocessableEntity, "API error (HTTP 422)"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := api.MapAPIError(newTrackerError(tt.statusCode))
			if err.Error() != tt.want {
				t.Errorf("Message = %q, want %q", err.Error(), tt.want)
			}
		})
	}
}

func TestMapAPIError_MissingResponse(t *testing.T) {
	apiErr := &tracker.ErrorResponse{
		Errors: map[string]string{"size": "value is not allowed"},
	}

	err := api.MapAPIError(apiErr)

	want := "size: value is not allowed"
	if err == nil || err.Error() != want {
		t.Errorf("Message = %v, want %q", err, want)
	}
}
