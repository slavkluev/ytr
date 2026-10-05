package cmd

import (
	"net/http"
	"slices"
	"testing"

	ytrerrors "github.com/slavkluev/ytr/internal/errors"
	"github.com/slavkluev/ytr/internal/faketracker"
)

const (
	trackerUser = `{"uid": 12345, "display": "John Doe", "login": "john.doe", "email": "john@example.com",
		"firstName": "John", "lastName": "Doe", "dismissed": false, "hasLicense": true, "external": false}`
	userJSON = `{"uid": 12345, "display": "John Doe", "login": "john.doe", "email": "john@example.com",
		"firstName": "John", "lastName": "Doe", "dismissed": false, "hasLicense": true, "external": false}`
)

func TestUserMyself(t *testing.T) {
	t.Parallel()

	const path = "/v3/myself"
	user := trackerGET(path, trackerUser)

	runLeafRows(t, []leafRow{
		{
			name: "Every field", args: []string{"user", "myself"},
			exchanges: []faketracker.Exchange{user}, json: userJSON,
		},
		{
			name: "A bare user", args: []string{"user", "myself"},
			exchanges: []faketracker.Exchange{trackerGET(path, `{}`)},
			json: `{"uid": 0, "display": "", "login": "", "dismissed": false, "hasLicense": false,
				"external": false}`,
		},
		{
			name: "JSON", args: []string{"user", "myself", "--json", "uid,login"},
			exchanges: []faketracker.Exchange{user}, json: `{"uid": 12345, "login": "john.doe"}`,
		},
		{
			name: "jq", args: []string{"user", "myself", "--jq", ".login"},
			exchanges: []faketracker.Exchange{user}, stdout: "john.doe\n",
		},
		notFoundRow(path, "user", "myself"),
	})
}

func TestUserGet(t *testing.T) {
	t.Parallel()

	const path = "/v3/users/12345"
	user := trackerGET(path, trackerUser)

	runLeafRows(t, []leafRow{
		{
			name: "Every field", args: []string{"user", "get", "12345"},
			exchanges: []faketracker.Exchange{user}, json: userJSON,
		},
		{
			name: "jq", args: []string{"user", "get", "12345", "--json", "login", "--jq", ".login"},
			exchanges: []faketracker.Exchange{user}, stdout: "john.doe\n",
		},
		{
			name: "ID without its spaces", args: []string{"user", "get", " 12345 ", "--jq", ".uid"},
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
		notFoundRow(path, "user", "get", "12345"),
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
			name: "Every field", args: list(), exchanges: []faketracker.Exchange{two},
			json: `{"items": [
				{"uid": 100, "display": "Alice", "login": "alice", "email": "alice@example.com"},
				{"uid": 200, "display": "Bob", "login": "bob", "email": "bob@example.com"}],
				"pagination": {"hasMore": false, "total": 2}}`,
		},
		{
			name:      "A bare user",
			args:      list(),
			exchanges: []faketracker.Exchange{userPage(1, 50, 1, `[{}]`)},
			json:      `{"items": [{"uid": 0, "display": "", "login": ""}], "pagination": {"hasMore": false, "total": 1}}`,
		},
		{
			name: "A full page has more", args: list("--limit", "2", "--json", "uid"),
			exchanges: []faketracker.Exchange{userPage(1, 2, 5, users)},
			json: `{"items": [{"uid": 100}, {"uid": 200}],
				"pagination": {"cursor": "2", "hasMore": true, "total": 5}}`,
		},
		{
			name: "jq", args: list("--jq", ".items[0].login"), exchanges: []faketracker.Exchange{two},
			stdout: "alice\n",
		},
		{
			name: "Limit", args: list("--limit", "10", "--jq", ".items[].uid"),
			exchanges: []faketracker.Exchange{userPage(1, 10, 0, `[]`)},
		},
		{
			name: "Limit at the maximum", args: list("--limit", "1000", "--jq", ".items[].uid"),
			exchanges: []faketracker.Exchange{userPage(1, 1000, 0, `[]`)},
		},
		{
			name: "Cursor", args: list("--cursor", "2", "--jq", ".items[].uid"),
			exchanges: []faketracker.Exchange{userPage(2, 50, 10, users)}, stdout: "100\n200\n",
		},
		{
			name: "Not a page cursor", args: list("--cursor", "abc"), signedOut: true, code: ytrerrors.ExitUserError,
			stderr: []string{"invalid cursor"},
		},
		{
			name: "All pages", args: list("--all", "--limit", "2", "--jq", ".items[].uid"),
			exchanges: []faketracker.Exchange{
				userPage(1, 2, 3, users), userPage(2, 2, 3, `[{"uid": 300, "login": "carol"}]`),
			},
			stdout: "100\n200\n300\n",
			check:  assertRequestOrder("page=1&perPage=2", "page=2&perPage=2"),
		},
		{
			name: "All pages end on a full page", args: list("--all", "--limit", "2", "--jq", ".items[].uid"),
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
			name: "A later page fails", args: list("--all", "--limit", "2"),
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
			name: "Empty", args: list(), exchanges: []faketracker.Exchange{empty},
			json: `{"items": [], "pagination": {"hasMore": false}}`,
		},
		failureRow(withQuery(trackerNotFound("/v3/users"), pageQuery(1, 50)), list()...),
	})
}
