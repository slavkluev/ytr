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

// queueContextClient abstracts the read-only requests behind queue context,
// which span the queue, workflow, and field services, for testability.
type queueContextClient interface {
	GetQueue(ctx context.Context, key string, opts *tracker.QueueGetOptions) (*tracker.Queue, *tracker.Response, error)
	GetWorkflow(ctx context.Context, id string) (*tracker.Workflow, *tracker.Response, error)
	ListComponents(
		ctx context.Context,
		key string,
		opts *tracker.QueueComponentsListOptions,
	) ([]*tracker.Component, *tracker.Response, error)
	ListQueueFields(ctx context.Context, key string) ([]*tracker.Field, *tracker.Response, error)
	ListLocalFields(ctx context.Context, key string) ([]*tracker.Field, *tracker.Response, error)
	ListGlobalFields(ctx context.Context) ([]*tracker.Field, *tracker.Response, error)
}

// trackerContextClient implements queueContextClient over one tracker.Client.
// The services share method names (Queues.Get, Workflows.Get), so no single
// service satisfies the interface.
type trackerContextClient struct {
	client *tracker.Client
}

func (c trackerContextClient) GetQueue(
	ctx context.Context,
	key string,
	opts *tracker.QueueGetOptions,
) (*tracker.Queue, *tracker.Response, error) {
	return c.client.Queues.Get(ctx, key, opts)
}

func (c trackerContextClient) GetWorkflow(
	ctx context.Context,
	id string,
) (*tracker.Workflow, *tracker.Response, error) {
	return c.client.Workflows.Get(ctx, id)
}

func (c trackerContextClient) ListComponents(
	ctx context.Context,
	key string,
	opts *tracker.QueueComponentsListOptions,
) ([]*tracker.Component, *tracker.Response, error) {
	return c.client.Queues.ListComponents(ctx, key, opts)
}

func (c trackerContextClient) ListQueueFields(
	ctx context.Context,
	key string,
) ([]*tracker.Field, *tracker.Response, error) {
	return c.client.Queues.ListFields(ctx, key)
}

func (c trackerContextClient) ListLocalFields(
	ctx context.Context,
	key string,
) ([]*tracker.Field, *tracker.Response, error) {
	return c.client.Fields.ListLocal(ctx, key)
}

func (c trackerContextClient) ListGlobalFields(ctx context.Context) ([]*tracker.Field, *tracker.Response, error) {
	return c.client.Fields.List(ctx)
}

// newLister creates a queueLister from resolved auth. Replaceable for testing.
var newLister = func(auth *config.ResolvedAuth) queueLister {
	return api.NewClient(auth).Queues
}

// newGetter creates a queueGetter from resolved auth. Replaceable for testing.
var newGetter = func(auth *config.ResolvedAuth) queueGetter {
	return api.NewClient(auth).Queues
}

// newContextClient creates a queueContextClient from resolved auth. Replaceable for testing.
var newContextClient = func(auth *config.ResolvedAuth) queueContextClient {
	return trackerContextClient{client: api.NewClient(auth)}
}

// NewCmd creates the parent "queue" command with list, view, and context subcommands.
func NewCmd() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "queue",
		Short: "Manage queues",
		Long: "List and view Yandex Tracker queues, and show a queue's context: what an agent needs " +
			"to create and move issues in it.",
	}

	cmd.AddCommand(newListCmd())
	cmd.AddCommand(newViewCmd())
	cmd.AddCommand(newContextCmd())

	return cmd
}
