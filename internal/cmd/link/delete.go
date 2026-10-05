package link

import (
	"context"

	"github.com/slavkluev/go-yandex-tracker/tracker"
	"github.com/spf13/cobra"

	"github.com/slavkluev/ytr/internal/cmd/runner"
)

func newDeleteCmd() *cobra.Command {
	return runner.Delete{
		Use:   "delete ISSUE-KEY LINK-ID",
		Short: "Delete a link",
		Long:  `Delete a link from a Yandex Tracker issue.`,
		Example: `  # Delete link 456 from PROJ-123
  ytr link delete PROJ-123 456

  # Delete and print only the deleted ID
  ytr link delete PROJ-123 456 --json id`,
		Args: []runner.Arg{runner.IssueKey, runner.StringID("link ID")},
		Call: func(ctx context.Context, c *tracker.Client, args []string) error {
			_, err := c.Issues.DeleteLink(ctx, args[0], args[1])
			return err
		},
	}.Command()
}
