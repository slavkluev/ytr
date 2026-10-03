// Package component provides component management commands for the ytr CLI.
package component

import (
	"github.com/spf13/cobra"
)

// NewCmd creates the parent "component" command with subcommands.
func NewCmd() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "component",
		Short: "Manage project components",
		Long:  "List, create, edit, and delete project components in Yandex Tracker.",
	}

	cmd.AddCommand(newListCmd())
	cmd.AddCommand(newGetCmd())
	cmd.AddCommand(newCreateCmd())
	cmd.AddCommand(newEditCmd())
	cmd.AddCommand(newDeleteCmd())

	return cmd
}
