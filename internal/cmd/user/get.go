package user

import (
	"github.com/spf13/cobra"

	"github.com/slavkluev/ytr/internal/api"
	"github.com/slavkluev/ytr/internal/cmd/jsonfields"
	"github.com/slavkluev/ytr/internal/config"
	"github.com/slavkluev/ytr/internal/output"
)

// newGetCmd creates the "user get" command for displaying user details by UID.
func newGetCmd() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "get UID",
		Short: "Show user details",
		Long: `Display detailed information about a Yandex Tracker user by UID.

JSON FIELDS
  uid, display, login, email, firstName, lastName, dismissed, hasLicense, external

SEE ALSO
  ytr user myself   - Show current user
  ytr user list     - List organization users`,
		Example: `  # Show user details by UID
  ytr user get 12345

  # Get user as JSON
  ytr user get 12345 --json uid,display,login,email

  # Get just the login
  ytr user get 12345 --json login --jq '.login'`,
		Args: cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			return runGet(cmd, args[0])
		},
	}

	jsonfields.Register("ytr user get", UserDetailFields)

	return cmd
}

// runGet executes the user get logic.
func runGet(cmd *cobra.Command, userID string) error {
	if output.IsJSON() && !output.HasFieldSelection() && output.JQFilter == "" {
		return output.PrintFieldHint(cmd.ErrOrStderr(), "user get", UserDetailFields)
	}

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

	client := newUserGetter(auth)

	user, _, err := client.Get(cmd.Context(), userID)
	if err != nil {
		return api.MapAPIError(err)
	}

	return renderDetailOutput(cmd.OutOrStdout(), user)
}
