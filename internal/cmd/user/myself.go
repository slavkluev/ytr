package user

import (
	"context"

	"github.com/slavkluev/go-yandex-tracker/tracker"
	"github.com/spf13/cobra"

	"github.com/slavkluev/ytr/internal/api"
	"github.com/slavkluev/ytr/internal/cmd/runner"
)

type userDetail struct {
	UID        int    `json:"uid"`
	Display    string `json:"display"`
	Login      string `json:"login"`
	Email      string `json:"email,omitempty"`
	FirstName  string `json:"firstName,omitempty"`
	LastName   string `json:"lastName,omitempty"`
	Dismissed  bool   `json:"dismissed"`
	HasLicense bool   `json:"hasLicense"`
	External   bool   `json:"external"`
}

func toUserDetail(u *tracker.User) userDetail {
	return userDetail{
		UID:        api.DerefInt(u.UID, 0),
		Display:    api.DerefString(u.Display, ""),
		Login:      api.DerefString(u.Login, ""),
		Email:      api.DerefString(u.Email, ""),
		FirstName:  api.DerefString(u.FirstName, ""),
		LastName:   api.DerefString(u.LastName, ""),
		Dismissed:  api.DerefBool(u.Dismissed, false),
		HasLicense: api.DerefBool(u.HasLicense, false),
		External:   api.DerefBool(u.External, false),
	}
}

func newMyselfCmd() *cobra.Command {
	return runner.Get[*tracker.User, userDetail]{
		Use:   "myself",
		Short: "Show current user",
		Long:  `Display detailed information about the currently authenticated user.`,
		Example: `  # Show current user
  ytr user myself

  # Only the UID, name, login and email
  ytr user myself --json uid,display,login,email

  # Get just the UID
  ytr user myself --jq .uid`,
		Call: func(ctx context.Context, c *tracker.Client, _ []string) (*tracker.User, error) {
			user, _, err := c.Users.Myself(ctx)
			return user, err
		},
		Item: toUserDetail,
	}.Command()
}
