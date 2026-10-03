package faketracker

import (
	"bytes"
	"encoding/json"
	"errors"
	"io"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

type roundTripFunc func(*http.Request) (*http.Response, error)

func (fn roundTripFunc) RoundTrip(req *http.Request) (*http.Response, error) {
	return fn(req)
}

func TestRecorderRefusesAWriteWithoutSendingIt(t *testing.T) {
	sent := 0
	rec := NewRecorder(roundTripFunc(func(*http.Request) (*http.Response, error) {
		sent++
		return nil, errors.New("sent")
	}))

	req, err := http.NewRequestWithContext(t.Context(), http.MethodPost,
		"https://api.tracker.yandex.net/v3/issues/", strings.NewReader(`{"summary":"x"}`))
	if err != nil {
		t.Fatalf("NewRequest: %v", err)
	}

	if _, err := rec.RoundTrip(req); err == nil || !strings.Contains(err.Error(), "POST /v3/issues/") {
		t.Errorf("RoundTrip error = %v, want a refusal naming POST /v3/issues/", err)
	}
	if sent != 0 {
		t.Errorf("base transport got %d requests, want 0", sent)
	}

	path := filepath.Join(t.TempDir(), "fixture.json")
	if err := rec.Save(path); err == nil {
		t.Error("Save after a refused write returned nil, want an error")
	}
	if _, err := os.Stat(path); !errors.Is(err, os.ErrNotExist) {
		t.Errorf("Stat(%s) = %v, want the fixture not written", path, err)
	}
}

func TestRecorderKeepsTheExchangeAndHandsTheBodyOn(t *testing.T) {
	rec := NewRecorder(roundTripFunc(func(req *http.Request) (*http.Response, error) {
		if req.Header.Get("Authorization") == "" {
			t.Error("request reached the base without its Authorization header")
		}
		return &http.Response{
			StatusCode: http.StatusOK,
			Header: http.Header{
				"Content-Type":  {"application/json;charset=UTF-8"},
				"X-Total-Count": {"2"},
				"Set-Cookie":    {"session=abc"},
				"X-Request-Id":  {"req-1"},
			},
			Body:    io.NopCloser(strings.NewReader(`[{"id":1}]`)),
			Request: req,
		}, nil
	}))

	req, err := http.NewRequestWithContext(t.Context(), http.MethodGet,
		"https://api.tracker.yandex.net/v3/priorities?localized=false", nil)
	if err != nil {
		t.Fatalf("NewRequest: %v", err)
	}
	req.Header.Set("Authorization", "OAuth secret")
	req.Header.Set("X-Org-Id", "42")

	resp, err := rec.RoundTrip(req)
	if err != nil {
		t.Fatalf("RoundTrip: %v", err)
	}
	body, _ := io.ReadAll(resp.Body)
	resp.Body.Close()
	if string(body) != `[{"id":1}]` {
		t.Errorf("client got body %q, want the response body unchanged", body)
	}

	path := filepath.Join(t.TempDir(), "fixtures", "priority-list.json")
	if saveErr := rec.Save(path); saveErr != nil {
		t.Fatalf("Save: %v", saveErr)
	}

	got := Load(t, path)
	if len(got) != 1 {
		t.Fatalf("fixture holds %d exchanges, want 1", len(got))
	}
	ex := got[0]
	if ex.Method != http.MethodGet || ex.Path != "/v3/priorities" || ex.Query.Get("localized") != "false" ||
		ex.Status != http.StatusOK {
		t.Errorf("exchange = %s %s %v %d, want GET /v3/priorities localized=false 200",
			ex.Method, ex.Path, ex.Query, ex.Status)
	}
	var compact bytes.Buffer
	if compactErr := json.Compact(&compact, ex.Body); compactErr != nil || compact.String() != `[{"id":1}]` {
		t.Errorf("body = %s, want [{\"id\":1}]", ex.Body)
	}
	if len(ex.Header) != 2 || ex.Header.Get("Content-Type") == "" || ex.Header.Get("X-Total-Count") != "2" {
		t.Errorf("header = %v, want only Content-Type and X-Total-Count", ex.Header)
	}

	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("ReadFile: %v", err)
	}
	for _, leaked := range []string{"secret", "Authorization", "X-Org-Id", "session", "req-1"} {
		if strings.Contains(string(data), leaked) {
			t.Errorf("fixture contains %q:\n%s", leaked, data)
		}
	}
}
