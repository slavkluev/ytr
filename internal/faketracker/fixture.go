// Package faketracker stands in for Yandex Tracker below the real
// go-yandex-tracker client: Fake replays recorded exchanges and records the
// requests it gets, and Recorder captures exchanges from the real Tracker for
// Save to scrub and write as fixtures.
package faketracker

import (
	"bytes"
	"encoding/json"
	"fmt"
	"net/http"
	"net/url"
	"os"
	"path/filepath"
	"testing"
)

// Git stores a file's mode only as executable or not, so a committed fixture
// is readable to every checkout whatever these say.
const (
	fixtureDirMode  = 0o750
	fixtureFileMode = 0o600
)

// Exchange is one request and the response Tracker gave to it. A fixture file
// holds a JSON array of them and never a request header, so no credential can
// reach one.
type Exchange struct {
	Method string          `json:"method"`
	Path   string          `json:"path"`
	Query  url.Values      `json:"query,omitempty"`
	Status int             `json:"status"`
	Header http.Header     `json:"header,omitempty"`
	Body   json.RawMessage `json:"body,omitempty"`
}

// Secret is a value no fixture may contain, with the name a failed leak check
// reports in its place.
type Secret struct {
	Name  string
	Value string
}

// Load reads the exchanges of a fixture file and fails t when the file is
// missing or holds anything but exchanges.
func Load(t testing.TB, path string) []Exchange {
	t.Helper()

	data, err := os.ReadFile(filepath.Clean(path))
	if err != nil {
		t.Fatalf("faketracker: %v", err)
	}

	dec := json.NewDecoder(bytes.NewReader(data))
	dec.DisallowUnknownFields()

	var exchanges []Exchange
	if err := dec.Decode(&exchanges); err != nil {
		t.Fatalf("faketracker: decoding %s: %v", path, err)
	}

	return exchanges
}

// Save replaces the identity keys, id, self, display and names of every user
// object in the exchanges' bodies with fakes, and writes the exchanges to path
// as a fixture. Nothing else is scrubbed: it writes nothing when a replaced
// value or one of the secrets still appears anywhere in the fixture, and its
// error names each of those by key, never by value.
func Save(path string, exchanges []Exchange, secrets ...Secret) error {
	scrubbed, replaced, err := scrub(exchanges)
	if err != nil {
		return err
	}

	data, err := encodeFixture(scrubbed)
	if err != nil {
		return err
	}

	if err := checkLeaks(data, append(replaced, secrets...)); err != nil {
		return fmt.Errorf("faketracker: not writing %s: %w", path, err)
	}

	if err := os.MkdirAll(filepath.Dir(path), fixtureDirMode); err != nil {
		return err
	}

	return os.WriteFile(path, data, fixtureFileMode)
}

func encodeFixture(exchanges []Exchange) ([]byte, error) {
	var buf bytes.Buffer
	enc := json.NewEncoder(&buf)
	enc.SetEscapeHTML(false)
	enc.SetIndent("", "  ")
	if err := enc.Encode(exchanges); err != nil {
		return nil, err
	}

	return buf.Bytes(), nil
}
