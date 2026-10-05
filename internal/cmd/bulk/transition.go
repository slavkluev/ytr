package bulk

import (
	"github.com/slavkluev/go-yandex-tracker/tracker"
	"github.com/spf13/cobra"

	"github.com/slavkluev/ytr/internal/validate"
)

func newTransitionCmd() *cobra.Command {
	cmd := transitionChange.command(&cobra.Command{
		Use:   "transition [ISSUE-KEY...]",
		Short: "Transition multiple issues to a new status",
		Long: `Transition multiple Yandex Tracker issues to a new status in a single
bulk operation.

Issues can be given as keys (PROJ-1) or 24-character hexadecimal issue IDs,
as positional arguments or piped via stdin (one per line). With --from-json,
the body's "issues" is the only source of issues: issue arguments are refused
and stdin is not read for issues. The command waits up to --timeout for the
operation to finish. One still running then is not a failure: the command
exits 0 with the status Tracker last reported, and suggestion is the
ytr bulk status command that checks it again. Running the command again would
start a second operation.

--from-json takes the request body as one JSON object. --transition is
shorthand for its "transition" key, --field for one entry of "values", and the
issue arguments for "issues".

JSON FIELDS
  id, status, statusText, totalIssues, totalCompletedIssues,
  executionIssuePercent, executionChunkPercent, createdBy, createdById, createdAt,
  suggestion`,
		Example: `  # Transition issues to resolved
  ytr bulk transition --from-json '{"transition":"close","issues":["PROJ-1","PROJ-2"]}'

  # Transition with field updates
  ytr bulk transition --from-json '{"transition":"close","issues":["PROJ-1"],"values":{"resolution":"fixed"}}'

  # Build the body from a search and pipe it in
  ytr issue list --filter queue=PROJ --all --jq '{transition:"close",issues:[.items[].key]}' | ytr bulk transition --from-json -`,
	})

	cmd.Flags().String("transition", "", "Transition ID or key (required)")

	return cmd
}

var transitionChange = change[tracker.BulkTransitionRequest]{
	body: validate.Body{
		Flags:    []validate.BodyFlag{{Name: "transition", Key: "transition"}, fieldFlag},
		Required: []string{"transition"},
	},
	issues: func(req *tracker.BulkTransitionRequest) []string { return req.Issues },
	start:  (*tracker.BulkChangeService).Transition,
}
