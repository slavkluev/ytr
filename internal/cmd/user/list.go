package user

import (
	"context"
	"iter"

	"github.com/slavkluev/go-yandex-tracker/tracker"
	"github.com/spf13/cobra"

	"github.com/slavkluev/ytr/internal/api"
	"github.com/slavkluev/ytr/internal/cmd/runner"
)

type userItem struct {
	UID     int    `json:"uid"`
	Display string `json:"display"`
	Login   string `json:"login"`
	Email   string `json:"email,omitempty"`
}

func toUserItem(u *tracker.User) userItem {
	return userItem{
		UID:     api.DerefInt(u.UID, 0),
		Display: api.DerefString(u.Display, ""),
		Login:   api.DerefString(u.Login, ""),
		Email:   api.DerefString(u.Email, ""),
	}
}

func newListCmd() *cobra.Command {
	return runner.Pages[*tracker.User, userItem]{
		Use:   "list",
		Short: "List organization users",
		Long:  `List Yandex Tracker organization users with pagination.`,
		Example: `  # List all users
  ytr user list

  # Only UIDs, names, logins and emails
  ytr user list --json uid,display,login,email

  # Get all user logins
  ytr user list --all --json login --jq '.items[].login'`,
		Page: func(ctx context.Context, c *tracker.Client, o tracker.ListOptions) (
			[]*tracker.User, *tracker.Response, error,
		) {
			return c.Users.List(ctx, &tracker.UserListOptions{ListOptions: o})
		},
		All: func(ctx context.Context, c *tracker.Client, o tracker.ListOptions) iter.Seq2[*tracker.User, error] {
			return c.Users.ListIter(ctx, &tracker.UserListOptions{ListOptions: o})
		},
		Item: toUserItem,
	}.Command()
}
