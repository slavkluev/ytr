// Package user provides user management commands for the ytr CLI.
package user

import (
	"context"

	"github.com/slavkluev/go-yandex-tracker/tracker"
	"github.com/spf13/cobra"

	"github.com/slavkluev/ytr/internal/api"
	"github.com/slavkluev/ytr/internal/config"
)

type userMyself interface {
	Myself(ctx context.Context) (*tracker.User, *tracker.Response, error)
}

type userGetter interface {
	Get(ctx context.Context, userID string) (*tracker.User, *tracker.Response, error)
}

type userLister interface {
	List(ctx context.Context, opts *tracker.UserListOptions) ([]*tracker.User, *tracker.Response, error)
}

var newUserMyself = func(auth *config.ResolvedAuth) userMyself {
	return api.NewClient(auth).Users
}

var newUserGetter = func(auth *config.ResolvedAuth) userGetter {
	return api.NewClient(auth).Users
}

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
