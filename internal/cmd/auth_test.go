package cmd

import (
	"encoding/json"
	"errors"
	"net/http"
	"os"
	"path/filepath"
	"slices"
	"testing"

	ytrerrors "github.com/slavkluev/ytr/internal/errors"
	"github.com/slavkluev/ytr/internal/faketracker"
)

func myselfAnswer(display string) faketracker.Exchange {
	return trackerGET("/v3/myself", `{"uid": 12345, "display": "`+display+`"}`)
}

func myselfError(status int, message string) faketracker.Exchange {
	return trackerError(http.MethodGet, "/v3/myself", status, message)
}

// assertSignIns wants the run's requests to sign in with token as orgID, the
// first through the 360 header and the second, when there is one, through the
// cloud header: the order org-type detection tries them in.
func assertSignIns(token, orgID string) func(*testing.T, cliResult) {
	return func(t *testing.T, res cliResult) {
		t.Helper()

		headers := []struct{ set, unset string }{{"X-Org-Id", "X-Cloud-Org-Id"}, {"X-Cloud-Org-Id", "X-Org-Id"}}
		for i, req := range res.Requests {
			if i >= len(headers) {
				t.Fatalf("requests = %+v, want at most %d sign-ins", res.Requests, len(headers))
			}
			if got := req.Header.Get(headers[i].set); got != orgID || req.Header.Get(headers[i].unset) != "" {
				t.Errorf("request %d carries %s %q and %s %q, want only %s %q", i+1, headers[i].set, got,
					headers[i].unset, req.Header.Get(headers[i].unset), headers[i].set, orgID)
			}
			if got := req.Header.Get("Authorization"); got != "OAuth "+token {
				t.Errorf("request %d carries Authorization %q, want the token %q", i+1, got, token)
			}
		}
	}
}

// assertConfigFile wants the run's config.yaml to hold exactly want.
func assertConfigFile(want string) func(*testing.T, cliResult) {
	return func(t *testing.T, res cliResult) {
		t.Helper()

		data, err := os.ReadFile(filepath.Join(res.ConfigDir, "config.yaml"))
		if err != nil {
			t.Fatalf("reading the config: %v", err)
		}
		if string(data) != want {
			t.Errorf("config.yaml = %q, want %q", data, want)
		}
	}
}

func assertNoConfigFile(t *testing.T, res cliResult) {
	t.Helper()

	if _, err := os.Stat(filepath.Join(res.ConfigDir, "config.yaml")); !errors.Is(err, os.ErrNotExist) {
		t.Errorf("config.yaml: %v, want it never written", err)
	}
}

// assertStderrNamesTheConfig wants stderr to be want followed by the path of
// the run's config.yaml and a newline.
func assertStderrNamesTheConfig(want string) func(*testing.T, cliResult) {
	return func(t *testing.T, res cliResult) {
		t.Helper()

		if full := want + filepath.Join(res.ConfigDir, "config.yaml") + "\n"; res.Stderr != full {
			t.Errorf("stderr = %q, want %q", res.Stderr, full)
		}
	}
}

// assertJSONNamesTheConfig wants stdout to be want with the path of the run's
// config.yaml as its config_path.
func assertJSONNamesTheConfig(want string) func(*testing.T, cliResult) {
	return func(t *testing.T, res cliResult) {
		t.Helper()

		path, err := json.Marshal(filepath.Join(res.ConfigDir, "config.yaml"))
		if err != nil {
			t.Fatalf("encoding the config path: %v", err)
		}
		assertSameJSONAs(t, "stdout", res.Stdout, want[:len(want)-1]+`, "config_path": `+string(path)+"}")
	}
}

func TestAuthLogin(t *testing.T) {
	t.Parallel()

	login := func(extra ...string) []string { return slices.Concat([]string{"auth", "login"}, extra) }
	flags := []string{"--token", "test-token", "--org-id", "test-org"}

	runLeafRows(t, []leafRow{
		{
			name: "Flags", args: login(slices.Concat(flags, []string{"--org-type", "360"})...), signedOut: true,
			exchanges: []faketracker.Exchange{myselfAnswer("Test User")},
			stderr:    []string{"Authenticated as Test User (org: test-org, type: 360)\nConfig saved to "},
			check: func(t *testing.T, res cliResult) {
				t.Helper()
				assertConfigFile("token: test-token\norg_id: test-org\norg_type: \"360\"\n")(t, res)
				assertStderrNamesTheConfig("Authenticated as Test User (org: test-org, type: 360)\nConfig saved to ")(
					t, res)
			},
		},
		{
			name: "JSON", args: login(slices.Concat(flags, []string{"--org-type", "cloud", "--json", "status"})...),
			signedOut: true, exchanges: []faketracker.Exchange{myselfAnswer("JSON User")},
			json:  `{"status": "authenticated"}`,
			check: assertConfigFile("token: test-token\norg_id: test-org\norg_type: cloud\n"),
		},
		{
			name:      "jq of the whole document",
			args:      login(slices.Concat(flags, []string{"--org-type", "cloud", "--jq", "."})...),
			signedOut: true,
			exchanges: []faketracker.Exchange{myselfAnswer("JSON User")},
			holds:     []string{`"user":"JSON User"`},
			check: assertJSONNamesTheConfig(
				`{"status": "authenticated", "user": "JSON User", "org_id": "test-org", "org_type": "cloud"}`),
		},
		{
			name: "Unknown field before any request", args: login("--token", "T", "--org-id", "O", "--json", "bogus"),
			signedOut: true, code: ytrerrors.ExitUserError,
			stderr: []string{`"code":"invalid_field"`, `"invalidField":"bogus"`},
			check:  assertNoConfigFile,
		},
		{
			name: "Field hint before the token is read", args: login("--org-id", "O", "--json="), signedOut: true,
			stdin: "tok\n", code: ytrerrors.ExitUserError,
			stderr: []string{fieldHint("auth login", []string{"status", "user", "org_id", "org_type", "config_path"})},
			check:  assertNoConfigFile,
		},
		{
			name: "Rejected token", args: login(slices.Concat(flags, []string{"--org-type", "360"})...),
			signedOut: true, exchanges: []faketracker.Exchange{myselfError(http.StatusForbidden, "Token expired")},
			code: ytrerrors.ExitAuthError, stderr: []string{"Error: Token expired\n"},
			check: assertNoConfigFile,
		},
		{
			name: "Piped token", args: login("--org-id", "O"), signedOut: true, stdin: "tok\n",
			exchanges: []faketracker.Exchange{
				myselfError(http.StatusForbidden, "No access to organization"), myselfAnswer("Piped User"),
			},
			stderr: []string{"Authenticated as Piped User (org: O, type: cloud)\n"},
			check: func(t *testing.T, res cliResult) {
				t.Helper()
				assertSignIns("tok", "O")(t, res)
				assertConfigFile("token: tok\norg_id: O\norg_type: cloud\n")(t, res)
			},
		},
		{
			name: "Detection tries 360 first", args: login(flags...), signedOut: true,
			exchanges: []faketracker.Exchange{myselfAnswer("360 User")},
			stderr:    []string{"Authenticated as 360 User (org: test-org, type: 360)\n"},
			check: func(t *testing.T, res cliResult) {
				t.Helper()
				assertSignIns("test-token", "test-org")(t, res)
				assertConfigFile("token: test-token\norg_id: test-org\norg_type: \"360\"\n")(t, res)
			},
		},
		{
			name: "Detection denied by both", args: login(flags...), signedOut: true,
			exchanges: []faketracker.Exchange{
				myselfError(http.StatusForbidden, "360 access denied"),
				myselfError(http.StatusForbidden, "cloud access denied"),
			},
			code: ytrerrors.ExitAuthError,
			stderr: []string{
				"Error: failed to detect organization type: " +
					"360: GET https://api.tracker.yandex.net/v3/myself: 403 [360 access denied] map[]; " +
					"cloud: GET https://api.tracker.yandex.net/v3/myself: 403 [cloud access denied] map[]\n" +
					"Check your token and access to the Tracker organization\n",
			},
			check: func(t *testing.T, res cliResult) {
				t.Helper()
				assertSignIns("test-token", "test-org")(t, res)
				assertNoConfigFile(t, res)
			},
		},
		{
			name: "Detection fails differently", args: login(flags...), signedOut: true,
			exchanges: []faketracker.Exchange{
				myselfError(http.StatusForbidden, "360 access denied"),
				myselfError(http.StatusNotFound, "cloud not found"),
			},
			code: ytrerrors.ExitUserError, stderr: []string{"\nRetry with --org-type 360 or --org-type cloud\n"},
			check: assertSignIns("test-token", "test-org"),
		},
		{
			name: "Detection fails on both servers", args: login(flags...), signedOut: true,
			exchanges: []faketracker.Exchange{
				myselfError(http.StatusInternalServerError, "360 unavailable"),
				myselfError(http.StatusBadGateway, "cloud unavailable"),
			},
			code: ytrerrors.ExitUserError, stderr: []string{"\nRetry later\n"},
			check: assertSignIns("test-token", "test-org"),
		},
		{
			name: "Detection refused as a bad request", args: login(flags...), signedOut: true,
			exchanges: []faketracker.Exchange{
				myselfError(http.StatusBadRequest, "360 invalid request"),
				myselfError(http.StatusBadRequest, "cloud invalid request"),
			},
			code: ytrerrors.ExitUserError, stderr: []string{"\nReview the reported Tracker error details and retry\n"},
			check: assertSignIns("test-token", "test-org"),
		},
		{
			name: "Empty piped token", args: login("--org-id", "O"), signedOut: true, stdin: "\n",
			code: ytrerrors.ExitUserError,
			stderr: []string{
				"Error: empty token from stdin\nPipe a non-empty token: echo TOKEN | ytr auth login --org-id ORG\n",
			},
		},
		{
			name: "No token", args: login("--org-id", "O"), signedOut: true, code: ytrerrors.ExitUserError,
			stderr: []string{"Error: no token provided\nProvide a token: ytr auth login --token TOKEN\n"},
		},
		{
			name: "No org ID", args: login(), signedOut: true, stdin: "tok\n", code: ytrerrors.ExitUserError,
			stderr: []string{"Error: org-id is required\nUse --org-id flag: ytr auth login --org-id ORG\n"},
		},
		{
			name: "Bad org type", args: login(slices.Concat(flags, []string{"--org-type", "invalid"})...),
			signedOut: true, code: ytrerrors.ExitUserError,
			stderr: []string{"Error: invalid org-type \"invalid\"\nUse --org-type 360 or --org-type cloud\n"},
		},
	})
}

func TestAuthStatus(t *testing.T) {
	t.Parallel()

	const cloudConfig = "token: valid-token\norg_id: org-123\norg_type: cloud\n"
	status := []string{"auth", "status"}
	jq := []string{"auth", "status", "--jq", "."}

	runLeafRows(t, []leafRow{
		{
			name: "From the config", args: status, signedOut: true, config: cloudConfig,
			exchanges: []faketracker.Exchange{myselfAnswer("Status User")},
			stderr: []string{
				"Authenticated as Status User\n  Token source: config\n  Organization: org-123\n" +
					"  Organization type: cloud\n",
			},
		},
		{
			name: "JSON from the config", args: jq, signedOut: true, config: cloudConfig,
			exchanges: []faketracker.Exchange{myselfAnswer("JQ Status User")},
			json: `{"status": "authenticated", "user": "JQ Status User", "org_id": "org-123", "org_type": "cloud",
				"token_source": "config"}`,
		},
		{
			name: "From the environment", args: jq, signedOut: true,
			env:       map[string]string{"YTR_TOKEN": "env-token", "YTR_ORG_ID": "env-org", "YTR_ORG_TYPE": "360"},
			exchanges: []faketracker.Exchange{myselfAnswer("Env User")},
			json: `{"status": "authenticated", "user": "Env User", "org_id": "env-org", "org_type": "360",
				"token_source": "env"}`,
		},
		{
			name: "From the flags", args: []string{"auth", "status", "--jq", "."},
			exchanges: []faketracker.Exchange{myselfAnswer("Flag User")},
			json: `{"status": "authenticated", "user": "Flag User", "org_id": "test-org", "org_type": "360",
				"token_source": "flag"}`,
		},
		{
			name: "Fields", args: []string{"auth", "status", "--json", "status"},
			exchanges: []faketracker.Exchange{myselfAnswer("Flag User")}, json: `{"status": "authenticated"}`,
		},
		{
			name: "jq", args: []string{"auth", "status", "--jq", ".user"},
			exchanges: []faketracker.Exchange{myselfAnswer("Flag User")}, stdout: "Flag User\n",
		},
		{
			name: "Field hint signed out", args: []string{"auth", "status", "--json="}, signedOut: true,
			code: ytrerrors.ExitUserError,
			stderr: []string{
				fieldHint("auth status", []string{"status", "user", "org_id", "org_type", "token_source"}),
			},
		},
		{
			name: "Signed out", args: status, signedOut: true, code: ytrerrors.ExitAuthError,
			stderr: []string{"Error: not authenticated\n"},
		},
		{
			name: "Rejected token", args: status, signedOut: true, config: cloudConfig,
			exchanges: []faketracker.Exchange{myselfError(http.StatusForbidden, "Token expired")},
			code:      ytrerrors.ExitAuthError, stderr: []string{"Error: Token expired\n"},
		},
	})
}

func TestAuthLogout(t *testing.T) {
	t.Parallel()

	const signedIn = "token: some-token\norg_id: some-org\norg_type: \"360\"\n"
	logout := []string{"auth", "logout"}

	runLeafRows(t, []leafRow{
		{
			name: "Clears the credentials and keeps the file", args: logout, config: signedIn,
			check: func(t *testing.T, res cliResult) {
				t.Helper()
				assertConfigFile("{}\n")(t, res)
				assertStderrNamesTheConfig("Logged out. Credentials removed from ")(t, res)
			},
			stderr: []string{"Logged out. Credentials removed from "},
		},
		{
			name: "No config yet", args: logout, stderr: []string{"Logged out. Credentials removed from "},
			check: assertConfigFile("{}\n"),
		},
		{
			name: "JSON", args: []string{"auth", "logout", "--json", "status"}, config: signedIn,
			json: `{"status": "logged_out"}`, check: assertConfigFile("{}\n"),
		},
		{
			name: "jq of the whole document", args: []string{"auth", "logout", "--jq", "."}, config: signedIn,
			holds: []string{`"status":"logged_out"`}, check: assertJSONNamesTheConfig(`{"status": "logged_out"}`),
		},
		{
			name: "Field hint before the config is written", args: []string{"auth", "logout", "--json="},
			config: signedIn, code: ytrerrors.ExitUserError,
			stderr: []string{fieldHint("auth logout", []string{"status", "config_path"})},
			check:  assertConfigFile(signedIn),
		},
	})
}
