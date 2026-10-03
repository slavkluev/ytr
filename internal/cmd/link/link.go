// Package link provides link management commands for the ytr CLI.
package link

import (
	"github.com/spf13/cobra"
)

// NewCmd creates the parent "link" command with subcommands.
func NewCmd() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "link",
		Short: "Manage issue links",
		Long:  "List, create, and delete links between Yandex Tracker issues.",
	}

	cmd.AddCommand(newListCmd())
	cmd.AddCommand(newCreateCmd())
	cmd.AddCommand(newDeleteCmd())

	return cmd
}
