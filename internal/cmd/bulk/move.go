package bulk

import (
	"github.com/slavkluev/go-yandex-tracker/tracker"
	"github.com/spf13/cobra"

	"github.com/slavkluev/ytr/internal/validate"
)

func newMoveCmd() *cobra.Command {
	cmd := moveChange.command(&cobra.Command{
		Use:   "move [ISSUE-KEY...]",
		Short: "Move issues to another queue",
		Long: `Move multiple Yandex Tracker issues to a different queue in a single
bulk operation.

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
		Example: `  # Move issues to another queue
  ytr bulk move PROJ-1 PROJ-2 PROJ-3 --queue TARGET

  # Move via stdin pipe
  ytr issue list --jq '.items[].key' | ytr bulk move --queue TARGET

  # Move with field updates
  ytr bulk move PROJ-1 PROJ-2 --queue TARGET --field priority=critical

  # Move via JSON (advanced options like moveAllFields)
  ytr bulk move --from-json '{"queue":"TARGET","issues":["PROJ-1"],"moveAllFields":true}'`,
	})

	cmd.Flags().String("queue", "", "Target queue key (required unless --from-json)")

	return cmd
}

var moveChange = change[tracker.BulkMoveRequest]{
	body: validate.Body{
		Flags:    []validate.BodyFlag{{Name: "queue", Key: "queue"}, fieldFlag},
		Required: []string{"queue"},
		FromJSON: true,
	},
	issues: func(req *tracker.BulkMoveRequest) []string { return req.Issues },
	start:  (*tracker.BulkChangeService).Move,
}
