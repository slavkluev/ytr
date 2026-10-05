package component

import (
	"context"

	"github.com/slavkluev/go-yandex-tracker/tracker"
	"github.com/spf13/cobra"

	"github.com/slavkluev/ytr/internal/cmd/runner"
)

func newCreateCmd() *cobra.Command {
	return runner.Write[tracker.ComponentRequest, *tracker.Component, componentItem]{
		Use:   "create",
		Short: "Create a component",
		Long: `Create a new project component in Yandex Tracker.

Provide --name and --queue for required fields, or --from-json for full JSON input.`,
		Example: `  # Create a simple component
  ytr component create --name "Backend" --queue PROJ

  # Create with all fields
  ytr component create --name "Backend" --queue PROJ --description "Backend services" --lead 12345 --assign-auto

  # Create via JSON
  ytr component create --from-json '{"name":"Backend","queue":"PROJ"}'`,
		Flags:    componentFlags("Component name (required)", "Queue key (required)"),
		FromJSON: `JSON input: inline '{"name":"...","queue":"..."}', @file, or - for stdin`,
		Required: []string{"name", "queue"},
		Call: func(
			ctx context.Context, c *tracker.Client, _ []string, req *tracker.ComponentRequest,
		) (*tracker.Component, error) {
			component, _, err := c.Components.Create(ctx, req)
			return component, err
		},
		Item: toComponentItem,
	}.Command()
}

// componentFlags are the request flags of create and edit, which word only
// the help of --name and --queue differently.
func componentFlags(name, queue string) []runner.Flag {
	return []runner.Flag{
		runner.Text("name", name),
		runner.Text("queue", queue),
		runner.Text("description", "Component description"),
		runner.Text("lead", "Lead user ID"),
		runner.Bool("assign-auto", "Auto-assign issues to lead").Key("assignAuto"),
	}
}
