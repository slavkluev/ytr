// Package component provides component management commands for the ytr CLI.
package component

import (
	"context"

	"github.com/slavkluev/go-yandex-tracker/tracker"
	"github.com/spf13/cobra"

	"github.com/slavkluev/ytr/internal/api"
	"github.com/slavkluev/ytr/internal/config"
)

// componentLister abstracts component list operations for testability.
type componentLister interface {
	List(ctx context.Context) ([]*tracker.Component, *tracker.Response, error)
}

// componentGetter abstracts component get operations for testability.
type componentGetter interface {
	Get(ctx context.Context, componentID string) (*tracker.Component, *tracker.Response, error)
}

// componentCreator abstracts component creation for testability.
type componentCreator interface {
	Create(ctx context.Context, component *tracker.ComponentRequest) (*tracker.Component, *tracker.Response, error)
}

// componentEditor abstracts component editing for testability.
type componentEditor interface {
	Edit(
		ctx context.Context,
		componentID string,
		component *tracker.ComponentRequest,
	) (*tracker.Component, *tracker.Response, error)
}

// componentDeleter abstracts component deletion for testability.
type componentDeleter interface {
	Delete(ctx context.Context, componentID string) (*tracker.Response, error)
}

// newComponentLister creates a componentLister from resolved auth. Replaceable for testing.
var newComponentLister = func(auth *config.ResolvedAuth) componentLister {
	return api.NewClient(auth).Components
}

// newComponentGetter creates a componentGetter from resolved auth. Replaceable for testing.
var newComponentGetter = func(auth *config.ResolvedAuth) componentGetter {
	return api.NewClient(auth).Components
}

// newComponentCreator creates a componentCreator from resolved auth. Replaceable for testing.
var newComponentCreator = func(auth *config.ResolvedAuth) componentCreator {
	return api.NewClient(auth).Components
}

// newComponentEditor creates a componentEditor from resolved auth. Replaceable for testing.
var newComponentEditor = func(auth *config.ResolvedAuth) componentEditor {
	return api.NewClient(auth).Components
}

// newComponentDeleter creates a componentDeleter from resolved auth. Replaceable for testing.
var newComponentDeleter = func(auth *config.ResolvedAuth) componentDeleter {
	return api.NewClient(auth).Components
}

// NewCmd creates the parent "component" command with subcommands.
func NewCmd() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "component",
		Short: "Manage project components",
		Long:  "List, create, edit, and delete project components in Yandex Tracker.",
	}

	cmd.AddCommand(newListCmd())
	cmd.AddCommand(newGetCmd())
	cmd.AddCommand(newCreateCmd())
	cmd.AddCommand(newEditCmd())
	cmd.AddCommand(newDeleteCmd())

	return cmd
}
