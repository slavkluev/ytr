// Package user provides user management commands for the ytr CLI.
package user

import (
	"context"

	"github.com/slavkluev/go-yandex-tracker/tracker"
	"github.com/spf13/cobra"

	"github.com/slavkluev/ytr/internal/api"
	"github.com/slavkluev/ytr/internal/config"
)

// userMyself abstracts the current user lookup for testability.
type userMyself interface {
	Myself(ctx context.Context) (*tracker.User, *tracker.Response, error)
}

// userGetter abstracts user lookup by ID for testability.
type userGetter interface {
	Get(ctx context.Context, userID string) (*tracker.User, *tracker.Response, error)
}

// userLister abstracts user list operations for testability.
type userLister interface {
	List(ctx context.Context, opts *tracker.UserListOptions) ([]*tracker.User, *tracker.Response, error)
}

// newUserMyself creates a userMyself from resolved auth. Replaceable for testing.
var newUserMyself = func(auth *config.ResolvedAuth) userMyself {
	return api.NewClient(auth).Users
}

// newUserGetter creates a userGetter from resolved auth. Replaceable for testing.
var newUserGetter = func(auth *config.ResolvedAuth) userGetter {
	return api.NewClient(auth).Users
}

// newUserLister creates a userLister from resolved auth. Replaceable for testing.
var newUserLister = func(auth *config.ResolvedAuth) userLister {
	return api.NewClient(auth).Users
}

// NewCmd creates the parent "user" command.
func NewCmd() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "user",
		Short: "Manage users",
		Long:  "View user information in Yandex Tracker.",
	}

	cmd.AddCommand(newMyselfCmd())
	cmd.AddCommand(newGetCmd())
	cmd.AddCommand(newListCmd())

	return cmd
}
