package faketracker

import (
	"bytes"
	"errors"
	"fmt"
	"io"
	"net/http"
	"sync"
)

// recordedHeaders are the response headers a fixture keeps: the ones the
// client reads. The rest are server noise, and some, like cookies, are
// personal.
var recordedHeaders = []string{
	"Content-Type", "Link", "X-Total-Count", "X-Total-Pages", "X-Scroll-Id", "X-Scroll-Token", "Retry-After",
}

// Recorder is an http.RoundTripper that sends GET requests through base and
// keeps each exchange for Save. It refuses every other method without sending
// it, so recording can never change the real Tracker.
type Recorder struct {
	base      http.RoundTripper
	mu        sync.Mutex
	exchanges []Exchange
	refused   error
}

// NewRecorder returns a Recorder that sends through base.
func NewRecorder(base http.RoundTripper) *Recorder {
	return &Recorder{base: base}
}

// RoundTrip sends a GET request through base and records the exchange.
func (r *Recorder) RoundTrip(req *http.Request) (*http.Response, error) {
	if req.Method != http.MethodGet {
		if req.Body != nil {
			_ = req.Body.Close()
		}

		err := fmt.Errorf("faketracker: refused to send %s %s: recording sends only GET",
			req.Method, req.URL.EscapedPath())
		r.mu.Lock()
		r.refused = errors.Join(r.refused, err)
		r.mu.Unlock()

		return nil, err
	}

	resp, err := r.base.RoundTrip(req)
	if err != nil {
		return nil, err
	}

	body, err := io.ReadAll(resp.Body)
	_ = resp.Body.Close()
	if err != nil {
		return nil, err
	}
	resp.Body = io.NopCloser(bytes.NewReader(body))

	ex := Exchange{
		Method: req.Method,
		Path:   req.URL.EscapedPath(),
		Query:  req.URL.Query(),
		Status: resp.StatusCode,
		Header: http.Header{},
		Body:   body,
	}
	for _, name := range recordedHeaders {
		if values := resp.Header.Values(name); len(values) > 0 {
			ex.Header[name] = values
		}
	}

	r.mu.Lock()
	r.exchanges = append(r.exchanges, ex)
	r.mu.Unlock()

	return resp, nil
}

// Save writes the recorded exchanges to path the way the package-level Save
// does. It writes nothing once the Recorder has refused a request.
func (r *Recorder) Save(path string, secrets ...Secret) error {
	r.mu.Lock()
	defer r.mu.Unlock()

	if r.refused != nil {
		return fmt.Errorf("faketracker: not writing %s: %w", path, r.refused)
	}

	return Save(path, r.exchanges, secrets...)
}
