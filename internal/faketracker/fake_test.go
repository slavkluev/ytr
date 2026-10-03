package faketracker

import (
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"slices"
	"strings"
	"testing"

	"github.com/slavkluev/go-yandex-tracker/tracker"
)

// recordingTB stands in for the test a Fake reports to, so a test can see the
// failures Fake raises instead of failing itself.
type recordingTB struct {
	testing.TB

	failures []string
	cleanups []func()
}

func (r *recordingTB) Helper() {}

func (r *recordingTB) Errorf(format string, args ...any) {
	r.failures = append(r.failures, fmt.Sprintf(format, args...))
}

func (r *recordingTB) Fatalf(format string, args ...any) {
	r.failures = append(r.failures, fmt.Sprintf(format, args...))
}

func (r *recordingTB) Cleanup(f func()) {
	r.cleanups = append(r.cleanups, f)
}

func (r *recordingTB) runCleanups() {
	for _, f := range slices.Backward(r.cleanups) {
		f()
	}
}

func get(t *testing.T, rt http.RoundTripper, rawURL string) (int, string) {
	t.Helper()

	req, err := http.NewRequestWithContext(t.Context(), http.MethodGet, rawURL, nil)
	if err != nil {
		t.Fatalf("NewRequest: %v", err)
	}

	resp, err := (&http.Client{Transport: rt}).Do(req)
	if err != nil {
		t.Fatalf("GET %s: %v", rawURL, err)
	}
	defer resp.Body.Close()

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		t.Fatalf("reading body: %v", err)
	}

	return resp.StatusCode, string(body)
}

func exchange(path string, query url.Values, body string) Exchange {
	return Exchange{
		Method: http.MethodGet,
		Path:   path,
		Query:  query,
		Status: http.StatusOK,
		Header: http.Header{"Content-Type": {"application/json"}},
		Body:   []byte(body),
	}
}

func TestFakeServesTheExchangeWhoseQueryMatches(t *testing.T) {
	fake := New(t, []Exchange{
		exchange("/v3/issues", url.Values{"page": {"1"}}, `"A"`),
		exchange("/v3/issues", url.Values{"page": {"2"}}, `"B"`),
	})

	if _, body := get(t, fake, "https://api.tracker.yandex.net/v3/issues?page=2"); body != `"B"` {
		t.Errorf("page 2 body = %s, want \"B\"", body)
	}
	if _, body := get(t, fake, "https://api.tracker.yandex.net/v3/issues?page=1"); body != `"A"` {
		t.Errorf("page 1 body = %s, want \"A\"", body)
	}
}

func TestFakeServesARepeatFromTheNextUnusedMatch(t *testing.T) {
	fake := New(t, []Exchange{
		exchange("/v3/statuses", nil, `"first"`),
		exchange("/v3/statuses", nil, `"second"`),
	})

	for _, want := range []string{`"first"`, `"second"`} {
		if _, body := get(t, fake, "https://api.tracker.yandex.net/v3/statuses"); body != want {
			t.Errorf("body = %s, want %s", body, want)
		}
	}
}

func TestFakeServesStatusHeadersAndBody(t *testing.T) {
	ex := exchange("/v3/issues/_search", nil, `[]`)
	ex.Header.Set("X-Total-Count", "7")
	fake := New(t, []Exchange{ex})

	req, err := http.NewRequestWithContext(t.Context(), http.MethodGet,
		"https://api.tracker.yandex.net/v3/issues/_search", nil)
	if err != nil {
		t.Fatalf("NewRequest: %v", err)
	}

	resp, err := fake.RoundTrip(req)
	if err != nil {
		t.Fatalf("RoundTrip: %v", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		t.Errorf("status = %d, want 200", resp.StatusCode)
	}
	if got := resp.Header.Get("X-Total-Count"); got != "7" {
		t.Errorf("X-Total-Count = %q, want 7", got)
	}
	if resp.Request != req {
		t.Error("response does not carry the request it answers")
	}
}

func TestFakeCanonicalizesFixtureHeaderNames(t *testing.T) {
	ex := exchange("/v3/statuses", nil, `[]`)
	ex.Header = http.Header{"link": {`<https://api.tracker.yandex.net/v3/statuses?page=2>; rel="next"`}}
	fake := New(t, []Exchange{ex})

	req, err := http.NewRequestWithContext(t.Context(), http.MethodGet,
		"https://api.tracker.yandex.net/v3/statuses", nil)
	if err != nil {
		t.Fatalf("NewRequest: %v", err)
	}

	resp, err := fake.RoundTrip(req)
	if err != nil {
		t.Fatalf("RoundTrip: %v", err)
	}
	defer resp.Body.Close()

	if resp.Header.Get("Link") == "" {
		t.Errorf("Link header missing from %v", resp.Header)
	}
}

func TestFakeRecordsEveryRequest(t *testing.T) {
	fake := New(t, []Exchange{
		exchange("/v3/statuses", nil, `[]`),
		{
			Method: http.MethodPost,
			Path:   "/v3/issues/PROJ-1/comments",
			Query:  url.Values{"isAddToFollowers": {"false"}},
			Status: http.StatusCreated,
		},
	})

	get(t, fake, "https://api.tracker.yandex.net/v3/statuses")

	req, err := http.NewRequestWithContext(t.Context(), http.MethodPost,
		"https://api.tracker.yandex.net/v3/issues/PROJ-1/comments?isAddToFollowers=false",
		strings.NewReader(`{"text":"hi"}`))
	if err != nil {
		t.Fatalf("NewRequest: %v", err)
	}
	resp, err := (&http.Client{Transport: fake}).Do(req)
	if err != nil {
		t.Fatalf("POST: %v", err)
	}
	resp.Body.Close()

	got := fake.Requests()
	if len(got) != 2 {
		t.Fatalf("requests = %+v, want 2", got)
	}
	if got[0].Method != http.MethodGet || got[0].Path != "/v3/statuses" || len(got[0].Query) != 0 || got[0].Body != "" {
		t.Errorf("first request = %+v, want a bare GET /v3/statuses", got[0])
	}
	want := Request{
		Method: http.MethodPost,
		Path:   "/v3/issues/PROJ-1/comments",
		Query:  url.Values{"isAddToFollowers": {"false"}},
		Body:   `{"text":"hi"}`,
	}
	if got[1].Method != want.Method || got[1].Path != want.Path ||
		!sameQuery(got[1].Query, want.Query) || got[1].Body != want.Body {
		t.Errorf("second request = %+v, want %+v", got[1], want)
	}
}

func TestFakeAnswersAnUnmatchedRequestWith501AndFailsTheTest(t *testing.T) {
	tb := &recordingTB{}
	fake := New(tb, nil)

	client := tracker.NewClient(tracker.WithHTTPClient(&http.Client{Transport: fake}))
	_, _, err := client.Statuses.List(t.Context(), nil)

	var errResp *tracker.ErrorResponse
	if !errors.As(err, &errResp) {
		t.Fatalf("error = %v, want a *tracker.ErrorResponse", err)
	}
	if errResp.Response.StatusCode != http.StatusNotImplemented {
		t.Errorf("status = %d, want 501", errResp.Response.StatusCode)
	}
	if len(errResp.ErrorMessages) != 1 || !strings.Contains(errResp.ErrorMessages[0], "GET /v3/statuses") {
		t.Errorf("errorMessages = %q, want one naming GET /v3/statuses", errResp.ErrorMessages)
	}

	if len(tb.failures) != 1 {
		t.Fatalf("failures = %q, want exactly one", tb.failures)
	}
	if !strings.Contains(tb.failures[0], `GET /v3/statuses query ""`) {
		t.Errorf("failure = %q, want it to name method, path and query", tb.failures[0])
	}
}

func TestFakeNamesTheQueryOfAnUnmatchedRequest(t *testing.T) {
	tb := &recordingTB{}
	fake := New(tb, []Exchange{exchange("/v3/priorities", url.Values{"localized": {"true"}}, `[]`)})

	code, _ := get(t, fake, "https://api.tracker.yandex.net/v3/priorities?localized=false")
	if code != http.StatusNotImplemented {
		t.Errorf("status = %d, want 501", code)
	}

	if len(tb.failures) != 1 || !strings.Contains(tb.failures[0], `GET /v3/priorities query "localized=false"`) {
		t.Errorf("failures = %q, want one naming GET /v3/priorities query localized=false", tb.failures)
	}
}

func TestFakeFailsAtCleanupOnAnUnusedExchange(t *testing.T) {
	tb := &recordingTB{}
	fake := New(tb, []Exchange{
		exchange("/v3/statuses", nil, `[]`),
		exchange("/v3/resolutions", url.Values{"perPage": {"50"}}, `[]`),
	})

	get(t, fake, "https://api.tracker.yandex.net/v3/statuses")
	if len(tb.failures) != 0 {
		t.Fatalf("failures before cleanup = %q, want none", tb.failures)
	}

	tb.runCleanups()

	if len(tb.failures) != 1 {
		t.Fatalf("failures = %q, want exactly one", tb.failures)
	}
	if !strings.Contains(tb.failures[0], `exchange 1, GET /v3/resolutions query "perPage=50", was never requested`) {
		t.Errorf("failure = %q, want it to name the unused exchange", tb.failures[0])
	}
}

func TestFakeRejectsAnExchangeWithoutStatus(t *testing.T) {
	tb := &recordingTB{}
	New(tb, []Exchange{{Method: http.MethodGet, Path: "/v3/statuses"}})

	if len(tb.failures) != 1 || !strings.Contains(tb.failures[0], "exchange 0") {
		t.Errorf("failures = %q, want one naming exchange 0", tb.failures)
	}
}
