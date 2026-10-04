// Package comment provides comment management commands for the ytr CLI.
package comment

import (
	"github.com/spf13/cobra"
)

// NewCmd creates the parent "comment" command with subcommands.
func NewCmd() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "comment",
		Short: "Manage issue comments",
		Long:  "List, create, edit, and delete comments on Yandex Tracker issues.",
	}

	cmd.AddCommand(newListCmd())
	cmd.AddCommand(newCreateCmd())
	cmd.AddCommand(newEditCmd())
	cmd.AddCommand(newDeleteCmd())

	return cmd
}
