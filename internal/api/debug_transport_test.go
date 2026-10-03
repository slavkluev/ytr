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
	ctx := output.NewContext(t.Context(), &output.Options{Debug: true, DebugOut: &buf})

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

	req, err := http.NewRequestWithContext(ctx, http.MethodPost,
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
		`[debug] response 500 method=POST path=/v2/issues?query_keys=perPage,signature duration=`,
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
	ctx := output.NewContext(t.Context(), &output.Options{Debug: true, DebugOut: &buf})

	transport := newDebugTransport(roundTripFunc(func(req *http.Request) (*http.Response, error) {
		return nil, errors.New("dial tcp: connection refused")
	}), "config")

	req, err := http.NewRequestWithContext(ctx, http.MethodGet,
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
	ctx := output.NewContext(t.Context(), &output.Options{Debug: true, DebugOut: &buf})

	transport := newDebugTransport(roundTripFunc(func(req *http.Request) (*http.Response, error) {
		return nil, errors.New("upstream rejected token abc123")
	}), "flag")

	req, err := http.NewRequestWithContext(ctx, http.MethodGet,
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
	ctx := output.NewContext(t.Context(), &output.Options{Debug: true, DebugOut: &buf})

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

	req, err := http.NewRequestWithContext(ctx, http.MethodGet,
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

func failingBase(t *testing.T) roundTripFunc {
	t.Helper()

	return func(req *http.Request) (*http.Response, error) {
		t.Errorf("base transport got %s %s, want the context transport to serve it", req.Method, req.URL)
		return nil, errors.New("base transport called")
	}
}

func okTransport(calls *int) roundTripFunc {
	return func(req *http.Request) (*http.Response, error) {
		*calls++
		return &http.Response{
			StatusCode: http.StatusOK,
			Header:     http.Header{},
			Body:       io.NopCloser(strings.NewReader(`[]`)),
			Request:    req,
		}, nil
	}
}

func TestDebugTransportSendsThroughContextTransport(t *testing.T) {
	calls := 0
	transport := newDebugTransport(failingBase(t), "flag")

	req, err := http.NewRequestWithContext(WithTransport(t.Context(), okTransport(&calls)),
		http.MethodGet, "https://api.tracker.yandex.net/v3/statuses", nil)
	if err != nil {
		t.Fatalf("http.NewRequest() returned error: %v", err)
	}

	resp, err := transport.RoundTrip(req)
	if err != nil {
		t.Fatalf("RoundTrip() returned error: %v", err)
	}
	defer resp.Body.Close()

	if calls != 1 {
		t.Errorf("context transport calls = %d, want 1", calls)
	}
}

func TestDebugTransportUsesBaseWithoutContextTransport(t *testing.T) {
	calls := 0
	transport := newDebugTransport(okTransport(&calls), "flag")

	req, err := http.NewRequestWithContext(t.Context(),
		http.MethodGet, "https://api.tracker.yandex.net/v3/statuses", nil)
	if err != nil {
		t.Fatalf("http.NewRequest() returned error: %v", err)
	}

	resp, err := transport.RoundTrip(req)
	if err != nil {
		t.Fatalf("RoundTrip() returned error: %v", err)
	}
	defer resp.Body.Close()

	if calls != 1 {
		t.Errorf("base transport calls = %d, want 1", calls)
	}
}

func TestDebugTransportLogsContextTransportExchange(t *testing.T) {
	var buf bytes.Buffer
	ctx := output.NewContext(t.Context(), &output.Options{Debug: true, DebugOut: &buf})

	calls := 0
	transport := newDebugTransport(failingBase(t), "flag")

	req, err := http.NewRequestWithContext(WithTransport(ctx, okTransport(&calls)),
		http.MethodGet, "https://api.tracker.yandex.net/v3/statuses", nil)
	if err != nil {
		t.Fatalf("http.NewRequest() returned error: %v", err)
	}

	resp, err := transport.RoundTrip(req)
	if err != nil {
		t.Fatalf("RoundTrip() returned error: %v", err)
	}
	defer resp.Body.Close()

	out := buf.String()
	for _, want := range []string{
		`[debug] request GET /v3/statuses auth_source=flag`,
		`[debug] response 200 method=GET path=/v3/statuses duration=`,
	} {
		if !strings.Contains(out, want) {
			t.Errorf("debug output missing %q in %q", want, out)
		}
	}
}
