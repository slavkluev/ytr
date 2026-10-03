// Package resolution provides resolution commands for the ytr CLI.
package resolution

import "github.com/spf13/cobra"

// NewCmd creates the parent "resolution" command.
func NewCmd() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "resolution",
		Short: "Manage resolutions",
		Long:  "View resolutions in Yandex Tracker.",
	}
	cmd.AddCommand(newListCmd())
	return cmd
}
