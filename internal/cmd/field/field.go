// Package field provides field discovery commands for the ytr CLI.
package field

import (
	"context"

	"github.com/slavkluev/go-yandex-tracker/tracker"
	"github.com/spf13/cobra"

	"github.com/slavkluev/ytr/internal/api"
	"github.com/slavkluev/ytr/internal/config"
)

// fieldLister abstracts field list operations (global and local) for testability.
type fieldLister interface {
	List(ctx context.Context) ([]*tracker.Field, *tracker.Response, error)
	ListLocal(ctx context.Context, queueKey string) ([]*tracker.Field, *tracker.Response, error)
}

// fieldGetter abstracts field get operations (global and local) for testability.
type fieldGetter interface {
	Get(ctx context.Context, fieldID string) (*tracker.Field, *tracker.Response, error)
	GetLocal(ctx context.Context, queueKey, fieldKey string) (*tracker.Field, *tracker.Response, error)
}

// newFieldLister creates a fieldLister from resolved auth. Replaceable for testing.
var newFieldLister = func(auth *config.ResolvedAuth) fieldLister {
	return api.NewClient(auth).Fields
}

// newFieldGetter creates a fieldGetter from resolved auth. Replaceable for testing.
var newFieldGetter = func(auth *config.ResolvedAuth) fieldGetter {
	return api.NewClient(auth).Fields
}

// NewCmd creates the parent "field" command with subcommands.
func NewCmd() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "field",
		Short: "Discover issue fields",
		Long:  "List and inspect Yandex Tracker fields including custom field schemas and allowed values.",
	}

	cmd.AddCommand(newListCmd())
	cmd.AddCommand(newGetCmd())

	return cmd
}
