package component

import (
	"context"

	"github.com/slavkluev/go-yandex-tracker/tracker"
	"github.com/spf13/cobra"

	"github.com/slavkluev/ytr/internal/api"
	"github.com/slavkluev/ytr/internal/cmd/runner"
	"github.com/slavkluev/ytr/internal/output"
)

func newGetCmd() *cobra.Command {
	return runner.Get[*tracker.Component, componentItem]{
		Use:   "get COMPONENT-ID",
		Short: "Show component details",
		Long:  `Display detailed information about a Yandex Tracker component.`,
		SeeAlso: `  ytr component list    - List all components
  ytr component edit    - Edit a component
  ytr component delete  - Delete a component`,
		Example: `  # View component details
  ytr component get 42

  # Get specific fields as JSON
  ytr component get 42 --json name,queue,lead`,
		Args: []runner.Arg{runner.NumericID("component ID")},
		Call: func(ctx context.Context, c *tracker.Client, args []string) (*tracker.Component, error) {
			component, _, err := c.Components.Get(ctx, args[0])
			return component, err
		},
		Item:   toComponentItem,
		Detail: componentCard,
		Quiet:  func(c *tracker.Component) string { return api.DerefFlexString(c.ID, "") },
	}.Command()
}

func componentCard(d *output.DetailPrinter, _ *output.Options, c *tracker.Component) {
	d.Field("ID", api.DerefFlexString(c.ID, ""))
	d.Field("Name", api.DerefString(c.Name, "-"))
	d.Field("Queue", componentQueue(c, "-"))
	d.Field("Lead", c.Lead.DisplayOr("-"))

	if desc := api.DerefString(c.Description, ""); desc != "" {
		d.Field("Description", desc)
	}

	assignAuto := "no"
	if api.DerefBool(c.AssignAuto, false) {
		assignAuto = "yes"
	}
	d.Field("AssignAuto", assignAuto)
}
