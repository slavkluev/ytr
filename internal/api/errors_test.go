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
