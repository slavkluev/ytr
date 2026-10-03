package user

import (
	"context"
	"strconv"

	"github.com/slavkluev/go-yandex-tracker/tracker"
	"github.com/spf13/cobra"

	"github.com/slavkluev/ytr/internal/api"
	"github.com/slavkluev/ytr/internal/cmd/runner"
	"github.com/slavkluev/ytr/internal/output"
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

  # Get current user as JSON
  ytr user myself --json uid,display,login,email

  # Get just the UID
  ytr user myself --quiet`,
		Call: func(ctx context.Context, c *tracker.Client, _ []string) (*tracker.User, error) {
			user, _, err := c.Users.Myself(ctx)
			return user, err
		},
		Item:   toUserDetail,
		Detail: userCard,
		Quiet:  userUID,
	}.Command()
}

func userUID(user *tracker.User) string {
	return strconv.Itoa(api.DerefInt(user.UID, 0))
}

func userCard(d *output.DetailPrinter, _ *output.Options, user *tracker.User) {
	d.Field("UID", userUID(user))
	d.Field("Display", api.DerefString(user.Display, "-"))
	d.Field("Login", api.DerefString(user.Login, "-"))
	d.Field("Email", api.DerefString(user.Email, "-"))
	d.Field("First Name", api.DerefString(user.FirstName, "-"))
	d.Field("Last Name", api.DerefString(user.LastName, "-"))
	d.Field("Dismissed", strconv.FormatBool(api.DerefBool(user.Dismissed, false)))
	d.Field("Has License", strconv.FormatBool(api.DerefBool(user.HasLicense, false)))
	d.Field("External", strconv.FormatBool(api.DerefBool(user.External, false)))
}
