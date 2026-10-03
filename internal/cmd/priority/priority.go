// Package priority provides priority commands for the ytr CLI.
package priority

import "github.com/spf13/cobra"

// NewCmd creates the parent "priority" command.
func NewCmd() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "priority",
		Short: "Manage priorities",
		Long:  "View priorities in Yandex Tracker.",
	}
	cmd.AddCommand(newListCmd())
	return cmd
}
