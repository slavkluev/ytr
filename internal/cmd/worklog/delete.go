package worklog

import (
	"context"

	"github.com/slavkluev/go-yandex-tracker/tracker"
	"github.com/spf13/cobra"

	"github.com/slavkluev/ytr/internal/cmd/runner"
)

func newDeleteCmd() *cobra.Command {
	return runner.Delete{
		Use:   "delete ISSUE-KEY WORKLOG-ID",
		Short: "Delete a worklog",
		Long:  `Delete a worklog from a Yandex Tracker issue.`,
		Example: `  # Delete worklog abc123 from PROJ-123
  ytr worklog delete PROJ-123 abc123

  # Delete and confirm via JSON
  ytr worklog delete PROJ-123 abc123 --jq '.deleted'`,
		Args: []runner.Arg{runner.IssueKey, runner.StringID("worklog ID")},
		Call: func(ctx context.Context, c *tracker.Client, args []string) error {
			_, err := c.Issues.DeleteWorklog(ctx, args[0], args[1])
			return err
		},
		Confirm: func(id string) string { return "Worklog " + id + " deleted" },
	}.Command()
}
