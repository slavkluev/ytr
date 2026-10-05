package component

import (
	"context"

	"github.com/slavkluev/go-yandex-tracker/tracker"
	"github.com/spf13/cobra"

	"github.com/slavkluev/ytr/internal/cmd/runner"
)

func newGetCmd() *cobra.Command {
	return runner.Get[*tracker.Component, componentItem]{
		Use:   "get COMPONENT-ID",
		Short: "Show component details",
		Long:  `Display detailed information about a Yandex Tracker component.`,
		Example: `  # View component details
  ytr component get 42

  # Only the name, queue and lead
  ytr component get 42 --json name,queue,lead`,
		Args: []runner.Arg{runner.NumericID("component ID")},
		Call: func(ctx context.Context, c *tracker.Client, args []string) (*tracker.Component, error) {
			component, _, err := c.Components.Get(ctx, args[0])
			return component, err
		},
		Item: toComponentItem,
	}.Command()
}
