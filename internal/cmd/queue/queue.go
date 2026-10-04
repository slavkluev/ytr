// Package queue provides queue management commands for the ytr CLI.
package queue

import (
	"github.com/spf13/cobra"
)

// NewCmd creates the parent "queue" command with list, view, and context subcommands.
func NewCmd() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "queue",
		Short: "Manage queues",
		Long: "List and view Yandex Tracker queues, and show a queue's context: what an agent needs " +
			"to create and move issues in it.",
	}

	cmd.AddCommand(newListCmd())
	cmd.AddCommand(newViewCmd())
	cmd.AddCommand(newContextCmd())

	return cmd
}
