package faketracker

import (
	"encoding/json"
	"fmt"
	"io"
	"maps"
	"net/http"
	"net/http/httptest"
	"net/url"
	"slices"
	"sync"
	"testing"
)

// Request is what Fake received: what Tracker would see of a request, less
// its headers.
type Request struct {
	Method string
	Path   string
	Query  url.Values
	Body   string
}

// Fake is an http.RoundTripper that answers each request with the first
// unused exchange of equal method, path and query, and records every request
// it gets. It fails its test on a request no exchange matches and, when the
// test ends, on every exchange no request used.
type Fake struct {
	t         testing.TB
	mu        sync.Mutex
	exchanges []Exchange
	used      []bool
	requests  []Request
}

// New returns a Fake serving exchanges, checked against t. It fails t at once
// on an exchange that lacks a method, a path or a valid status.
func New(t testing.TB, exchanges []Exchange) *Fake {
	t.Helper()

	for i, ex := range exchanges {
		if ex.Method == "" || ex.Path == "" || http.StatusText(ex.Status) == "" {
			t.Fatalf("faketracker: exchange %d needs a method, a path and a known status: %+v", i, ex)
		}
	}

	f := &Fake{
		t:         t,
		exchanges: exchanges,
		used:      make([]bool, len(exchanges)),
	}
	t.Cleanup(f.checkAllUsed)

	return f
}

// RoundTrip serves req in process, so no port opens and the client's URL
// stays the real Tracker's.
func (f *Fake) RoundTrip(req *http.Request) (*http.Response, error) {
	var body []byte
	if req.Body != nil {
		data, err := io.ReadAll(req.Body)
		_ = req.Body.Close()
		if err != nil {
			return nil, err
		}
		body = data
	}

	got := Request{
		Method: req.Method,
		Path:   req.URL.EscapedPath(),
		Query:  req.URL.Query(),
		Body:   string(body),
	}

	rec := httptest.NewRecorder()
	f.serve(rec, got)

	resp := rec.Result()
	resp.Request = req

	return resp, nil
}

// Requests returns every request Fake received, in order.
func (f *Fake) Requests() []Request {
	f.mu.Lock()
	defer f.mu.Unlock()

	return slices.Clone(f.requests)
}

func (f *Fake) serve(w http.ResponseWriter, got Request) {
	f.mu.Lock()
	f.requests = append(f.requests, got)
	ex, ok := f.take(got)
	f.mu.Unlock()

	if !ok {
		f.t.Errorf("faketracker: no exchange matches %s", describe(got.Method, got.Path, got.Query))
		writeUnmatched(w, got)
		return
	}

	maps.Copy(w.Header(), ex.Header.Clone())
	w.WriteHeader(ex.Status)
	_, _ = w.Write(ex.Body)
}

func (f *Fake) take(got Request) (Exchange, bool) {
	for i, ex := range f.exchanges {
		if !f.used[i] && ex.Method == got.Method && ex.Path == got.Path && sameQuery(ex.Query, got.Query) {
			f.used[i] = true
			return ex, true
		}
	}

	return Exchange{}, false
}

func (f *Fake) checkAllUsed() {
	f.mu.Lock()
	defer f.mu.Unlock()

	for i, ex := range f.exchanges {
		if !f.used[i] {
			f.t.Errorf("faketracker: exchange %d, %s, was never requested", i, describe(ex.Method, ex.Path, ex.Query))
		}
	}
}

func sameQuery(a, b url.Values) bool {
	return maps.EqualFunc(a, b, slices.Equal)
}

func describe(method, path string, query url.Values) string {
	return fmt.Sprintf("%s %s query %q", method, path, query.Encode())
}

// writeUnmatched answers in Tracker's error shape, so the command fails the
// way it would on a real error instead of on a body it cannot decode.
func writeUnmatched(w http.ResponseWriter, got Request) {
	body, _ := json.Marshal(map[string]any{
		"errorMessages": []string{"faketracker: no exchange matches " + describe(got.Method, got.Path, got.Query)},
		"errors":        map[string]string{},
	})

	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(http.StatusNotImplemented)
	_, _ = w.Write(body)
}
