package user

import (
	"context"
	"iter"

	"github.com/slavkluev/go-yandex-tracker/tracker"
	"github.com/spf13/cobra"

	"github.com/slavkluev/ytr/internal/api"
	"github.com/slavkluev/ytr/internal/cmd/runner"
	"github.com/slavkluev/ytr/internal/output"
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

  # List users as JSON
  ytr user list --json uid,display,login,email

  # Get all user logins
  ytr user list --all --json login --jq '.items[].login'`,
		Empty: "No users found",
		Page: func(ctx context.Context, c *tracker.Client, o tracker.ListOptions) (
			[]*tracker.User, *tracker.Response, error,
		) {
			return c.Users.List(ctx, &tracker.UserListOptions{ListOptions: o})
		},
		All: func(ctx context.Context, c *tracker.Client, o tracker.ListOptions) iter.Seq2[*tracker.User, error] {
			return c.Users.ListIter(ctx, &tracker.UserListOptions{ListOptions: o})
		},
		Item:   toUserItem,
		Header: []string{"UID", "DISPLAY", "LOGIN", "EMAIL"},
		Row:    userRow,
		Quiet:  userUID,
	}.Command()
}

func userRow(_ *output.Options, u *tracker.User) []string {
	return []string{
		userUID(u),
		api.DerefString(u.Display, "-"),
		api.DerefString(u.Login, "-"),
		api.DerefString(u.Email, "-"),
	}
}
