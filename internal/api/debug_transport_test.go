package api

import (
	"bytes"
	"errors"
	"io"
	"net/http"
	"strings"
	"testing"

	"github.com/slavkluev/ytr/internal/output"
)

type roundTripFunc func(*http.Request) (*http.Response, error)

func (fn roundTripFunc) RoundTrip(req *http.Request) (*http.Response, error) {
	return fn(req)
}

type countingReadCloser struct {
	reader io.Reader
	reads  int
}

func (c *countingReadCloser) Read(p []byte) (int, error) {
	c.reads++
	return c.reader.Read(p)
}

func (c *countingReadCloser) Close() error {
	return nil
}

type errorAfterFirstReadCloser struct {
	data []byte
	read bool
}

func (e *errorAfterFirstReadCloser) Read(p []byte) (int, error) {
	if e.read {
		return 0, errors.New("injected read error")
	}

	e.read = true
	n := copy(p, e.data)
	return n, errors.New("injected read error")
}

func (e *errorAfterFirstReadCloser) Close() error {
	return nil
}

func TestDebugTransportLogsRequestAndResponse(t *testing.T) {
	var buf bytes.Buffer
	output.DebugFlag = true
	output.SetDebugWriter(&buf)
	defer output.ResetFlags()

	transport := newDebugTransport(roundTripFunc(func(req *http.Request) (*http.Response, error) {
		return &http.Response{
			StatusCode: http.StatusInternalServerError,
			Header: http.Header{
				"Content-Type": []string{"text/plain"},
				"X-Request-Id": []string{"req-123"},
			},
			Body:    io.NopCloser(strings.NewReader("Internal Server Error")),
			Request: req,
		}, nil
	}), "env")

	req, err := http.NewRequest(http.MethodPost,
		"https://api.tracker.yandex.net/v2/issues?perPage=50&signature=secret-value",
		strings.NewReader(`{"queue":"PROJ","summary":"Bug","priority":"critical"}`))
	if err != nil {
		t.Fatalf("http.NewRequest() returned error: %v", err)
	}
	req.Header.Set("Content-Type", "application/json")

	resp, err := transport.RoundTrip(req)
	if err != nil {
		t.Fatalf("RoundTrip() returned error: %v", err)
	}

	data, err := io.ReadAll(resp.Body)
	if err != nil {
		t.Fatalf("io.ReadAll() returned error: %v", err)
	}

	if got := string(data); got != "Internal Server Error" {
		t.Fatalf("response body = %q, want %q", got, "Internal Server Error")
	}

	out := buf.String()
	for _, want := range []string{
		`[debug] request POST /v2/issues?query_keys=perPage,signature auth_source=env body=json_keys=priority,queue,summary`,
		`[debug] response 500 duration=`,
		`request_id=req-123`,
		`[debug] response_preview text="Internal Server Error"`,
	} {
		if !strings.Contains(out, want) {
			t.Errorf("debug output missing %q in %q", want, out)
		}
	}

	if strings.Contains(out, "secret-value") {
		t.Errorf("debug output leaked query secret in %q", out)
	}
}

func TestDebugTransportLogsTransportError(t *testing.T) {
	var buf bytes.Buffer
	output.DebugFlag = true
	output.SetDebugWriter(&buf)
	defer output.ResetFlags()

	transport := newDebugTransport(roundTripFunc(func(req *http.Request) (*http.Response, error) {
		return nil, errors.New("dial tcp: connection refused")
	}), "config")

	req, err := http.NewRequest(http.MethodGet,
		"https://api.tracker.yandex.net/v2/issues/TEST-1", nil)
	if err != nil {
		t.Fatalf("http.NewRequest() returned error: %v", err)
	}

	if _, err := transport.RoundTrip(req); err == nil {
		t.Fatal("RoundTrip() error = nil, want transport error")
	}

	out := buf.String()
	for _, want := range []string{
		`[debug] request GET /v2/issues/TEST-1 auth_source=config`,
		`[debug] transport_error method=GET path=/v2/issues/TEST-1 duration=`,
		`error="dial tcp: connection refused"`,
	} {
		if !strings.Contains(out, want) {
			t.Errorf("debug output missing %q in %q", want, out)
		}
	}
}

func TestDebugTransportLogsSanitizedTransportError(t *testing.T) {
	var buf bytes.Buffer
	output.DebugFlag = true
	output.SetDebugWriter(&buf)
	defer output.ResetFlags()

	transport := newDebugTransport(roundTripFunc(func(req *http.Request) (*http.Response, error) {
		return nil, errors.New("upstream rejected token abc123")
	}), "flag")

	req, err := http.NewRequest(http.MethodGet,
		"https://api.tracker.yandex.net/v2/issues/TEST-2", nil)
	if err != nil {
		t.Fatalf("http.NewRequest() returned error: %v", err)
	}

	if _, err := transport.RoundTrip(req); err == nil {
		t.Fatal("RoundTrip() error = nil, want transport error")
	}

	out := buf.String()
	if strings.Contains(out, "abc123") {
		t.Fatalf("debug output leaked sensitive token in %q", out)
	}

	if !strings.Contains(out, `error="upstream rejected token <redacted>"`) {
		t.Fatalf("sanitized transport error missing in %q", out)
	}
}

func TestDebugTransportSkipsBodyPreviewForClientErrors(t *testing.T) {
	var buf bytes.Buffer
	output.DebugFlag = true
	output.SetDebugWriter(&buf)
	defer output.ResetFlags()

	body := &countingReadCloser{
		reader: strings.NewReader(`{"errorMessages":["bad request"]}`),
	}

	transport := newDebugTransport(roundTripFunc(func(req *http.Request) (*http.Response, error) {
		return &http.Response{
			StatusCode: http.StatusBadRequest,
			Header: http.Header{
				"Content-Type": []string{"application/json"},
			},
			Body:    body,
			Request: req,
		}, nil
	}), "env")

	req, err := http.NewRequest(http.MethodGet,
		"https://api.tracker.yandex.net/v2/issues/TEST-3", nil)
	if err != nil {
		t.Fatalf("http.NewRequest() returned error: %v", err)
	}

	if _, err := transport.RoundTrip(req); err != nil {
		t.Fatalf("RoundTrip() returned error: %v", err)
	}

	if body.reads != 0 {
		t.Fatalf("response body was read for 4xx preview path, reads = %d", body.reads)
	}
}

func TestSnapshotResponsePreviewPreservesBodyOnReadError(t *testing.T) {
	resp := &http.Response{
		StatusCode: http.StatusInternalServerError,
		Header: http.Header{
			"Content-Type": []string{"text/plain"},
		},
		Body: &errorAfterFirstReadCloser{
			data: []byte("partial body"),
		},
	}

	if got := snapshotResponsePreview(resp); got != "" {
		t.Fatalf("snapshotResponsePreview() = %q, want empty preview on read error", got)
	}

	data, err := io.ReadAll(resp.Body)
	if err == nil {
		t.Fatal("io.ReadAll() error = nil, want preserved read error")
	}

	if got := string(data); got != "partial body" {
		t.Fatalf("restored response body = %q, want %q", got, "partial body")
	}
}
