package cmd

import (
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
	userHelpFields = "JSON FIELDS\n  uid, display, login, email, firstName, lastName, dismissed, hasLicense, external\n\n"
)

var userFields = []string{
	"uid", "display", "login", "email", "firstName", "lastName", "dismissed", "hasLicense", "external",
}

func TestUserMyself(t *testing.T) {
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
		fieldHintRow("user myself", nil, userFields...),
		notFoundRow(path, "user", "myself", "--json", "uid"),
		helpRow("user myself", userHelpFields+"SEE ALSO\n  ytr user get     - Show user details by UID\n"+
			"  ytr user list    - List organization users\n"),
	})
}

func TestUserGet(t *testing.T) {
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
		fieldHintRow("user get", []string{"12345"}, userFields...),
		notFoundRow(path, "user", "get", "12345", "--json", "uid"),
		helpRow("user get", userHelpFields+"SEE ALSO\n  ytr user myself   - Show current user\n"+
			"  ytr user list     - List organization users\n"),
	})
}
