// Package user provides user management commands for the ytr CLI.
package user

import (
	"github.com/spf13/cobra"
)

// NewCmd creates the parent "user" command.
func NewCmd() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "user",
		Short: "Manage users",
		Long:  "View user information in Yandex Tracker.",
	}

	cmd.AddCommand(newMyselfCmd())
	cmd.AddCommand(newGetCmd())
	cmd.AddCommand(newListCmd())

	return cmd
}
