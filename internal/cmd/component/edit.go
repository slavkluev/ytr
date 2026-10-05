package component

import (
	"context"

	"github.com/slavkluev/go-yandex-tracker/tracker"
	"github.com/spf13/cobra"

	"github.com/slavkluev/ytr/internal/cmd/runner"
)

func newEditCmd() *cobra.Command {
	return runner.Write[tracker.ComponentRequest, *tracker.Component, componentItem]{
		Use:   "edit COMPONENT-ID",
		Short: "Edit a component",
		Long: `Edit an existing project component in Yandex Tracker.

--from-json takes the request body as one JSON object. Each flag is shorthand
for the body key of the same name, and --assign-auto for "assignAuto".`,
		Example: `  # Update component name
  ytr component edit 42 --from-json '{"name":"New Name"}'

  # Update multiple fields
  ytr component edit 42 --from-json '{"name":"Backend","lead":"12345","assignAuto":true}'`,
		Args:   []runner.Arg{runner.NumericID("component ID")},
		Flags:  componentFlags("Component name", "Queue key"),
		Update: true,
		Call: func(
			ctx context.Context, c *tracker.Client, args []string, req *tracker.ComponentRequest,
		) (*tracker.Component, error) {
			component, _, err := c.Components.Edit(ctx, args[0], req)
			return component, err
		},
		Item: toComponentItem,
	}.Command()
}
