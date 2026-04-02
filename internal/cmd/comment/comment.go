// Package comment provides comment management commands for the ytr CLI.
package comment

import (
	"context"

	"github.com/slavkluev/go-yandex-tracker/tracker"
	"github.com/spf13/cobra"

	"github.com/slavkluev/ytr/internal/api"
	"github.com/slavkluev/ytr/internal/config"
)

// commentLister abstracts comment list operations for testability.
type commentLister interface {
	ListComments(
		ctx context.Context,
		issueKey string,
		opts *tracker.CommentListOptions,
	) ([]*tracker.Comment, *tracker.Response, error)
}

// commentCreator abstracts comment creation for testability.
type commentCreator interface {
	CreateComment(
		ctx context.Context,
		issueKey string,
		req *tracker.CommentRequest,
	) (*tracker.Comment, *tracker.Response, error)
}

// newCommentLister creates a commentLister from resolved auth. Replaceable for testing.
var newCommentLister = func(auth *config.ResolvedAuth) commentLister {
	return api.NewClient(auth).Issues
}

// newCommentCreator creates a commentCreator from resolved auth. Replaceable for testing.
var newCommentCreator = func(auth *config.ResolvedAuth) commentCreator {
	return api.NewClient(auth).Issues
}

// commentEditor abstracts comment edit operations for testability.
type commentEditor interface {
	EditComment(
		ctx context.Context,
		issueKey string,
		commentID string,
		req *tracker.CommentRequest,
	) (*tracker.Comment, *tracker.Response, error)
}

// commentDeleter abstracts comment deletion for testability.
type commentDeleter interface {
	DeleteComment(
		ctx context.Context,
		issueKey string,
		commentID string,
	) (*tracker.Response, error)
}

// newCommentEditor creates a commentEditor from resolved auth. Replaceable for testing.
var newCommentEditor = func(auth *config.ResolvedAuth) commentEditor {
	return api.NewClient(auth).Issues
}

// newCommentDeleter creates a commentDeleter from resolved auth. Replaceable for testing.
var newCommentDeleter = func(auth *config.ResolvedAuth) commentDeleter {
	return api.NewClient(auth).Issues
}

// NewCmd creates the parent "comment" command with subcommands.
func NewCmd() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "comment",
		Short: "Manage issue comments",
		Long:  "List, create, edit, and delete comments on Yandex Tracker issues.",
	}

	cmd.AddCommand(newListCmd())
	cmd.AddCommand(newCreateCmd())
	cmd.AddCommand(newEditCmd())
	cmd.AddCommand(newDeleteCmd())

	return cmd
}
