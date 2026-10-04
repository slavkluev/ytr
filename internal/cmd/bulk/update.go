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

Issue keys can be provided as positional arguments or piped via stdin
(one per line). The command waits for the operation to complete by default.

JSON FIELDS
  id, status, statusText, totalIssues, totalCompletedIssues,
  executionIssuePercent, executionChunkPercent, createdBy, createdById, createdAt`,
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
