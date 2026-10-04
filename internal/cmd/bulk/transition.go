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
and stdin is not read for issues. The command waits for the operation to
complete by default.

JSON FIELDS
  id, status, statusText, totalIssues, totalCompletedIssues,
  executionIssuePercent, executionChunkPercent, createdBy, createdById, createdAt`,
		Example: `  # Transition issues to resolved
  ytr bulk transition PROJ-1 PROJ-2 --transition close

  # Transition with field updates
  ytr bulk transition PROJ-1 --transition close --field resolution=fixed

  # Transition via stdin pipe
  ytr issue list --quiet | ytr bulk transition --transition close

  # Transition via JSON
  ytr bulk transition --from-json '{"transition":"close","issues":["PROJ-1"]}'`,
	})

	cmd.Flags().String("transition", "", "Transition ID or key (required unless --from-json)")

	return cmd
}

var transitionChange = change[tracker.BulkTransitionRequest]{
	body: validate.Body{
		Flags:    []validate.BodyFlag{{Name: "transition", Key: "transition"}, fieldFlag},
		Required: []string{"transition"},
		FromJSON: true,
	},
	issues: func(req *tracker.BulkTransitionRequest) []string { return req.Issues },
	start:  (*tracker.BulkChangeService).Transition,
}
