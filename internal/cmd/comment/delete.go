package comment

import (
	"context"

	"github.com/slavkluev/go-yandex-tracker/tracker"
	"github.com/spf13/cobra"

	"github.com/slavkluev/ytr/internal/cmd/runner"
)

func newDeleteCmd() *cobra.Command {
	return runner.Delete{
		Use:   "delete ISSUE-KEY COMMENT-ID",
		Short: "Delete a comment",
		Long:  `Delete a comment from a Yandex Tracker issue.`,
		Example: `  # Delete comment 42 from PROJ-123
  ytr comment delete PROJ-123 42

  # Delete and print only the deleted ID
  ytr comment delete PROJ-123 42 --json id`,
		Args: []runner.Arg{runner.IssueKey, runner.NumericID("comment ID")},
		Call: func(ctx context.Context, c *tracker.Client, args []string) error {
			_, err := c.Issues.DeleteComment(ctx, args[0], args[1])
			return err
		},
	}.Command()
}
