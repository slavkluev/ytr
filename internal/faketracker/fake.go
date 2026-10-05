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

// Request is what Fake received: what Tracker would see of a request.
type Request struct {
	Method string
	Path   string
	Query  url.Values
	Header http.Header
	Body   string
}

// Fake is an http.RoundTripper that records every request it gets. One from
// New answers each request with the first unused exchange of equal method,
// path and query; it fails its test on a request no exchange matches and,
// when the test ends, on every exchange no request used.
type Fake struct {
	t         testing.TB
	mu        sync.Mutex
	exchanges []Exchange
	used      []bool
	requests  []Request

	// failure, when set, answers every request in place of the exchanges.
	failure *failure
}

type failure struct {
	status  int
	message string
}

// New returns a Fake serving exchanges, checked against t. It fails t at once
// on an exchange that lacks a method or a path, on one that answers without a
// valid status, and on one that sets Stall or Err beside a response or both.
func New(t testing.TB, exchanges []Exchange) *Fake {
	t.Helper()

	for i, ex := range exchanges {
		response := ex.Status != 0 || ex.Header != nil || ex.Body != nil
		switch {
		case ex.Method == "" || ex.Path == "":
			t.Fatalf("faketracker: exchange %d needs a method and a path: %+v", i, ex)
		case ex.answers() && http.StatusText(ex.Status) == "":
			t.Fatalf("faketracker: exchange %d needs a known status: %+v", i, ex)
		case !ex.answers() && (response || ex.Stall && ex.Err != nil):
			t.Fatalf("faketracker: exchange %d sets Stall or Err beside a response or both: %+v", i, ex)
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

// Failing returns a Fake that answers every request with status and
// Tracker's error body carrying message, and records it. Unlike New's, its
// requests fail no test: whether one was sent is for the caller to judge.
func Failing(t testing.TB, status int, message string) *Fake {
	t.Helper()

	if http.StatusText(status) == "" {
		t.Fatalf("faketracker: Failing needs a known status, not %d", status)
	}

	return &Fake{t: t, failure: &failure{status: status, message: message}}
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
		Header: req.Header.Clone(),
		Body:   string(body),
	}

	rec := httptest.NewRecorder()
	switch ex := f.serve(rec, got); {
	case ex.Stall:
		<-req.Context().Done()
		return nil, req.Context().Err()
	case ex.Err != nil:
		return nil, ex.Err
	}

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

// serve answers got into w, unless the exchange it matches stands for no
// response: it returns that exchange for RoundTrip to play out, and the zero
// Exchange otherwise.
func (f *Fake) serve(w http.ResponseWriter, got Request) Exchange {
	f.mu.Lock()
	f.requests = append(f.requests, got)
	ex, ok := f.take(got)
	f.mu.Unlock()

	if f.failure != nil {
		writeError(w, f.failure.status, f.failure.message)
		return Exchange{}
	}

	if !ok {
		unmatched := "no exchange matches " + describe(got.Method, got.Path, got.Query)
		f.t.Errorf("faketracker: %s", unmatched)
		// Tracker's error shape makes the command fail the way it would on a
		// real error instead of on a body it cannot decode.
		writeError(w, http.StatusNotImplemented, "faketracker: "+unmatched)
		return Exchange{}
	}

	if !ex.answers() {
		return ex
	}

	for name, values := range ex.Header {
		for _, value := range values {
			w.Header().Add(name, value)
		}
	}
	w.WriteHeader(ex.Status)
	_, _ = w.Write(ex.Body)

	return Exchange{}
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

func writeError(w http.ResponseWriter, status int, message string) {
	body, _ := json.Marshal(map[string]any{
		"errorMessages": []string{message},
		"errors":        map[string]string{},
		"statusCode":    status,
	})

	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	_, _ = w.Write(body)
}
