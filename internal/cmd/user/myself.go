package user

import (
	"io"
	"strconv"

	"github.com/slavkluev/go-yandex-tracker/tracker"
	"github.com/spf13/cobra"

	"github.com/slavkluev/ytr/internal/api"
	"github.com/slavkluev/ytr/internal/cmd/jsonfields"
	"github.com/slavkluev/ytr/internal/config"
	"github.com/slavkluev/ytr/internal/output"
)

// UserDetailFields lists the available JSON field names for user detail output.
var UserDetailFields = []string{
	"uid", "display", "login", "email",
	"firstName", "lastName", "dismissed", "hasLicense", "external",
}

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
	cmd := &cobra.Command{
		Use:   "myself",
		Short: "Show current user",
		Long: `Display detailed information about the currently authenticated user.

JSON FIELDS
  uid, display, login, email, firstName, lastName, dismissed, hasLicense, external

SEE ALSO
  ytr user get     - Show user details by UID
  ytr user list    - List organization users`,
		Example: `  # Show current user
  ytr user myself

  # Get current user as JSON
  ytr user myself --json uid,display,login,email

  # Get just the UID
  ytr user myself --quiet`,
		Args: cobra.NoArgs,
		RunE: func(cmd *cobra.Command, _ []string) error {
			return runMyself(cmd)
		},
	}

	jsonfields.Register("ytr user myself", UserDetailFields)

	return cmd
}

func runMyself(cmd *cobra.Command) error {
	opts := output.FromContext(cmd.Context())

	if opts.WantsFieldHint(cmd.Flags().Changed("json")) {
		return output.PrintFieldHint(cmd.ErrOrStderr(), "user myself", UserDetailFields)
	}

	if opts.JQFilter != "" && !opts.HasFieldSelection() {
		opts.JSONFields = UserDetailFields
	}

	if opts.HasFieldSelection() {
		if err := output.ValidateFields(opts.JSONFields, UserDetailFields); err != nil {
			return err
		}
		opts.JSONFields = output.NormalizeFields(opts.JSONFields, UserDetailFields)
	}

	tokenFlag, _ := cmd.Root().PersistentFlags().GetString("token")
	orgIDFlag, _ := cmd.Root().PersistentFlags().GetString("org-id")
	orgTypeFlag, _ := cmd.Root().PersistentFlags().GetString("org-type")

	auth, err := config.ResolveAuth(tokenFlag, orgIDFlag, orgTypeFlag)
	if err != nil {
		return err
	}

	client := newUserMyself(auth)

	user, _, err := client.Myself(cmd.Context())
	if err != nil {
		return api.MapAPIError(err)
	}

	return renderDetailOutput(cmd.OutOrStdout(), opts, user)
}

func renderDetailOutput(w io.Writer, opts *output.Options, user *tracker.User) error {
	if opts.IsJSON() {
		detail := toUserDetail(user)

		if opts.HasFieldSelection() {
			filtered := output.FilterFields(detail, opts.JSONFields)
			if opts.JQFilter != "" {
				return output.ApplyJQ(w, filtered, opts.JQFilter)
			}
			return opts.PrintJSON(w, filtered)
		}
		if opts.JQFilter != "" {
			return output.ApplyJQ(w, detail, opts.JQFilter)
		}
		return opts.PrintJSON(w, detail)
	}

	if opts.Quiet {
		output.PrintQuiet(w, strconv.Itoa(api.DerefInt(user.UID, 0)))
		return nil
	}

	d := opts.NewDetail(w)

	d.Field("UID", strconv.Itoa(api.DerefInt(user.UID, 0)))
	d.Field("Display", api.DerefString(user.Display, "-"))
	d.Field("Login", api.DerefString(user.Login, "-"))
	d.Field("Email", api.DerefString(user.Email, "-"))
	d.Field("First Name", api.DerefString(user.FirstName, "-"))
	d.Field("Last Name", api.DerefString(user.LastName, "-"))
	d.Field("Dismissed", strconv.FormatBool(api.DerefBool(user.Dismissed, false)))
	d.Field("Has License", strconv.FormatBool(api.DerefBool(user.HasLicense, false)))
	d.Field("External", strconv.FormatBool(api.DerefBool(user.External, false)))

	return d.Err()
}
