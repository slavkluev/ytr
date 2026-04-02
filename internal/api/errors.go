package api

import (
	"bytes"
	"encoding/json"
	stderrors "errors"
	"fmt"
	"strings"

	"github.com/slavkluev/go-yandex-tracker/tracker"

	ytrerrors "github.com/slavkluev/ytr/internal/errors"
	"github.com/slavkluev/ytr/internal/output"
)

// MapAPIError translates a go-yandex-tracker API error into the appropriate
// ytr ExitError type. Returns nil if err is nil. Preserves existing ExitError
// values, checks for specific HTTP status codes (404, 403, 429), then falls
// back to extracting error messages from ErrorResponse, and finally wraps
// unknown errors.
func MapAPIError(err error) error {
	if err == nil {
		return nil
	}

	var exitErr *ytrerrors.ExitError
	if stderrors.As(err, &exitErr) {
		return exitErr
	}

	debugAPIError(err)

	if tracker.IsNotFound(err) {
		return ytrerrors.NewNotFoundError(
			"resource not found",
			"Check the issue key or queue name",
		)
	}

	if tracker.IsForbidden(err) {
		return ytrerrors.NewAuthError(
			"access denied",
			"Check your permissions for this resource",
		)
	}

	if tracker.IsRateLimited(err) {
		return ytrerrors.NewRateLimitedError(
			"API rate limit exceeded",
			"Wait and retry",
		)
	}

	// Try to extract message from ErrorResponse
	var errResp *tracker.ErrorResponse
	if stderrors.As(err, &errResp) {
		msg := strings.Join(errResp.ErrorMessages, "; ")
		if msg == "" {
			msg = fmt.Sprintf("API error (HTTP %d)", errResp.Response.StatusCode)
		}
		return ytrerrors.NewUserError(msg, "")
	}

	return fmt.Errorf("API request failed: %w", err)
}

func debugAPIError(err error) {
	if !output.DebugEnabled() {
		return
	}

	var errResp *tracker.ErrorResponse
	if !stderrors.As(err, &errResp) || errResp.Response == nil {
		return
	}

	method := "-"
	path := "-"
	if errResp.Response.Request != nil {
		method = errResp.Response.Request.Method
		path = requestPath(errResp.Response.Request.URL)
	}

	output.Debugf("api_error status=%d method=%s path=%s",
		errResp.Response.StatusCode, method, path)

	if len(errResp.ErrorMessages) > 0 {
		output.Debugf("api_error_messages messages=%s",
			formatStringSlice(output.SanitizeDebugStrings(errResp.ErrorMessages)))
	}

	if len(errResp.Errors) > 0 {
		output.Debugf("api_error_fields fields=%s",
			formatStringMap(output.SanitizeDebugMap(errResp.Errors)))
	}
}

func formatStringSlice(values []string) string {
	data, err := marshalDebugJSON(values)
	if err != nil {
		return "[]"
	}

	return data
}

func formatStringMap(values map[string]string) string {
	data, err := marshalDebugJSON(values)
	if err != nil {
		return "{}"
	}

	return data
}

func marshalDebugJSON(value any) (string, error) {
	var buf bytes.Buffer
	enc := json.NewEncoder(&buf)
	enc.SetEscapeHTML(false)
	if err := enc.Encode(value); err != nil {
		return "", err
	}

	return strings.TrimSpace(buf.String()), nil
}
