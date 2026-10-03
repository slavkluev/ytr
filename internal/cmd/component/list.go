package component

import (
	"context"

	"github.com/slavkluev/go-yandex-tracker/tracker"
	"github.com/spf13/cobra"

	"github.com/slavkluev/ytr/internal/api"
	"github.com/slavkluev/ytr/internal/cmd/runner"
	"github.com/slavkluev/ytr/internal/output"
)

type componentItem struct {
	ID          string `json:"id"`
	Name        string `json:"name"`
	Queue       string `json:"queue,omitempty"`
	Lead        string `json:"lead,omitempty"`
	LeadID      string `json:"leadId"`
	Description string `json:"description,omitempty"`
	AssignAuto  bool   `json:"assignAuto"`
}

func toComponentItem(c *tracker.Component) componentItem {
	return componentItem{
		ID:          api.DerefFlexString(c.ID, ""),
		Name:        api.DerefString(c.Name, ""),
		Queue:       componentQueue(c, ""),
		Lead:        c.Lead.DisplayOr(""),
		LeadID:      c.Lead.IDOr(""),
		Description: api.DerefString(c.Description, ""),
		AssignAuto:  api.DerefBool(c.AssignAuto, false),
	}
}

func componentQueue(c *tracker.Component, fallback string) string {
	if c.Queue == nil {
		return fallback
	}

	return api.DerefString(c.Queue.Key, fallback)
}

func newListCmd() *cobra.Command {
	return runner.List[*tracker.Component, componentItem]{
		Use:   "list",
		Short: "List components",
		Long:  `List all project components in Yandex Tracker.`,
		SeeAlso: `  ytr component get     - Show component details
  ytr component create  - Create a component`,
		Example: `  # List all components
  ytr component list

  # Get components as JSON
  ytr component list --json id,name,queue`,
		Empty: "No components found",
		Call: func(ctx context.Context, c *tracker.Client, _ []string) ([]*tracker.Component, error) {
			components, _, err := c.Components.List(ctx)
			return components, err
		},
		Item:   toComponentItem,
		Header: []string{"ID", "NAME", "QUEUE", "LEAD"},
		Row: func(_ *output.Options, c *tracker.Component) []string {
			return []string{
				api.DerefFlexString(c.ID, ""),
				api.DerefString(c.Name, "-"),
				componentQueue(c, "-"),
				c.Lead.DisplayOr("-"),
			}
		},
		Quiet: func(c *tracker.Component) string { return api.DerefFlexString(c.ID, "") },
	}.Command()
}
