package cmd

import (
	"net/http"
	"slices"
	"testing"

	ytrerrors "github.com/slavkluev/ytr/internal/errors"
	"github.com/slavkluev/ytr/internal/faketracker"
)

const linkAnswer = `{"id": 101, "direction": "inward",
	"type": {"id": "depends", "inward": "depends on", "outward": "is dependency for"},
	"object": {"id": "4ff3e8dae4b0e2ac00000001", "key": "PROJ-2", "display": "Setup database"},
	"createdBy": {"id": "uid-a", "display": "Иван Петров"}}`

func TestLinkCreate(t *testing.T) {
	const path = "/v3/issues/PROJ-1/links"
	created := trackerPOST(path, linkAnswer)
	create := func(extra ...string) []string { return slices.Concat([]string{"link", "create", "PROJ-1"}, extra) }
	body := `{"relationship": "depends on", "issue": "PROJ-2"}`

	runLeafRows(t, []leafRow{
		{
			name: "Flag body", args: create("--type", "depends on", "--issue", "PROJ-2"),
			exchanges: []faketracker.Exchange{created}, body: body, stdout: "Link 101 created on PROJ-1\n",
		},
		{
			name: "JSON body", args: create("--from-json", body), exchanges: []faketracker.Exchange{created},
			body: body, stdout: "Link 101 created on PROJ-1\n",
		},
		{
			name: "JSON", args: create("--type", "depends on", "--issue", "PROJ-2", "--json", "id,type,issue,summary"),
			exchanges: []faketracker.Exchange{created},
			json:      `{"id": "101", "type": "depends on", "issue": "PROJ-2", "summary": ""}`,
		},
		{
			name: "Quiet", args: create("--type", "depends on", "--issue", "PROJ-2", "--quiet"),
			exchanges: []faketracker.Exchange{created}, stdout: "101\n",
		},
		{
			name: "jq", args: create("--type", "depends on", "--issue", "PROJ-2", "--jq", ".type"),
			exchanges: []faketracker.Exchange{created}, stdout: "depends on\n",
		},
		{
			name: "Conflict", args: create("--type", "relates", "--from-json", `{"relationship": "relates"}`),
			code: ytrerrors.ExitUserError, stderr: []string{"cannot use --type/--issue and --from-json together"},
		},
		{
			name:   "Conflict on the issue",
			args:   create("--issue", "PROJ-2", "--from-json", `{"relationship": "relates"}`),
			code:   ytrerrors.ExitUserError,
			stderr: []string{"cannot use --type/--issue and --from-json together"},
		},
		{
			name: "Missing", args: create(), code: ytrerrors.ExitUserError,
			stderr: []string{"both --type and --issue are required"},
		},
		{
			name: "Missing type", args: create("--issue", "PROJ-2"), code: ytrerrors.ExitUserError,
			stderr: []string{"both --type and --issue are required"},
		},
		{
			name: "Missing issue", args: create("--type", "relates"), code: ytrerrors.ExitUserError,
			stderr: []string{"both --type and --issue are required"},
		},
		{
			name: "Missing in JSON",
			args: create("--from-json", `{"issue": "PROJ-2"}`),
			exchanges: []faketracker.Exchange{
				created,
			},
			body:   `{"issue": "PROJ-2"}`,
			stdout: "Link 101 created on PROJ-1\n",
		},
		{
			name: "Bad value", args: create("--type", "relates", "--issue", "bad-key"), code: ytrerrors.ExitUserError,
			stderr: []string{`Error: invalid issue key "bad-key": expected format QUEUE-123`},
		},
		{
			name: "Unknown key", args: create("--from-json", `{"relationship": "relates", "bogus": 1}`, "--json", "id"),
			code: ytrerrors.ExitUserError, stderr: []string{`"code":"invalid_field"`, `"invalidFields":["bogus"]`},
		},
		{
			name: "Bad arg", args: []string{"link", "create", "bad-key", "--type", "relates", "--issue", "PROJ-2"},
			code: ytrerrors.ExitUserError, stderr: []string{`invalid issue key "bad-key"`},
		},
		fieldHintRow("link create", []string{"PROJ-1", "--type", "relates", "--issue", "PROJ-2"}, "id", "type", "issue",
			"summary"),
		failureRow("Tracker 404", trackerFailure(http.MethodPost, path, http.StatusNotFound, "Issue not found"),
			create("--type", "relates", "--issue", "PROJ-2", "--json", "id")...),
		failureRow(
			"Tracker 500",
			trackerFailure(http.MethodPost, path, http.StatusInternalServerError, "Internal error"),
			create("--type", "relates", "--issue", "PROJ-2", "--json", "id")...),
		helpRow(
			"link create",
			"Provide --type and --issue for individual flags, or --from-json for full JSON input.\n\n"+
				"JSON FIELDS\n  id, type, issue, summary\n\n"+
				"SEE ALSO\n  ytr link list    - List links on issue\n  ytr link delete  - Delete a link\n",
		),
	})
}

func TestLinkDelete(t *testing.T) {
	const path = "/v3/issues/PROJ-1/links/101"
	args := []string{"link", "delete", "PROJ-1", "101"}

	runLeafRows(t, slices.Concat(deleteRows(args, trackerDELETE(path), "101", "Link 101 deleted"), []leafRow{
		{
			name: "Trimmed ID", args: []string{"link", "delete", "PROJ-1", " 101 "},
			exchanges: []faketracker.Exchange{trackerDELETE(path)}, stdout: "Link 101 deleted\n",
		},
		{
			name: "Empty ID", args: []string{"link", "delete", "PROJ-1", " "}, code: ytrerrors.ExitUserError,
			stderr: []string{"invalid link ID: expected a non-empty value"},
		},
		{
			name: "Bad issue key", args: []string{"link", "delete", "bad", "101"}, code: ytrerrors.ExitUserError,
			stderr: []string{`invalid issue key "bad"`},
		},
		failureRow("Tracker 404", trackerFailure(http.MethodDelete, path, http.StatusNotFound, "Link not found"),
			slices.Concat(args, []string{"--json", "id"})...),
		helpRow("link delete", "Delete a link from a Yandex Tracker issue.\n\n"+
			"SEE ALSO\n  ytr link list    - List links on issue\n  ytr link create  - Create a link to another issue\n"),
	}))
}
