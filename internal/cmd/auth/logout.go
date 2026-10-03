package auth

import (
	"fmt"

	"github.com/spf13/cobra"

	"github.com/slavkluev/ytr/internal/config"
	"github.com/slavkluev/ytr/internal/output"
)

func newLogoutCmd() *cobra.Command {
	return &cobra.Command{
		Use:   "logout",
		Short: "Remove stored credentials",
		Long: `Remove stored authentication credentials from the config file. The config file itself
is preserved; only the token, org_id, and org_type fields are cleared.`,
		Example: `  # Remove credentials
  ytr auth logout`,
		Args: cobra.NoArgs,
		RunE: runLogout,
	}
}

func runLogout(cmd *cobra.Command, args []string) error {
	opts := output.FromContext(cmd.Context())

	// Load existing config; treat missing file as success (nothing to log out of).
	cfg, err := config.Load()
	if err != nil {
		return fmt.Errorf("failed to load config: %w", err)
	}

	// Clear auth fields, preserving the file and any future non-auth fields.
	cfg.Token = ""
	cfg.OrgID = ""
	cfg.OrgType = ""

	if err := config.Save(cfg); err != nil {
		return fmt.Errorf("failed to save config: %w", err)
	}

	// Config was just saved successfully, so ConfigFilePath cannot fail.
	cfgPath, _ := config.ConfigFilePath()

	// No field selection or hints -- fixed-structure JSON.
	jsonRequested := cmd.Flags().Changed("json") || opts.IsJSON()
	if jsonRequested {
		return opts.PrintJSON(cmd.OutOrStdout(), map[string]string{
			"status":      "logged_out",
			"config_path": cfgPath,
		})
	}

	_, _ = fmt.Fprintf(cmd.ErrOrStderr(),
		"Logged out. Credentials removed from %s\n",
		cfgPath)
	return nil
}
