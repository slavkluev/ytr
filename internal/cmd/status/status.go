// Package status provides status commands for the ytr CLI.
package status

import (
	"context"

	"github.com/slavkluev/go-yandex-tracker/tracker"
	"github.com/spf13/cobra"

	"github.com/slavkluev/ytr/internal/api"
	"github.com/slavkluev/ytr/internal/config"
)

// statusLister abstracts status list operations for testability.
type statusLister interface {
	List(ctx context.Context) ([]*tracker.Status, *tracker.Response, error)
}

// newStatusLister creates a statusLister from resolved auth. Replaceable for testing.
var newStatusLister = func(auth *config.ResolvedAuth) statusLister {
	return api.NewClient(auth).Statuses
}

// NewCmd creates the parent "status" command.
func NewCmd() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "status",
		Short: "Manage workflow statuses",
		Long:  "View workflow statuses in Yandex Tracker.",
	}
	cmd.AddCommand(newListCmd())
	return cmd
}
