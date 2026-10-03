// Package priority provides priority commands for the ytr CLI.
package priority

import (
	"context"

	"github.com/slavkluev/go-yandex-tracker/tracker"
	"github.com/spf13/cobra"

	"github.com/slavkluev/ytr/internal/api"
	"github.com/slavkluev/ytr/internal/config"
)

type priorityLister interface {
	List(ctx context.Context, opts *tracker.PriorityListOptions) ([]*tracker.Priority, *tracker.Response, error)
}

var newPriorityLister = func(auth *config.ResolvedAuth) priorityLister {
	return api.NewClient(auth).Priorities
}

// NewCmd creates the parent "priority" command.
func NewCmd() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "priority",
		Short: "Manage priorities",
		Long:  "View priorities in Yandex Tracker.",
	}
	cmd.AddCommand(newListCmd())
	return cmd
}
