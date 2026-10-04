// Package issue provides issue management commands for the ytr CLI.
package issue

import (
	"github.com/spf13/cobra"
)

// NewCmd creates the parent "issue" command with list and view subcommands.
func NewCmd() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "issue",
		Short: "Manage issues",
		Long:  "Create, view, edit, and search Yandex Tracker issues.",
	}

	cmd.AddCommand(newListCmd())
	cmd.AddCommand(newViewCmd())
	cmd.AddCommand(newCreateCmd())
	cmd.AddCommand(newUpdateCmd())
	cmd.AddCommand(newTransitionCmd())
	cmd.AddCommand(newChangelogCmd())

	return cmd
}
