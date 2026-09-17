package api

import (
	"bytes"
	"encoding/json"
	stderrors "errors"
	"fmt"
	"net/http"
	"sort"
	"strings"

	"github.com/slavkluev/go-yandex-tracker/tracker"

	ytrerrors "github.com/slavkluev/ytr/internal/errors"
	"github.com/slavkluev/ytr/internal/output"
)

// MapAPIError translates a go-yandex-tracker API error into the appropriate
// ytr ExitError type. Returns nil if err is nil. Preserves existing ExitError
// values, then maps the HTTP status code to a semantic exit code, keeping the
// message the server sent and falling back to a generic one only when the
// response body carried no text at all.
func MapAPIError(err error) error {
	if err == nil {
		return nil
	}

	var exitErr *ytrerrors.ExitError
	if stderrors.As(err, &exitErr) {
		return exitErr
	}

	debugAPIError(err)

	var errResp *tracker.ErrorResponse
	if !stderrors.As(err, &errResp) {
		return fmt.Errorf("API request failed: %w", err)
	}

	msg := apiErrorMessage(errResp)

	if errResp.Response == nil {
		return ytrerrors.NewUserError(orFallback(msg, "API error"), "")
	}

	switch errResp.Response.StatusCode {
	case http.StatusNotFound:
		return ytrerrors.NewNotFoundError(
			orFallback(msg, "resource not found"),
			"Check the issue key or queue name",
		)
	case http.StatusForbidden:
		return ytrerrors.NewAuthError(
			orFallback(msg, "access denied"),
			"Check your permissions for this resource",
		)
	case http.StatusTooManyRequests:
		return ytrerrors.NewRateLimitedError(
			orFallback(msg, "API rate limit exceeded"),
			"Wait and retry",
		)
	}

	return ytrerrors.NewUserError(
		orFallback(msg, fmt.Sprintf("API error (HTTP %d)", errResp.Response.StatusCode)),
		"",
	)
}

// apiErrorMessage joins everything the API said about the failure: the
// top-level errorMessages first, then the field-level errors map as
// "field: reason" pairs. Keys are sorted because Go randomizes map iteration
// order, and an error message that reshuffles between runs is untestable.
func apiErrorMessage(errResp *tracker.ErrorResponse) string {
	parts := make([]string, 0, len(errResp.ErrorMessages)+len(errResp.Errors))
	parts = append(parts, errResp.ErrorMessages...)

	keys := make([]string, 0, len(errResp.Errors))
	for key := range errResp.Errors {
		keys = append(keys, key)
	}
	sort.Strings(keys)

	for _, key := range keys {
		parts = append(parts, fmt.Sprintf("%s: %s", key, errResp.Errors[key]))
	}

	return strings.Join(parts, "; ")
}

func orFallback(value, fallback string) string {
	if value == "" {
		return fallback
	}

	return value
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
