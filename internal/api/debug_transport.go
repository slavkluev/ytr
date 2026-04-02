package api

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"sort"
	"strconv"
	"strings"
	"time"

	"github.com/slavkluev/ytr/internal/output"
)

const (
	debugRequestBodyLimit  = 4096
	debugResponseTextLimit = 160
)

type replayReadCloser struct {
	io.Reader
	io.Closer
}

type debugTransport struct {
	base       http.RoundTripper
	authSource string
}

func newDebugTransport(base http.RoundTripper, authSource string) http.RoundTripper {
	if base == nil {
		base = http.DefaultTransport
	}

	return &debugTransport{
		base:       base,
		authSource: authSource,
	}
}

func (t *debugTransport) RoundTrip(req *http.Request) (*http.Response, error) {
	if !output.DebugEnabled() {
		return t.base.RoundTrip(req)
	}

	path := requestPath(req.URL)
	authSource := t.authSource
	if authSource == "" {
		authSource = "unknown"
	}

	if bodySummary := summarizeRequestBody(req); bodySummary != "" {
		output.Debugf("request %s %s auth_source=%s %s",
			req.Method, path, authSource, bodySummary)
	} else {
		output.Debugf("request %s %s auth_source=%s",
			req.Method, path, authSource)
	}

	start := time.Now()
	resp, err := t.base.RoundTrip(req)
	duration := formatDebugDuration(time.Since(start))
	if err != nil {
		output.Debugf("transport_error method=%s path=%s duration=%s error=%q",
			req.Method, path, duration, output.SanitizeDebugString(err.Error()))
		return nil, err
	}

	responsePreview := ""
	if resp.StatusCode >= http.StatusInternalServerError {
		responsePreview = snapshotResponsePreview(resp)
	}

	requestID := output.SanitizeDebugString(extractRequestID(resp.Header))
	if requestID != "" {
		output.Debugf("response %d duration=%s request_id=%s",
			resp.StatusCode, duration, requestID)
	} else {
		output.Debugf("response %d duration=%s", resp.StatusCode, duration)
	}

	if responsePreview != "" {
		output.Debugf("response_preview text=%q", responsePreview)
	}

	return resp, nil
}

func requestPath(u *url.URL) string {
	if u == nil {
		return "-"
	}

	path := u.EscapedPath()
	if path == "" {
		path = "/"
	}

	if u.RawQuery == "" {
		return path
	}

	queryValues, err := url.ParseQuery(u.RawQuery)
	if err != nil || len(queryValues) == 0 {
		return path + "?query=redacted"
	}

	keys := make([]string, 0, len(queryValues))
	for key := range queryValues {
		keys = append(keys, key)
	}
	sort.Strings(keys)

	return path + "?query_keys=" + strings.Join(keys, ",")
}

func summarizeRequestBody(req *http.Request) string {
	if req == nil {
		return ""
	}

	if req.GetBody == nil {
		if req.ContentLength > 0 {
			return "body_bytes=" + strconv.FormatInt(req.ContentLength, 10)
		}
		return ""
	}

	body, err := req.GetBody()
	if err != nil {
		return "body=unavailable"
	}
	defer body.Close()

	data, err := io.ReadAll(io.LimitReader(body, debugRequestBodyLimit+1))
	if err != nil || len(data) == 0 {
		return ""
	}

	if len(data) > debugRequestBodyLimit {
		data = data[:debugRequestBodyLimit]
	}

	trimmed := bytes.TrimSpace(data)
	if len(trimmed) == 0 {
		return ""
	}

	if strings.Contains(req.Header.Get("Content-Type"), "application/json") ||
		json.Valid(trimmed) {
		var object map[string]json.RawMessage
		if err := json.Unmarshal(trimmed, &object); err == nil {
			keys := make([]string, 0, len(object))
			for key := range object {
				keys = append(keys, key)
			}
			sort.Strings(keys)
			return "body=json_keys=" + strings.Join(keys, ",")
		}

		var array []json.RawMessage
		if err := json.Unmarshal(trimmed, &array); err == nil {
			return fmt.Sprintf("body=json_array_len=%d", len(array))
		}

		return fmt.Sprintf("body=json_bytes=%d", len(trimmed))
	}

	return fmt.Sprintf("body_bytes=%d", len(trimmed))
}

func snapshotResponsePreview(resp *http.Response) string {
	if resp == nil || resp.Body == nil {
		return ""
	}

	originalBody := resp.Body
	data, err := io.ReadAll(io.LimitReader(originalBody, debugResponseTextLimit+1))
	resp.Body = &replayReadCloser{
		Reader: io.MultiReader(bytes.NewReader(data), originalBody),
		Closer: originalBody,
	}

	if err != nil {
		return ""
	}

	return summarizeResponseBody(resp, data)
}

func summarizeResponseBody(resp *http.Response, data []byte) string {
	if resp == nil || resp.StatusCode < http.StatusInternalServerError {
		return ""
	}

	truncated := false
	if len(data) > debugResponseTextLimit {
		data = data[:debugResponseTextLimit]
		truncated = true
	}

	trimmed := bytes.TrimSpace(data)
	if len(trimmed) == 0 {
		return ""
	}

	contentType := resp.Header.Get("Content-Type")
	if strings.Contains(contentType, "json") || json.Valid(trimmed) {
		return ""
	}

	text := strings.Join(strings.Fields(string(trimmed)), " ")
	if text == "" {
		return ""
	}

	text = output.SanitizeDebugString(text)
	if truncated {
		return text + "..."
	}

	return text
}

func extractRequestID(header http.Header) string {
	for _, key := range []string{
		"X-Request-Id",
		"X-Req-Id",
		"X-Ya-Request-Id",
		"X-YaRequestId",
	} {
		if value := header.Get(key); value != "" {
			return value
		}
	}

	return ""
}

func formatDebugDuration(duration time.Duration) string {
	if duration >= time.Millisecond {
		return duration.Round(time.Millisecond).String()
	}

	return duration.String()
}
