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
		SeeAlso: `  ytr component list    - List all components
  ytr component get     - Show component details
  ytr component create  - Create a component`,
		Example: `  # Delete component 42
  ytr component delete 42

  # Delete and confirm via JSON
  ytr component delete 42 --json id`,
		Args: []runner.Arg{runner.NumericID("component ID")},
		Call: func(ctx context.Context, c *tracker.Client, args []string) error {
			_, err := c.Components.Delete(ctx, args[0])
			return err
		},
		Confirm: func(id string) string { return "Component " + id + " deleted" },
	}.Command()
}
