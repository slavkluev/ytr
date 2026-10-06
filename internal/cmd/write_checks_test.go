package cmd

import (
	"slices"
	"strings"
	"testing"

	ytrerrors "github.com/slavkluev/ytr/internal/errors"
	"github.com/slavkluev/ytr/internal/jsonenc"
)

// writeCheck is one invocation a write leaf must refuse before any request,
// with the error every write leaf words the same way.
type writeCheck struct {
	args                []string
	message, suggestion string
}

const bothSuggestion = "Pass the request as flags or as --from-json, not both"

// TestWriteChecksShareOneWording runs the conflict, missing and
// nothing-to-update checks of every write leaf: each names the flags in the
// order the leaf declares them, and a conflict names only the flags that were
// set.
func TestWriteChecksShareOneWording(t *testing.T) {
	t.Parallel()

	conflicts := []writeCheck{
		{args: []string{"issue", "create", "--queue", "PROJ", "--from-json", `{"queue": "PROJ"}`},
			message: "cannot combine --from-json with --queue"},
		{args: []string{"issue", "create", "--type", "bug", "--summary", "x", "--from-json", `{}`},
			message: "cannot combine --from-json with --summary, --type"},
		{args: []string{"issue", "update", "PROJ-1", "--parent", "PROJ-2", "--from-json", `{}`},
			message: "cannot combine --from-json with --parent"},
		{args: []string{"issue", "transition", "PROJ-1", "--to", "x", "--from-json", `{}`},
			message: "cannot combine --from-json with --to"},
		{args: []string{"comment", "create", "PROJ-1", "--body", "x", "--from-json", `{}`},
			message: "cannot combine --from-json with --body"},
		{args: []string{"comment", "edit", "PROJ-1", "555", "--body", "x", "--from-json", `{}`},
			message: "cannot combine --from-json with --body"},
		{args: []string{"worklog", "create", "PROJ-1", "--comment", "x", "--duration", "PT1H", "--from-json", `{}`},
			message: "cannot combine --from-json with --duration, --comment"},
		{args: []string{"worklog", "edit", "PROJ-1", "101", "--start", "x", "--from-json", `{}`},
			message: "cannot combine --from-json with --start"},
		{args: []string{"checklist", "create", "PROJ-1", "--assignee", "uid-b", "--from-json", `{}`},
			message: "cannot combine --from-json with --assignee"},
		{args: []string{"checklist", "edit", "PROJ-1", "item-2", "--checked=false", "--from-json", `{}`},
			message: "cannot combine --from-json with --checked"},
		{args: []string{"link", "create", "PROJ-1", "--issue", "PROJ-2", "--type", "relates", "--from-json", `{}`},
			message: "cannot combine --from-json with --type, --issue"},
	}
	for i := range conflicts {
		conflicts[i].suggestion = bothSuggestion
	}

	missing := []writeCheck{
		{
			args:       []string{"issue", "create"},
			message:    "missing --queue, --summary",
			suggestion: `Pass them as flags, or as the keys "queue", "summary" in --from-json`,
		},
		{
			args:       []string{"issue", "create", "--queue", "PROJ"},
			message:    "missing --summary",
			suggestion: `Pass it as a flag, or as the key "summary" in --from-json`,
		},
		{
			args:       []string{"issue", "create", "--from-json", `{"summary": "x", "queue": null}`},
			message:    "missing --queue",
			suggestion: `Pass it as a flag, or as the key "queue" in --from-json`,
		},
		{
			args:       []string{"issue", "create", "--from-json", `{"QUEUE": "PROJ"}`},
			message:    "missing --summary",
			suggestion: `Pass it as a flag, or as the key "summary" in --from-json`,
		},
		{
			args:       []string{"issue", "transition", "PROJ-1"},
			message:    "missing --to",
			suggestion: `Pass it as a flag, or as the key "to" in --from-json`,
		},
		{
			args:       []string{"issue", "transition", "PROJ-1", "--from-json", `{}`},
			message:    "missing --to",
			suggestion: `Pass it as a flag, or as the key "to" in --from-json`,
		},
		{
			args:       []string{"comment", "create", "PROJ-1"},
			message:    "missing --body",
			suggestion: `Pass it as a flag, or as the key "text" in --from-json`,
		},
		{
			args:       []string{"comment", "create", "PROJ-1", "--from-json", `{"summonees": ["uid-a"]}`},
			message:    "missing --body",
			suggestion: `Pass it as a flag, or as the key "text" in --from-json`,
		},
		{
			args:       []string{"worklog", "create", "PROJ-1"},
			message:    "missing --duration, --start",
			suggestion: `Pass them as flags, or as the keys "duration", "start" in --from-json`,
		},
		{
			args:       []string{"worklog", "create", "PROJ-1", "--duration", "PT1H"},
			message:    "missing --start",
			suggestion: `Pass it as a flag, or as the key "start" in --from-json`,
		},
		{
			args:       []string{"worklog", "create", "PROJ-1", "--from-json", `{"comment": "x"}`},
			message:    "missing --duration, --start",
			suggestion: `Pass them as flags, or as the keys "duration", "start" in --from-json`,
		},
		{
			args:       []string{"checklist", "create", "PROJ-1", "--assignee", "uid-b"},
			message:    "missing --text",
			suggestion: `Pass it as a flag, or as the key "text" in --from-json`,
		},
		{
			args:       []string{"checklist", "create", "PROJ-1", "--from-json", `{"assignee": "uid-b"}`},
			message:    "missing --text",
			suggestion: `Pass it as a flag, or as the key "text" in --from-json`,
		},
		{
			args:       []string{"link", "create", "PROJ-1", "--issue", "PROJ-2"},
			message:    "missing --type",
			suggestion: `Pass it as a flag, or as the key "relationship" in --from-json`,
		},
		{
			args:       []string{"link", "create", "PROJ-1", "--from-json", `{}`},
			message:    "missing --type, --issue",
			suggestion: `Pass them as flags, or as the keys "relationship", "issue" in --from-json`,
		},
	}

	nothing := []writeCheck{
		{
			args: []string{"issue", "update", "PROJ-1"},
			suggestion: "Pass at least one of --summary, --description, --type, --priority, --assignee, --parent, " +
				"or a --from-json object with at least one key",
		},
		{
			args: []string{"issue", "update", "PROJ-1", "--from-json", `{}`},
			suggestion: "Pass at least one of --summary, --description, --type, --priority, --assignee, --parent, " +
				"or a --from-json object with at least one key",
		},
		{
			args:       []string{"comment", "edit", "PROJ-1", "555"},
			suggestion: "Pass --body, or a --from-json object with at least one key",
		},
		{
			args:       []string{"comment", "edit", "PROJ-1", "555", "--from-json", `{}`},
			suggestion: "Pass --body, or a --from-json object with at least one key",
		},
		{
			args:       []string{"comment", "edit", "PROJ-1", "555", "--from-json", `{"text": null}`},
			suggestion: "Pass --body, or a --from-json object with at least one key",
		},
		{
			args:       []string{"comment", "edit", "PROJ-1", "555", "--from-json", `{"attachmentIds": []}`},
			suggestion: "Pass --body, or a --from-json object with at least one key",
		},
		{
			args:       []string{"worklog", "edit", "PROJ-1", "101"},
			suggestion: "Pass at least one of --duration, --comment, --start, or a --from-json object with at least one key",
		},
		{
			args:       []string{"worklog", "edit", "PROJ-1", "101", "--from-json", `{}`},
			suggestion: "Pass at least one of --duration, --comment, --start, or a --from-json object with at least one key",
		},
		{
			args:       []string{"checklist", "edit", "PROJ-1", "item-2"},
			suggestion: "Pass at least one of --text, --checked, --assignee, or a --from-json object with at least one key",
		},
		{
			args:       []string{"checklist", "edit", "PROJ-1", "item-2", "--from-json", `null`},
			suggestion: "Pass at least one of --text, --checked, --assignee, or a --from-json object with at least one key",
		},
	}
	for i := range nothing {
		nothing[i].message = "nothing to update"
	}

	var rows []leafRow
	for _, check := range slices.Concat(conflicts, missing, nothing) {
		doc, err := jsonenc.Marshal(struct {
			Code       string `json:"code"`
			Message    string `json:"message"`
			Suggestion string `json:"suggestion"`
		}{ytrerrors.CodeUserError, check.message, check.suggestion})
		if err != nil {
			t.Fatalf("encoding the expected error document: %v", err)
		}

		rows = append(rows, leafRow{
			name: strings.Join(check.args, " "), args: check.args, code: ytrerrors.ExitUserError,
			stderr: []string{string(doc) + "\n"},
		})
	}

	runLeafRows(t, rows)
}
