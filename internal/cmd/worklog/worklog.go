// Package worklog provides worklog management commands for the ytr CLI.
package worklog

import (
	"github.com/spf13/cobra"
)

// NewCmd creates the parent "worklog" command with subcommands.
func NewCmd() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "worklog",
		Short: "Manage issue worklogs",
		Long:  "List, create, edit, and delete worklogs on Yandex Tracker issues.",
	}

	cmd.AddCommand(newListCmd())
	cmd.AddCommand(newCreateCmd())
	cmd.AddCommand(newEditCmd())
	cmd.AddCommand(newDeleteCmd())

	return cmd
}
