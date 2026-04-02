// Package link provides link management commands for the ytr CLI.
package link

import (
	"context"

	"github.com/slavkluev/go-yandex-tracker/tracker"
	"github.com/spf13/cobra"

	"github.com/slavkluev/ytr/internal/api"
	"github.com/slavkluev/ytr/internal/config"
)

// linkLister abstracts link list operations for testability.
type linkLister interface {
	GetLinks(
		ctx context.Context,
		issueKey string,
	) ([]*tracker.IssueLink, *tracker.Response, error)
}

// linkCreator abstracts link creation for testability.
type linkCreator interface {
	CreateLink(
		ctx context.Context,
		issueKey string,
		link *tracker.LinkRequest,
	) (*tracker.IssueLink, *tracker.Response, error)
}

// linkDeleter abstracts link deletion for testability.
type linkDeleter interface {
	DeleteLink(
		ctx context.Context,
		issueKey, linkID string,
	) (*tracker.Response, error)
}

// newLinkLister creates a linkLister from resolved auth. Replaceable for testing.
var newLinkLister = func(auth *config.ResolvedAuth) linkLister {
	return api.NewClient(auth).Issues
}

// newLinkCreator creates a linkCreator from resolved auth. Replaceable for testing.
var newLinkCreator = func(auth *config.ResolvedAuth) linkCreator {
	return api.NewClient(auth).Issues
}

// newLinkDeleter creates a linkDeleter from resolved auth. Replaceable for testing.
var newLinkDeleter = func(auth *config.ResolvedAuth) linkDeleter {
	return api.NewClient(auth).Issues
}

// NewCmd creates the parent "link" command with subcommands.
func NewCmd() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "link",
		Short: "Manage issue links",
		Long:  "List, create, and delete links between Yandex Tracker issues.",
	}

	cmd.AddCommand(newListCmd())
	cmd.AddCommand(newCreateCmd())
	cmd.AddCommand(newDeleteCmd())

	return cmd
}
