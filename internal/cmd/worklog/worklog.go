// Package worklog provides worklog management commands for the ytr CLI.
package worklog

import (
	"context"

	"github.com/slavkluev/go-yandex-tracker/tracker"
	"github.com/spf13/cobra"

	"github.com/slavkluev/ytr/internal/api"
	"github.com/slavkluev/ytr/internal/config"
)

// worklogLister abstracts worklog list operations for testability.
type worklogLister interface {
	ListWorklogs(
		ctx context.Context,
		issueKey string,
	) ([]*tracker.Worklog, *tracker.Response, error)
}

// worklogCreator abstracts worklog creation for testability.
type worklogCreator interface {
	CreateWorklog(
		ctx context.Context,
		issueKey string,
		worklog *tracker.WorklogRequest,
	) (*tracker.Worklog, *tracker.Response, error)
}

// worklogEditor abstracts worklog edit operations for testability.
type worklogEditor interface {
	EditWorklog(
		ctx context.Context,
		issueKey, worklogID string,
		worklog *tracker.WorklogRequest,
	) (*tracker.Worklog, *tracker.Response, error)
}

// worklogDeleter abstracts worklog deletion for testability.
type worklogDeleter interface {
	DeleteWorklog(
		ctx context.Context,
		issueKey, worklogID string,
	) (*tracker.Response, error)
}

// newWorklogLister creates a worklogLister from resolved auth. Replaceable for testing.
var newWorklogLister = func(auth *config.ResolvedAuth) worklogLister {
	return api.NewClient(auth).Issues
}

// newWorklogCreator creates a worklogCreator from resolved auth. Replaceable for testing.
var newWorklogCreator = func(auth *config.ResolvedAuth) worklogCreator {
	return api.NewClient(auth).Issues
}

// newWorklogEditor creates a worklogEditor from resolved auth. Replaceable for testing.
var newWorklogEditor = func(auth *config.ResolvedAuth) worklogEditor {
	return api.NewClient(auth).Issues
}

// newWorklogDeleter creates a worklogDeleter from resolved auth. Replaceable for testing.
var newWorklogDeleter = func(auth *config.ResolvedAuth) worklogDeleter {
	return api.NewClient(auth).Issues
}

// NewCmd creates the parent "worklog" command with subcommands.
func NewCmd() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "worklog",
		Short: "Manage issue worklogs",
		Long:  "List, create, edit, and delete worklogs on Yandex Tracker issues.",
	}

	cmd.AddCommand(newListCmd())
	cmd.AddCommand(newCreateCmd())
	cmd.AddCommand(newEditCmd())
	cmd.AddCommand(newDeleteCmd())

	return cmd
}
