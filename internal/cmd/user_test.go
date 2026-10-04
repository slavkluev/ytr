package cmd

import (
	"net/http"
	"slices"
	"testing"

	ytrerrors "github.com/slavkluev/ytr/internal/errors"
	"github.com/slavkluev/ytr/internal/faketracker"
	"github.com/slavkluev/ytr/internal/output"
)

const (
	trackerUser = `{"uid": 12345, "display": "John Doe", "login": "john.doe", "email": "john@example.com",
		"firstName": "John", "lastName": "Doe", "dismissed": false, "hasLicense": true, "external": false}`
	userAllFields = "uid,display,login,email,firstName,lastName,dismissed,hasLicense,external"
	userJSON      = `{"uid": 12345, "display": "John Doe", "login": "john.doe", "email": "john@example.com",
		"firstName": "John", "lastName": "Doe", "dismissed": false, "hasLicense": true, "external": false}`
	userCard = "UID\t12345\nDisplay\tJohn Doe\nLogin\tjohn.doe\nEmail\tjohn@example.com\n" +
		"First Name\tJohn\nLast Name\tDoe\nDismissed\tfalse\nHas License\ttrue\nExternal\tfalse\n"
	userTTYCard = "UID:  12345\nDisplay:  John Doe\nLogin:  john.doe\nEmail:  john@example.com\n" +
		"First Name:  John\nLast Name:  Doe\nDismissed:  false\nHas License:  true\nExternal:  false\n"
)

func TestUserMyself(t *testing.T) {
	t.Parallel()

	const path = "/v3/myself"
	user := trackerGET(path, trackerUser)

	runLeafRows(t, []leafRow{
		{
			name: "JSON", args: []string{"user", "myself", "--json", userAllFields},
			exchanges: []faketracker.Exchange{user}, json: userJSON,
		},
		{
			name: "JSON of a bare user", args: []string{"user", "myself", "--jq", "."},
			exchanges: []faketracker.Exchange{trackerGET(path, `{}`)},
			json: `{"uid": 0, "display": "", "login": "", "dismissed": false, "hasLicense": false,
				"external": false}`,
		},
		{
			name: "Card", args: []string{"user", "myself"}, exchanges: []faketracker.Exchange{user},
			stdout: userCard,
		},
		{
			name: "Card of a bare user", args: []string{"user", "myself"},
			exchanges: []faketracker.Exchange{trackerGET(path, `{}`)},
			stdout: "UID\t0\nDisplay\t-\nLogin\t-\nEmail\t-\nFirst Name\t-\nLast Name\t-\n" +
				"Dismissed\tfalse\nHas License\tfalse\nExternal\tfalse\n",
		},
		{
			name: "TTY", args: []string{"user", "myself"}, term: output.Options{TTY: true, Colors: true},
			exchanges: []faketracker.Exchange{user}, holds: []string{userTTYCard},
		},
		{
			name: "Quiet", args: []string{"user", "myself", "--quiet"},
			exchanges: []faketracker.Exchange{user}, stdout: "12345\n",
		},
		{
			name: "jq", args: []string{"user", "myself", "--jq", ".login"},
			exchanges: []faketracker.Exchange{user}, stdout: "john.doe\n",
		},
		notFoundRow(path, "user", "myself", "--json", "uid"),
	})
}

func TestUserGet(t *testing.T) {
	t.Parallel()

	const path = "/v3/users/12345"
	user := trackerGET(path, trackerUser)

	runLeafRows(t, []leafRow{
		{
			name: "JSON", args: []string{"user", "get", "12345", "--json", userAllFields},
			exchanges: []faketracker.Exchange{user}, json: userJSON,
		},
		{
			name: "Card", args: []string{"user", "get", "12345"}, exchanges: []faketracker.Exchange{user},
			stdout: userCard,
		},
		{
			name: "TTY", args: []string{"user", "get", "12345"}, term: output.Options{TTY: true, Colors: true},
			exchanges: []faketracker.Exchange{user}, holds: []string{userTTYCard},
		},
		{
			name: "Quiet", args: []string{"user", "get", "12345", "--quiet"},
			exchanges: []faketracker.Exchange{user}, stdout: "12345\n",
		},
		{
			name: "jq", args: []string{"user", "get", "12345", "--json", "login", "--jq", ".login"},
			exchanges: []faketracker.Exchange{user}, stdout: "john.doe\n",
		},
		{
			name: "ID without its spaces", args: []string{"user", "get", " 12345 ", "--quiet"},
			exchanges: []faketracker.Exchange{user}, stdout: "12345\n",
		},
		{
			name: "Bad arg", args: []string{"user", "get", " "}, code: ytrerrors.ExitUserError,
			stderr: []string{"invalid user ID: expected a non-empty value"},
		},
		{
			name: "Empty arg", args: []string{"user", "get", ""}, code: ytrerrors.ExitUserError,
			stderr: []string{"invalid user ID: expected a non-empty value"},
		},
		{
			name: "Bad arg before the hint", args: []string{"user", "get", " ", "--json="},
			code: ytrerrors.ExitUserError, stderr: []string{"invalid user ID"},
		},
		notFoundRow(path, "user", "get", "12345", "--json", "uid"),
	})
}

func userPage(page, perPage, total int, users string) faketracker.Exchange {
	return countedPage(http.MethodGet, "/v3/users", page, perPage, total, users)
}

func TestUserList(t *testing.T) {
	t.Parallel()

	const users = `[
		{"uid": 100, "display": "Alice", "login": "alice", "email": "alice@example.com"},
		{"uid": 200, "display": "Bob", "login": "bob", "email": "bob@example.com"}]`
	two := userPage(1, 50, 2, users)
	empty := userPage(1, 50, 0, `[]`)
	list := func(extra ...string) []string { return slices.Concat([]string{"user", "list"}, extra) }

	runLeafRows(t, []leafRow{
		{
			name:      "Table",
			args:      list(),
			exchanges: []faketracker.Exchange{two},
			stdout:    "UID\tDISPLAY\tLOGIN\tEMAIL\n100\tAlice\talice\talice@example.com\n200\tBob\tbob\tbob@example.com\n",
		},
		{
			name: "TTY", args: list(), term: output.Options{TTY: true, Colors: true},
			exchanges: []faketracker.Exchange{two},
			holds: []string{
				"UID  DISPLAY  LOGIN  EMAIL",
				"100  Alice    alice  alice@example.com",
				"200  Bob      bob    bob@example.com",
			},
			check: assertAlignedTable,
		},
		{
			name: "JSON", args: list("--json", "uid,display,login,email"), exchanges: []faketracker.Exchange{two},
			json: `{"items": [
				{"uid": 100, "display": "Alice", "login": "alice", "email": "alice@example.com"},
				{"uid": 200, "display": "Bob", "login": "bob", "email": "bob@example.com"}],
				"pagination": {"hasMore": false, "total": 2}}`,
		},
		{
			name:      "JSON of a bare user",
			args:      list("--json", "uid,display,login,email"),
			exchanges: []faketracker.Exchange{userPage(1, 50, 1, `[{}]`)},
			json:      `{"items": [{"uid": 0, "display": "", "login": ""}], "pagination": {"hasMore": false, "total": 1}}`,
		},
		{
			name: "Table of a bare user", args: list(), exchanges: []faketracker.Exchange{userPage(1, 50, 1, `[{}]`)},
			stdout: "UID\tDISPLAY\tLOGIN\tEMAIL\n0\t-\t-\t-\n",
		},
		{
			name: "A full page has more", args: list("--limit", "2", "--json", "uid"),
			exchanges: []faketracker.Exchange{userPage(1, 2, 5, users)},
			json: `{"items": [{"uid": 100}, {"uid": 200}],
				"pagination": {"cursor": "2", "hasMore": true, "total": 5}}`,
		},
		{
			name: "Quiet", args: list("--quiet"), exchanges: []faketracker.Exchange{two}, stdout: "100\n200\n",
		},
		{
			name: "jq", args: list("--jq", ".items[0].login"), exchanges: []faketracker.Exchange{two},
			stdout: "alice\n",
		},
		{
			name: "Limit", args: list("--limit", "10", "--quiet"),
			exchanges: []faketracker.Exchange{userPage(1, 10, 0, `[]`)},
		},
		{
			name: "Limit at the maximum", args: list("--limit", "1000", "--quiet"),
			exchanges: []faketracker.Exchange{userPage(1, 1000, 0, `[]`)},
		},
		{
			name: "Cursor", args: list("--cursor", "2", "--quiet"),
			exchanges: []faketracker.Exchange{userPage(2, 50, 10, users)}, stdout: "100\n200\n",
		},
		{
			name: "Not a page cursor", args: list("--cursor", "abc"), signedOut: true, code: ytrerrors.ExitUserError,
			stderr: []string{"invalid cursor"},
		},
		{
			name: "All pages", args: list("--all", "--limit", "2", "--quiet"),
			exchanges: []faketracker.Exchange{
				userPage(1, 2, 3, users), userPage(2, 2, 3, `[{"uid": 300, "login": "carol"}]`),
			},
			stdout: "100\n200\n300\n",
			check:  assertRequestOrder("page=1&perPage=2", "page=2&perPage=2"),
		},
		{
			name: "All pages end on a full page", args: list("--all", "--limit", "2", "--quiet"),
			exchanges: []faketracker.Exchange{
				userPage(1, 2, 4, users),
				userPage(2, 2, 4, `[{"uid": 300, "login": "carol"}, {"uid": 400, "login": "dave"}]`),
			},
			stdout: "100\n200\n300\n400\n",
			check:  assertRequestOrder("page=1&perPage=2", "page=2&perPage=2"),
		},
		{
			name: "All pages as JSON", args: list("--all", "--limit", "2", "--json", "uid"),
			exchanges: []faketracker.Exchange{
				userPage(1, 2, 3, users), userPage(2, 2, 3, `[{"uid": 300, "login": "carol"}]`),
			},
			json: `{"items": [{"uid": 100}, {"uid": 200}, {"uid": 300}], "pagination": {"hasMore": false, "total": 3}}`,
		},
		{
			name: "A later page fails", args: list("--all", "--limit", "2", "--json", "uid"),
			exchanges: []faketracker.Exchange{
				userPage(1, 2, 3, users),
				withQuery(
					trackerError(http.MethodGet, "/v3/users", http.StatusInternalServerError, "Users unavailable"),
					pageQuery(2, 2),
				),
			},
			code: ytrerrors.ExitUserError, stderr: []string{`"message":"Users unavailable"`},
			check: assertOneErrorDocument("Users unavailable"),
		},
		{
			name: "Empty", args: list(), exchanges: []faketracker.Exchange{empty}, stdout: "No users found\n",
		},
		{
			name: "Empty as JSON", args: list("--json", "uid"), exchanges: []faketracker.Exchange{empty},
			json: `{"items": [], "pagination": {"hasMore": false}}`,
		},
		failureRow(withQuery(trackerNotFound("/v3/users"), pageQuery(1, 50)), list("--json", "uid")...),
	})
}
