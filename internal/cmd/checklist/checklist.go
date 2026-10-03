// Package checklist provides checklist management commands for the ytr CLI.
package checklist

import (
	"github.com/spf13/cobra"
)

// NewCmd creates the parent "checklist" command with subcommands.
func NewCmd() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "checklist",
		Short: "Manage issue checklists",
		Long:  "List, create, edit, and delete checklist items on Yandex Tracker issues.",
	}

	cmd.AddCommand(newListCmd())
	cmd.AddCommand(newCreateCmd())
	cmd.AddCommand(newEditCmd())
	cmd.AddCommand(newDeleteCmd())

	return cmd
}
