package auth

import (
	"fmt"

	"github.com/spf13/cobra"

	"github.com/slavkluev/ytr/internal/cmd/runner"
	"github.com/slavkluev/ytr/internal/config"
	"github.com/slavkluev/ytr/internal/output"
)

type logoutItem struct {
	Status     string `json:"status"`
	ConfigPath string `json:"config_path"`
}

var logoutFields = runner.ItemFields[logoutItem]()

func newLogoutCmd() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "logout",
		Short: "Remove stored credentials",
		Long: `Remove stored authentication credentials from the config file. The config file itself
is preserved; only the token, org_id, and org_type fields are cleared.

JSON FIELDS
  status, config_path`,
		Example: `  # Remove credentials
  ytr auth logout`,
		Args: cobra.NoArgs,
		RunE: runLogout,
	}

	runner.SetFields(cmd, logoutFields)

	return cmd
}

func runLogout(cmd *cobra.Command, _ []string) error {
	opts, err := runner.SelectFields(cmd, logoutFields)
	if err != nil {
		return err
	}

	// Load existing config; treat missing file as success (nothing to log out of).
	cfg, err := config.Load(cmd.Context())
	if err != nil {
		return fmt.Errorf("failed to load config: %w", err)
	}

	// Clear auth fields, preserving the file and any future non-auth fields.
	cfg.Token = ""
	cfg.OrgID = ""
	cfg.OrgType = ""

	if err := config.Save(cmd.Context(), cfg); err != nil {
		return fmt.Errorf("failed to save config: %w", err)
	}

	// Config was just saved successfully, so ConfigFilePath cannot fail.
	cfgPath, _ := config.ConfigFilePath(cmd.Context())

	item := logoutItem{Status: "logged_out", ConfigPath: cfgPath}

	return runner.PrintJSON(cmd, opts, output.FilterFields(item, opts.JSONFields))
}
