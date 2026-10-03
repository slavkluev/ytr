// Package status provides status commands for the ytr CLI.
package status

import "github.com/spf13/cobra"

// NewCmd creates the parent "status" command.
func NewCmd() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "status",
		Short: "Manage workflow statuses",
		Long:  "View workflow statuses in Yandex Tracker.",
	}
	cmd.AddCommand(newListCmd())
	return cmd
}
