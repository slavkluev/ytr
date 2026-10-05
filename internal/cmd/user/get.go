package user

import (
	"context"

	"github.com/slavkluev/go-yandex-tracker/tracker"
	"github.com/spf13/cobra"

	"github.com/slavkluev/ytr/internal/cmd/runner"
)

func newGetCmd() *cobra.Command {
	return runner.Get[*tracker.User, userDetail]{
		Use:   "get UID",
		Short: "Show user details",
		Long:  `Display detailed information about a Yandex Tracker user by UID.`,
		Example: `  # Show user details by UID
  ytr user get 12345

  # Only the UID, name, login and email
  ytr user get 12345 --json uid,display,login,email

  # Get just the login
  ytr user get 12345 --json login --jq '.login'`,
		Args: []runner.Arg{runner.StringID("user ID")},
		Call: func(ctx context.Context, c *tracker.Client, args []string) (*tracker.User, error) {
			user, _, err := c.Users.Get(ctx, args[0])
			return user, err
		},
		Item: toUserDetail,
	}.Command()
}
