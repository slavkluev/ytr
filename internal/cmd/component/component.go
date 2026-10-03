// Package component provides component management commands for the ytr CLI.
package component

import (
	"context"

	"github.com/slavkluev/go-yandex-tracker/tracker"
	"github.com/spf13/cobra"

	"github.com/slavkluev/ytr/internal/api"
	"github.com/slavkluev/ytr/internal/config"
)

type componentCreator interface {
	Create(ctx context.Context, component *tracker.ComponentRequest) (*tracker.Component, *tracker.Response, error)
}

type componentEditor interface {
	Edit(
		ctx context.Context,
		componentID string,
		component *tracker.ComponentRequest,
	) (*tracker.Component, *tracker.Response, error)
}

type componentDeleter interface {
	Delete(ctx context.Context, componentID string) (*tracker.Response, error)
}

var newComponentCreator = func(auth *config.ResolvedAuth) componentCreator {
	return api.NewClient(auth).Components
}

var newComponentEditor = func(auth *config.ResolvedAuth) componentEditor {
	return api.NewClient(auth).Components
}

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
