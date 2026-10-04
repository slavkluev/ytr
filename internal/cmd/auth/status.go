package auth

import (
	"fmt"

	"github.com/spf13/cobra"

	"github.com/slavkluev/ytr/internal/api"
	"github.com/slavkluev/ytr/internal/cmd/runner"
	"github.com/slavkluev/ytr/internal/config"
	"github.com/slavkluev/ytr/internal/output"
)

type statusItem struct {
	Status      string `json:"status"`
	User        string `json:"user"`
	OrgID       string `json:"org_id"`
	OrgType     string `json:"org_type"`
	TokenSource string `json:"token_source"`
}

var statusFields = runner.ItemFields[statusItem]()

func newStatusCmd() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "status",
		Short: "Show authentication status",
		Long: `Show the current authentication state. Validates the token via API call and displays
the token source, organization, and authenticated user.

JSON FIELDS
  status, user, org_id, org_type, token_source`,
		Example: `  # Check auth status
  ytr auth status

  # Check auth status as JSON
  ytr auth status --jq .

  # Get the authenticated user's name
  ytr auth status --json user --jq .user`,
		Args: cobra.NoArgs,
		RunE: runStatus,
	}

	runner.SetFields(cmd, statusFields)

	return cmd
}

func runStatus(cmd *cobra.Command, _ []string) error {
	opts, err := runner.SelectFields(cmd, statusFields)
	if err != nil {
		return err
	}

	tokenFlag := ""
	orgIDFlag := ""
	orgTypeFlag := ""
	if root := cmd.Root(); root != nil {
		tokenFlag, _ = root.PersistentFlags().GetString("token")
		orgIDFlag, _ = root.PersistentFlags().GetString("org-id")
		orgTypeFlag, _ = root.PersistentFlags().GetString("org-type")
	}

	auth, err := config.ResolveAuth(cmd.Context(), tokenFlag, orgIDFlag, orgTypeFlag)
	if err != nil {
		return err
	}

	user, _, err := api.NewClient(auth).Users.Myself(cmd.Context())
	if err != nil {
		return api.MapAPIError(err)
	}

	item := statusItem{
		Status:      "authenticated",
		User:        user.DisplayOr("unknown"),
		OrgID:       auth.OrgID,
		OrgType:     string(auth.OrgType),
		TokenSource: auth.TokenSource,
	}

	if opts.IsJSON() {
		return runner.PrintJSON(cmd, opts, output.FilterFields(item, opts.JSONFields))
	}

	_, _ = fmt.Fprintf(cmd.ErrOrStderr(),
		"Authenticated as %s\n  Token source: %s\n  Organization: %s\n  Organization type: %s\n",
		item.User, item.TokenSource, item.OrgID, item.OrgType)
	return nil
}
