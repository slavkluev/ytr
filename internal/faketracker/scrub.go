package faketracker

import (
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"maps"
	"regexp"
	"slices"
	"strconv"
	"strings"
)

// fakeNumberBase keeps fake user IDs clear of the small numbers a response
// also carries, such as orders, versions and reference IDs, so a fake never
// reads as one of those.
const fakeNumberBase = 1_000_000_000

const usersPathSegment = "/users/"

// identityKeys mark an object as a user. Any object carrying one is scrubbed,
// so a key Tracker also puts on something else costs an over-scrubbed value,
// never a leak.
var identityKeys = []string{"login", "email", "uid", "passportUid", "cloudUid", "trackerUid"}

// personalKeys are replaced on a user object. On anything else, display names
// a priority, a status or a queue and stays.
var personalKeys = map[string]bool{
	"login": true, "email": true, "uid": true, "passportUid": true, "cloudUid": true, "trackerUid": true,
	"id": true, "self": true, "display": true, "firstName": true, "lastName": true,
}

type scrubber struct {
	index    map[string]int
	seen     map[Secret]bool
	replaced []Secret
}

func scrub(exchanges []Exchange) ([]Exchange, []Secret, error) {
	s := &scrubber{index: map[string]int{}, seen: map[Secret]bool{}}
	out := make([]Exchange, len(exchanges))

	for i, ex := range exchanges {
		out[i] = ex
		if len(ex.Body) == 0 {
			continue
		}

		body, err := s.scrubBody(ex.Body)
		if err != nil {
			return nil, nil, fmt.Errorf("faketracker: body of %s %s: %w", ex.Method, ex.Path, err)
		}
		out[i].Body = body
	}

	return out, s.replaced, nil
}

func (s *scrubber) scrubBody(body json.RawMessage) (json.RawMessage, error) {
	dec := json.NewDecoder(bytes.NewReader(body))
	dec.UseNumber()

	var doc any
	if err := dec.Decode(&doc); err != nil {
		return nil, err
	}
	if _, err := dec.Token(); !errors.Is(err, io.EOF) {
		return nil, errors.New("more than one JSON value")
	}

	var buf bytes.Buffer
	enc := json.NewEncoder(&buf)
	enc.SetEscapeHTML(false)
	if err := enc.Encode(s.walk(doc)); err != nil {
		return nil, err
	}

	return bytes.TrimSuffix(buf.Bytes(), []byte("\n")), nil
}

func (s *scrubber) walk(v any) any {
	switch val := v.(type) {
	case map[string]any:
		user := isUser(val)
		// Sorted, so the same response gets the same fakes on every recording.
		for _, key := range slices.Sorted(maps.Keys(val)) {
			if user && personalKeys[key] {
				val[key] = s.fake(key, val[key])
			} else {
				val[key] = s.walk(val[key])
			}
		}
		return val
	case []any:
		for i := range val {
			val[i] = s.walk(val[i])
		}
		return val
	default:
		return v
	}
}

func isUser(obj map[string]any) bool {
	if self, ok := obj["self"].(string); ok && strings.Contains(self, usersPathSegment) {
		return true
	}

	for _, key := range identityKeys {
		if _, ok := obj[key]; ok {
			return true
		}
	}

	return false
}

func (s *scrubber) fake(key string, v any) any {
	switch val := v.(type) {
	case string:
		if val == "" {
			return val
		}
		s.remember(key, val)
		return s.fakeString(key, val)
	case json.Number:
		s.remember(key, val.String())
		return json.Number(s.fakeDigits(val.String()))
	default:
		return s.walk(v)
	}
}

// fakeString keeps a numeric ID numeric and a user's self link pointing at
// the fake ID, so the same user reads the same through every key that names
// it: an ID string, a numeric uid and a self link all map through one index.
func (s *scrubber) fakeString(key, original string) string {
	if isDigits(original) {
		return s.fakeDigits(original)
	}

	if key == "self" {
		if prefix, tail, ok := strings.Cut(original, usersPathSegment); ok && tail != "" {
			s.remember(key, tail)
			return prefix + usersPathSegment + s.fakeString("id", tail)
		}
	}

	n := s.number(original)
	switch key {
	case "email":
		return fmt.Sprintf("user%d@example.com", n)
	case "login":
		return fmt.Sprintf("user%d", n)
	case "display":
		return fmt.Sprintf("User %d", n)
	case "firstName":
		return fmt.Sprintf("First%d", n)
	case "lastName":
		return fmt.Sprintf("Last%d", n)
	default:
		return fmt.Sprintf("%s%d", key, n)
	}
}

func (s *scrubber) remember(key, original string) {
	secret := Secret{Name: key, Value: original}
	if !s.seen[secret] {
		s.seen[secret] = true
		s.replaced = append(s.replaced, secret)
	}
}

func (s *scrubber) fakeDigits(original string) string {
	return strconv.Itoa(fakeNumberBase + s.number(original))
}

func (s *scrubber) number(original string) int {
	n, ok := s.index[original]
	if !ok {
		n = len(s.index) + 1
		s.index[original] = n
	}

	return n
}

func isDigits(v string) bool {
	if v == "" {
		return false
	}

	for _, r := range v {
		if r < '0' || r > '9' {
			return false
		}
	}

	return true
}

// checkLeaks finds each secret in data as a whole word, ignoring case, in its
// raw and its JSON-escaped form, and names the ones it finds. Matching whole
// words keeps a short surname from being found inside an ordinary word.
func checkLeaks(data []byte, secrets []Secret) error {
	leaked := map[string]bool{}

	for _, secret := range secrets {
		if secret.Value == "" || leaked[secret.Name] {
			continue
		}

		for _, form := range secretForms(secret.Value) {
			if wordPattern(form).Match(data) {
				leaked[secret.Name] = true
				break
			}
		}
	}

	if len(leaked) == 0 {
		return nil
	}

	return fmt.Errorf("still present after scrubbing: %s", strings.Join(slices.Sorted(maps.Keys(leaked)), ", "))
}

func secretForms(value string) []string {
	forms := []string{value}

	var buf bytes.Buffer
	enc := json.NewEncoder(&buf)
	enc.SetEscapeHTML(false)
	if err := enc.Encode(value); err == nil {
		escaped := strings.TrimSuffix(strings.TrimSpace(buf.String()), `"`)
		escaped = strings.TrimPrefix(escaped, `"`)
		if escaped != value {
			forms = append(forms, escaped)
		}
	}

	return forms
}

func wordPattern(value string) *regexp.Regexp {
	return regexp.MustCompile(`(?i)(?:^|[^\pL\pN_])` + regexp.QuoteMeta(value) + `(?:$|[^\pL\pN_])`)
}
