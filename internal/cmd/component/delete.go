package component

import (
	"context"

	"github.com/slavkluev/go-yandex-tracker/tracker"
	"github.com/spf13/cobra"

	"github.com/slavkluev/ytr/internal/cmd/runner"
)

func newDeleteCmd() *cobra.Command {
	return runner.Delete{
		Use:   "delete COMPONENT-ID",
		Short: "Delete a component",
		Long:  `Delete a project component from Yandex Tracker.`,
		Example: `  # Delete component 42
  ytr component delete 42

  # Delete and print only the deleted ID
  ytr component delete 42 --json id`,
		Args: []runner.Arg{runner.NumericID("component ID")},
		Call: func(ctx context.Context, c *tracker.Client, args []string) error {
			_, err := c.Components.Delete(ctx, args[0])
			return err
		},
	}.Command()
}
