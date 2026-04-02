// Package issue provides issue management commands for the ytr CLI.
package issue

import (
	"context"

	"github.com/slavkluev/go-yandex-tracker/tracker"
	"github.com/spf13/cobra"

	"github.com/slavkluev/ytr/internal/api"
	"github.com/slavkluev/ytr/internal/config"
)

// issueSearcher abstracts issue search operations for testability.
type issueSearcher interface {
	Search(
		ctx context.Context,
		req *tracker.IssueSearchRequest,
		opts *tracker.IssueSearchOptions,
	) ([]*tracker.Issue, *tracker.Response, error)
}

// issueGetter abstracts single issue retrieval for testability.
type issueGetter interface {
	Get(ctx context.Context, key string, opts *tracker.IssueGetOptions) (*tracker.Issue, *tracker.Response, error)
}

// newSearcher creates an issueSearcher from resolved auth. Replaceable for testing.
var newSearcher = func(auth *config.ResolvedAuth) issueSearcher {
	return api.NewClient(auth).Issues
}

// newGetter creates an issueGetter from resolved auth. Replaceable for testing.
var newGetter = func(auth *config.ResolvedAuth) issueGetter {
	return api.NewClient(auth).Issues
}

// NewCmd creates the parent "issue" command with list and view subcommands.
func NewCmd() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "issue",
		Short: "Manage issues",
		Long:  "Create, view, edit, and search Yandex Tracker issues.",
	}

	cmd.AddCommand(newListCmd())
	cmd.AddCommand(newViewCmd())

	return cmd
}
