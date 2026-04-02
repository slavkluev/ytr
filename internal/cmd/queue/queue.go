// Package queue provides queue management commands for the ytr CLI.
package queue

import (
	"context"

	"github.com/slavkluev/go-yandex-tracker/tracker"
	"github.com/spf13/cobra"

	"github.com/slavkluev/ytr/internal/api"
	"github.com/slavkluev/ytr/internal/config"
)

// queueLister abstracts queue list operations for testability.
type queueLister interface {
	List(ctx context.Context, opts *tracker.QueueListOptions) ([]*tracker.Queue, *tracker.Response, error)
}

// queueGetter abstracts single queue retrieval for testability.
type queueGetter interface {
	Get(ctx context.Context, key string, opts *tracker.QueueGetOptions) (*tracker.Queue, *tracker.Response, error)
}

// newLister creates a queueLister from resolved auth. Replaceable for testing.
var newLister = func(auth *config.ResolvedAuth) queueLister {
	return api.NewClient(auth).Queues
}

// newGetter creates a queueGetter from resolved auth. Replaceable for testing.
var newGetter = func(auth *config.ResolvedAuth) queueGetter {
	return api.NewClient(auth).Queues
}

// NewCmd creates the parent "queue" command with list and view subcommands.
func NewCmd() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "queue",
		Short: "Manage queues",
		Long:  "List and view Yandex Tracker queues.",
	}

	cmd.AddCommand(newListCmd())
	cmd.AddCommand(newViewCmd())

	return cmd
}
