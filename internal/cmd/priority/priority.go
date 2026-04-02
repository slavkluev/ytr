// Package priority provides priority commands for the ytr CLI.
package priority

import (
	"context"

	"github.com/slavkluev/go-yandex-tracker/tracker"
	"github.com/spf13/cobra"

	"github.com/slavkluev/ytr/internal/api"
	"github.com/slavkluev/ytr/internal/config"
)

// priorityLister abstracts priority list operations for testability.
type priorityLister interface {
	List(ctx context.Context, opts *tracker.PriorityListOptions) ([]*tracker.Priority, *tracker.Response, error)
}

// newPriorityLister creates a priorityLister from resolved auth. Replaceable for testing.
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
