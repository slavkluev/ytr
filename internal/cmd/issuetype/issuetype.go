// Package issuetype provides issue type commands for the ytr CLI.
package issuetype

import "github.com/spf13/cobra"

// NewCmd creates the parent "issuetype" command.
func NewCmd() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "issuetype",
		Short: "Manage issue types",
		Long:  "View issue types in Yandex Tracker.",
	}
	cmd.AddCommand(newListCmd())
	return cmd
}
