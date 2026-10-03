// Package field provides field discovery commands for the ytr CLI.
package field

import (
	"github.com/spf13/cobra"
)

// NewCmd creates the parent "field" command with subcommands.
func NewCmd() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "field",
		Short: "Discover issue fields",
		Long:  "List and inspect Yandex Tracker fields including custom field schemas and allowed values.",
	}

	cmd.AddCommand(newListCmd())
	cmd.AddCommand(newGetCmd())

	return cmd
}
