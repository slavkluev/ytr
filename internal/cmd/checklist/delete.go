package checklist

import (
	"context"

	"github.com/slavkluev/go-yandex-tracker/tracker"
	"github.com/spf13/cobra"

	"github.com/slavkluev/ytr/internal/cmd/runner"
)

func newDeleteCmd() *cobra.Command {
	return runner.Delete{
		Use:   "delete ISSUE-KEY ITEM-ID",
		Short: "Delete a checklist item",
		Long:  `Delete a checklist item from a Yandex Tracker issue.`,
		Example: `  # Delete checklist item
  ytr checklist delete PROJ-123 item-1

  # Delete and confirm via JSON
  ytr checklist delete PROJ-123 item-1 --json id`,
		Args: []runner.Arg{runner.IssueKey, runner.StringID("checklist item ID")},
		Call: func(ctx context.Context, c *tracker.Client, args []string) error {
			_, _, err := c.Issues.DeleteChecklistItem(ctx, args[0], args[1])
			return err
		},
		Confirm: func(id string) string { return "Checklist item " + id + " deleted" },
	}.Command()
}
