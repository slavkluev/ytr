package auth

import (
	"fmt"

	"github.com/spf13/cobra"

	"github.com/slavkluev/ytr/internal/api"
	"github.com/slavkluev/ytr/internal/config"
	"github.com/slavkluev/ytr/internal/output"
)

// newStatusCmd creates the "auth status" command that shows the current
// authentication state. Validates the token via API call and displays
// the token source, organization, and authenticated user.
// Exits with code 3 (auth_error) when not authenticated.
func newStatusCmd() *cobra.Command {
	return &cobra.Command{
		Use:   "status",
		Short: "Show authentication status",
		Long: `Show the current authentication state. Validates the token via API call and displays
the token source, organization, and authenticated user.

Use --json to get machine-readable output with fixed structure (no field selection).

SEE ALSO
  ytr auth login    - Authenticate with Yandex Tracker
  ytr auth logout   - Remove stored credentials`,
		Example: `  # Check auth status
  ytr auth status

  # Check auth status as JSON
  ytr auth status --json`,
		Args: cobra.NoArgs,
		RunE: runStatus,
	}
}

func runStatus(cmd *cobra.Command, args []string) error {
	// Resolve auth from global flags, env vars, or config file.
	// Global --token, --org-id, and --org-type flags are read from root command.
	tokenFlag := ""
	orgIDFlag := ""
	orgTypeFlag := ""
	if root := cmd.Root(); root != nil {
		tokenFlag, _ = root.PersistentFlags().GetString("token")
		orgIDFlag, _ = root.PersistentFlags().GetString("org-id")
		orgTypeFlag, _ = root.PersistentFlags().GetString("org-type")
	}

	auth, err := config.ResolveAuth(tokenFlag, orgIDFlag, orgTypeFlag)
	if err != nil {
		return err
	}

	// Validate credentials via API
	validator := newValidator(auth)
	user, _, err := validator.Myself(cmd.Context())
	if err != nil {
		return api.MapAPIError(err)
	}

	// Extract username
	username := api.DerefUser(user, "unknown")

	// Output result
	// Auth commands use cmd.Flags().Changed("json") for JSON detection.
	// No field selection or hints -- fixed-structure JSON.
	jsonRequested := cmd.Flags().Changed("json") || output.IsJSON()
	if jsonRequested {
		return output.PrintJSON(cmd.OutOrStdout(), map[string]string{
			"status":       "authenticated",
			"user":         username,
			"org_id":       auth.OrgID,
			"org_type":     string(auth.OrgType),
			"token_source": auth.TokenSource,
		})
	}

	_, _ = fmt.Fprintf(cmd.ErrOrStderr(),
		"Authenticated as %s\n  Token source: %s\n  Organization: %s\n  Organization type: %s\n",
		username, auth.TokenSource, auth.OrgID, auth.OrgType)
	return nil
}
