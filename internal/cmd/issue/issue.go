// Package issue provides issue management commands for the ytr CLI.
package issue

import (
	"context"

	"github.com/slavkluev/go-yandex-tracker/tracker"
	"github.com/spf13/cobra"

	"github.com/slavkluev/ytr/internal/api"
	"github.com/slavkluev/ytr/internal/config"
)

type issueSearcher interface {
	Search(
		ctx context.Context,
		req *tracker.IssueSearchRequest,
		opts *tracker.IssueSearchOptions,
	) ([]*tracker.Issue, *tracker.Response, error)
}

type issueCreator interface {
	Create(ctx context.Context, issue *tracker.IssueRequest) (*tracker.Issue, *tracker.Response, error)
}

type issueEditor interface {
	Edit(
		ctx context.Context,
		issueKey string,
		req *tracker.IssueRequest,
		opts *tracker.IssueEditOptions,
	) (*tracker.Issue, *tracker.Response, error)
}

type issueTransitioner interface {
	GetTransitions(ctx context.Context, issueKey string) ([]*tracker.Transition, *tracker.Response, error)
	ExecuteTransition(
		ctx context.Context,
		issueKey string,
		transitionID string,
		req *tracker.TransitionRequest,
	) ([]*tracker.Transition, *tracker.Response, error)
}

var newSearcher = func(auth *config.ResolvedAuth) issueSearcher {
	return api.NewClient(auth).Issues
}

var newCreator = func(auth *config.ResolvedAuth) issueCreator {
	return api.NewClient(auth).Issues
}

var newEditor = func(auth *config.ResolvedAuth) issueEditor {
	return api.NewClient(auth).Issues
}

var newTransitioner = func(auth *config.ResolvedAuth) issueTransitioner {
	return api.NewClient(auth).Issues
}

type changelogGetter interface {
	GetChangelog(
		ctx context.Context,
		issueKey string,
		opts *tracker.ChangelogOptions,
	) ([]*tracker.Changelog, *tracker.Response, error)
}

var newChangelogGetter = func(auth *config.ResolvedAuth) changelogGetter {
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
	cmd.AddCommand(newCreateCmd())
	cmd.AddCommand(newUpdateCmd())
	cmd.AddCommand(newTransitionCmd())
	cmd.AddCommand(newChangelogCmd())

	return cmd
}
