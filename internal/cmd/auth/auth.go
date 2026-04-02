// Package auth provides authentication commands for the ytr CLI.
// It includes login, status, and logout subcommands under "ytr auth".
package auth

import (
	"github.com/spf13/cobra"
)

// NewCmd creates the parent "auth" command with login, status, and
// logout subcommands. Running "ytr auth" alone displays help.
func NewCmd() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "auth",
		Short: "Manage authentication",
		Long:  "Authenticate with Yandex Tracker. Login, check status, or logout.",
	}

	cmd.AddCommand(newLoginCmd())
	cmd.AddCommand(newStatusCmd())
	cmd.AddCommand(newLogoutCmd())

	return cmd
}
