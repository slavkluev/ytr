// Package resolution provides resolution commands for the ytr CLI.
package resolution

import (
	"context"

	"github.com/slavkluev/go-yandex-tracker/tracker"
	"github.com/spf13/cobra"

	"github.com/slavkluev/ytr/internal/api"
	"github.com/slavkluev/ytr/internal/config"
)

// resolutionLister abstracts resolution list operations for testability.
type resolutionLister interface {
	List(ctx context.Context) ([]*tracker.Resolution, *tracker.Response, error)
}

// newResolutionLister creates a resolutionLister from resolved auth. Replaceable for testing.
var newResolutionLister = func(auth *config.ResolvedAuth) resolutionLister {
	return api.NewClient(auth).Resolutions
}

// NewCmd creates the parent "resolution" command.
func NewCmd() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "resolution",
		Short: "Manage resolutions",
		Long:  "View resolutions in Yandex Tracker.",
	}
	cmd.AddCommand(newListCmd())
	return cmd
}
