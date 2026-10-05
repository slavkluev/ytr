package bulk

import (
	"github.com/slavkluev/go-yandex-tracker/tracker"
	"github.com/spf13/cobra"

	"github.com/slavkluev/ytr/internal/validate"
)

func newUpdateCmd() *cobra.Command {
	return updateChange.command(&cobra.Command{
		Use:   "update [ISSUE-KEY...]",
		Short: "Update fields on multiple issues",
		Long: `Update fields on multiple Yandex Tracker issues in a single bulk operation.

Issues can be given as keys (PROJ-1) or 24-character hexadecimal issue IDs,
as positional arguments or piped via stdin (one per line). With --from-json,
the body's "issues" is the only source of issues: issue arguments are refused
and stdin is not read for issues. The command waits up to --timeout for the
operation to finish. One still running then is not a failure: the command
exits 0 with the status Tracker last reported, and suggestion is the
ytr bulk status command that checks it again. Running the command again would
start a second operation.

JSON FIELDS
  id, status, statusText, totalIssues, totalCompletedIssues,
  executionIssuePercent, executionChunkPercent, createdBy, createdById, createdAt,
  suggestion`,
		Example: `  # Update priority on multiple issues
  ytr bulk update PROJ-1 PROJ-2 --field priority=critical

  # Update multiple fields
  ytr bulk update PROJ-1 PROJ-2 --field priority=critical --field assignee=user123

  # Update via stdin pipe
  ytr issue list --quiet | ytr bulk update --field status=done

  # Update via JSON
  ytr bulk update --from-json '{"issues":["PROJ-1"],"values":{"priority":"critical"}}'`,
	})
}

var updateChange = change[tracker.BulkUpdateRequest]{
	body: validate.Body{
		Flags:    []validate.BodyFlag{fieldFlag},
		Required: []string{fieldFlag.Key},
		FromJSON: true,
	},
	issues: func(req *tracker.BulkUpdateRequest) []string { return req.Issues },
	start:  (*tracker.BulkChangeService).Update,
}
