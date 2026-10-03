// Package checklist provides checklist management commands for the ytr CLI.
package checklist

import (
	"context"

	"github.com/slavkluev/go-yandex-tracker/tracker"
	"github.com/spf13/cobra"

	"github.com/slavkluev/ytr/internal/api"
	"github.com/slavkluev/ytr/internal/config"
)

type checklistLister interface {
	ListChecklistItems(
		ctx context.Context,
		issueKey string,
	) ([]*tracker.ChecklistItem, *tracker.Response, error)
}

// CreateChecklistItem returns *tracker.Issue (not *ChecklistItem),
// requiring item extraction from Issue.ChecklistItems.
type checklistCreator interface {
	CreateChecklistItem(
		ctx context.Context,
		issueKey string,
		item *tracker.ChecklistItemRequest,
	) (*tracker.Issue, *tracker.Response, error)
}

// EditChecklistItem returns *tracker.Issue, requiring item extraction by ID.
type checklistEditor interface {
	EditChecklistItem(
		ctx context.Context,
		issueKey, itemID string,
		item *tracker.ChecklistItemRequest,
	) (*tracker.Issue, *tracker.Response, error)
}

// DeleteChecklistItem returns *tracker.Issue (HTTP 200, not 204).
type checklistDeleter interface {
	DeleteChecklistItem(
		ctx context.Context,
		issueKey, itemID string,
	) (*tracker.Issue, *tracker.Response, error)
}

var newChecklistLister = func(auth *config.ResolvedAuth) checklistLister {
	return api.NewClient(auth).Issues
}

var newChecklistCreator = func(auth *config.ResolvedAuth) checklistCreator {
	return api.NewClient(auth).Issues
}

var newChecklistEditor = func(auth *config.ResolvedAuth) checklistEditor {
	return api.NewClient(auth).Issues
}

var newChecklistDeleter = func(auth *config.ResolvedAuth) checklistDeleter {
	return api.NewClient(auth).Issues
}

// NewCmd creates the parent "checklist" command with subcommands.
func NewCmd() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "checklist",
		Short: "Manage issue checklists",
		Long:  "List, create, edit, and delete checklist items on Yandex Tracker issues.",
	}

	cmd.AddCommand(newListCmd())
	cmd.AddCommand(newCreateCmd())
	cmd.AddCommand(newEditCmd())
	cmd.AddCommand(newDeleteCmd())

	return cmd
}
