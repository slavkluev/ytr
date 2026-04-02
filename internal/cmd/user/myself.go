package user

import (
	"fmt"
	"io"
	"strconv"

	"github.com/jedib0t/go-pretty/v6/text"
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

// userDetail is a clean struct for JSON serialization of full user info.
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

// toUserDetail converts a tracker.User to a clean JSON-serializable struct.
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

// newMyselfCmd creates the "user myself" command for displaying current user info.
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

// runMyself executes the user myself logic.
func runMyself(cmd *cobra.Command) error {
	// Handle --json field hint (no fields specified, D-10).
	if output.IsJSON() && !output.HasFieldSelection() && output.JQFilter == "" {
		return output.PrintFieldHint(cmd.ErrOrStderr(), "user myself", UserDetailFields)
	}

	// If --jq without --json: auto-populate all fields (Pitfall 5).
	if output.JQFilter != "" && !output.HasFieldSelection() {
		output.JSONFields = UserDetailFields
	}

	// Validate requested fields.
	if output.HasFieldSelection() {
		if err := output.ValidateFields(output.JSONFields, UserDetailFields); err != nil {
			return err
		}
		output.JSONFields = output.NormalizeFields(output.JSONFields, UserDetailFields)
	}

	// Resolve auth from root persistent flags.
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

	return renderDetailOutput(cmd.OutOrStdout(), user)
}

// renderDetailOutput handles JSON/quiet/table output for a user detail result.
func renderDetailOutput(w io.Writer, user *tracker.User) error {
	if output.IsJSON() {
		detail := toUserDetail(user)

		if output.HasFieldSelection() {
			filtered := output.FilterFields(detail, output.JSONFields)
			if output.JQFilter != "" {
				return output.ApplyJQ(w, filtered, output.JQFilter)
			}
			return output.PrintJSON(w, filtered)
		}
		if output.JQFilter != "" {
			return output.ApplyJQ(w, detail, output.JQFilter)
		}
		return output.PrintJSON(w, detail)
	}

	if output.IsQuiet() {
		output.PrintQuiet(w, strconv.Itoa(api.DerefInt(user.UID, 0)))
		return nil
	}

	// Table-style view: labeled rows similar to gh issue view.
	bold := func(label string) string {
		if output.ColorsEnabled() {
			return text.Colors{text.Bold}.Sprint(label)
		}
		return label
	}

	// writeErr captures the first write error encountered.
	var writeErr error
	printField := func(label, value string) {
		if writeErr != nil {
			return
		}
		_, writeErr = fmt.Fprintf(w, "%s  %s\n", bold(label+":"), value)
	}

	printField("UID", strconv.Itoa(api.DerefInt(user.UID, 0)))
	printField("Display", api.DerefString(user.Display, "-"))
	printField("Login", api.DerefString(user.Login, "-"))
	printField("Email", api.DerefString(user.Email, "-"))
	printField("First Name", api.DerefString(user.FirstName, "-"))
	printField("Last Name", api.DerefString(user.LastName, "-"))
	printField("Dismissed", strconv.FormatBool(api.DerefBool(user.Dismissed, false)))
	printField("Has License", strconv.FormatBool(api.DerefBool(user.HasLicense, false)))
	printField("External", strconv.FormatBool(api.DerefBool(user.External, false)))

	return writeErr
}
