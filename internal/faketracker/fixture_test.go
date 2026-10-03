package faketracker

import (
	"encoding/json"
	"errors"
	"net/http"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"testing"
)

func saveAndDecode(t *testing.T, body string) any {
	t.Helper()

	path := filepath.Join(t.TempDir(), "fixture.json")
	if err := Save(path, []Exchange{exchange("/v3/x", nil, body)}); err != nil {
		t.Fatalf("Save: %v", err)
	}

	got := Load(t, path)
	var doc any
	if err := json.Unmarshal(got[0].Body, &doc); err != nil {
		t.Fatalf("scrubbed body is not JSON: %v", err)
	}

	return doc
}

func TestSaveScrubsAUserFoundBySelf(t *testing.T) {
	doc := saveAndDecode(t, `{
		"self": "https://api.tracker.yandex.net/v3/users/9000000000000012",
		"id": "9000000000000012",
		"display": "Ivan Petrov"
	}`)

	want := map[string]any{
		"self":    "https://api.tracker.yandex.net/v3/users/1000000002",
		"id":      "1000000002",
		"display": "User 1",
	}
	assertJSONEqual(t, doc, want)
}

func TestSaveScrubsAUserFoundByIdentityKeys(t *testing.T) {
	doc := saveAndDecode(t, `[
		{"uid": 9000000000000012, "login": "ipetrov", "email": "ipetrov@example.ru",
		 "firstName": "Ivan", "lastName": "Petrov", "display": "Ivan Petrov", "dismissed": false},
		{"passportUid": 9000000000000013, "cloudUid": "ajeabcdef", "trackerUid": 7, "display": "Anna"}
	]`)

	want := []any{
		map[string]any{
			"uid": 1000000006.0, "login": "user5", "email": "user2@example.com",
			"firstName": "First3", "lastName": "Last4", "display": "User 1", "dismissed": false,
		},
		map[string]any{
			"passportUid": 1000000009.0, "cloudUid": "cloudUid7", "trackerUid": 1000000010.0, "display": "User 8",
		},
	}
	assertJSONEqual(t, doc, want)
}

func TestSaveGivesTheSameOriginalTheSameFake(t *testing.T) {
	doc := saveAndDecode(t, `{
		"createdBy": {"self": "https://api.tracker.yandex.net/v3/users/42", "id": "42", "display": "Ivan Petrov"},
		"assignee": {"self": "https://api.tracker.yandex.net/v3/users/42", "id": "42", "display": "Ivan Petrov"},
		"followers": [{"uid": 42, "display": "Ivan Petrov"}]
	}`)

	root, _ := doc.(map[string]any)
	created, _ := root["createdBy"].(map[string]any)
	assignee, _ := root["assignee"].(map[string]any)
	followers, _ := root["followers"].([]any)
	follower, _ := followers[0].(map[string]any)

	assertJSONEqual(t, assignee, created)
	if follower["display"] != created["display"] {
		t.Errorf("follower display = %v, want %v", follower["display"], created["display"])
	}
	uid, isNumber := follower["uid"].(float64)
	if !isNumber {
		t.Fatalf("follower uid = %#v, want a number", follower["uid"])
	}
	if strconv.FormatFloat(uid, 'f', -1, 64) != created["id"] {
		t.Errorf("follower uid %v and creator id %v, want the same fake for the same 42", uid, created["id"])
	}
}

func TestSaveKeepsDisplayOutsideUsers(t *testing.T) {
	body := `{"priority":{"self":"https://api.tracker.yandex.net/v3/priorities/2","id":"2","key":"normal","display":"Normal"},` +
		`"status":{"self":"https://api.tracker.yandex.net/v3/statuses/1","id":"1","key":"open","display":"Open"}}`

	doc := saveAndDecode(t, body)

	var want any
	if err := json.Unmarshal([]byte(body), &want); err != nil {
		t.Fatal(err)
	}
	assertJSONEqual(t, doc, want)
}

func TestSaveRefusesALeakedUserValue(t *testing.T) {
	path := filepath.Join(t.TempDir(), "fixture.json")
	body := `[{"self":"https://api.tracker.yandex.net/v3/users/42","display":"Ivan Petrov"},{"summary":"Ask Ivan Petrov"}]`

	err := Save(path, []Exchange{exchange("/v3/x", nil, body)})

	assertRefused(t, err, path, "display")
	if strings.Contains(err.Error(), "Ivan") {
		t.Errorf("error %q repeats the leaked value", err)
	}
}

func TestSaveRefusesALeakedToken(t *testing.T) {
	path := filepath.Join(t.TempDir(), "fixture.json")
	ex := exchange("/v3/x", nil, `{"note":"y0_secret-token"}`)

	err := Save(path, []Exchange{ex}, Secret{Name: "token", Value: "y0_secret-token"})

	assertRefused(t, err, path, "token")
	if strings.Contains(err.Error(), "y0_secret-token") {
		t.Errorf("error %q repeats the token", err)
	}
}

func TestSaveRefusesALeakedOrgIDAnywhereInTheFixture(t *testing.T) {
	path := filepath.Join(t.TempDir(), "fixture.json")
	ex := exchange("/v3/x", nil, `[]`)
	ex.Header.Set("Link", `<https://api.tracker.yandex.net/v3/x?org=bpf123>; rel="next"`)

	err := Save(path, []Exchange{ex}, Secret{Name: "org ID", Value: "bpf123"})

	assertRefused(t, err, path, "org ID")
}

func TestSaveRefusesAUserValueAfterAJSONEscape(t *testing.T) {
	for _, escape := range []string{`\n`, `\t`, `\u2028`} {
		path := filepath.Join(t.TempDir(), "fixture.json")
		body := `[{"self":"https://api.tracker.yandex.net/v3/users/42","display":"Ivan Petrov"},` +
			`{"description":"Hello` + escape + `Ivan Petrov"}]`

		err := Save(path, []Exchange{exchange("/v3/x", nil, body)})

		assertRefused(t, err, path, "display")
	}
}

func TestSaveIgnoresASecretInsideALongerWord(t *testing.T) {
	path := filepath.Join(t.TempDir(), "fixture.json")
	body := `[{"self":"https://api.tracker.yandex.net/v3/users/42","lastName":"Li"},{"key":"Linked","name":"Lists"}]`

	if err := Save(path, []Exchange{exchange("/v3/x", nil, body)}); err != nil {
		t.Errorf("Save: %v, want the surname Li not found inside Linked or Lists", err)
	}
}

func TestSaveRefusesABodyThatIsNotJSON(t *testing.T) {
	path := filepath.Join(t.TempDir(), "fixture.json")
	ex := exchange("/v3/x", nil, `<html>Bad gateway</html>`)
	ex.Status = http.StatusBadGateway

	if err := Save(path, []Exchange{ex}); err == nil {
		t.Error("Save of an HTML body returned nil, want an error")
	}
	if _, err := os.Stat(path); !errors.Is(err, os.ErrNotExist) {
		t.Errorf("Stat(%s) = %v, want the fixture not written", path, err)
	}
}

func TestLoadRejectsUnknownKeys(t *testing.T) {
	path := filepath.Join(t.TempDir(), "fixture.json")
	fixture := `[{"method":"GET","path":"/v3/x","status":200,"headers":{}}]`
	if err := os.WriteFile(path, []byte(fixture), 0o600); err != nil {
		t.Fatal(err)
	}

	tb := &recordingTB{}
	Load(tb, path)

	if len(tb.failures) != 1 || !strings.Contains(tb.failures[0], "headers") {
		t.Errorf("failures = %q, want one naming the unknown key headers", tb.failures)
	}
}

func assertRefused(t *testing.T, err error, path, name string) {
	t.Helper()

	if err == nil || !strings.Contains(err.Error(), name) {
		t.Errorf("Save error = %v, want a refusal naming %s", err, name)
	}
	if _, statErr := os.Stat(path); !errors.Is(statErr, os.ErrNotExist) {
		t.Errorf("Stat(%s) = %v, want the fixture not written", path, statErr)
	}
}

func assertJSONEqual(t *testing.T, got, want any) {
	t.Helper()

	gotJSON, _ := json.Marshal(got)
	wantJSON, _ := json.Marshal(want)
	if string(gotJSON) != string(wantJSON) {
		t.Errorf("got  %s\nwant %s", gotJSON, wantJSON)
	}
}
