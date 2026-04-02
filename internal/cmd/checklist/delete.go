package checklist

import (
	"fmt"

	"github.com/spf13/cobra"

	"github.com/slavkluev/ytr/internal/api"
	"github.com/slavkluev/ytr/internal/config"
	"github.com/slavkluev/ytr/internal/output"
	"github.com/slavkluev/ytr/internal/validate"
)

// newDeleteCmd creates the "checklist delete" command.
func newDeleteCmd() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "delete ISSUE-KEY ITEM-ID",
		Short: "Delete a checklist item",
		Long: `Delete a checklist item from a Yandex Tracker issue.

SEE ALSO
  ytr checklist list    - List checklist items on issue
  ytr checklist create  - Add checklist item to issue
  ytr checklist edit    - Edit a checklist item`,
		Example: `  # Delete checklist item
  ytr checklist delete PROJ-123 item-1

  # Delete and confirm via JSON
  ytr checklist delete PROJ-123 item-1 --json id`,
		Args: cobra.ExactArgs(2),
		PreRunE: func(cmd *cobra.Command, args []string) error {
			if err := validate.ValidateIssueKey(args[0]); err != nil {
				return err
			}
			_, err := validate.ValidateStringID(args[1], "checklist item ID")
			return err
		},
		RunE: func(cmd *cobra.Command, args []string) error {
			itemID, _ := validate.ValidateStringID(args[1], "checklist item ID")
			return runDelete(cmd, args[0], itemID)
		},
	}

	return cmd
}

// runDelete executes the checklist delete logic.
func runDelete(cmd *cobra.Command, issueKey, itemID string) error {
	// Resolve auth from root persistent flags.
	tokenFlag, _ := cmd.Root().PersistentFlags().GetString("token")
	orgIDFlag, _ := cmd.Root().PersistentFlags().GetString("org-id")
	orgTypeFlag, _ := cmd.Root().PersistentFlags().GetString("org-type")

	auth, err := config.ResolveAuth(tokenFlag, orgIDFlag, orgTypeFlag)
	if err != nil {
		return err
	}

	deleter := newChecklistDeleter(auth)

	// API returns (*Issue, *Response, error) but per D-07 we ignore the
	// returned *Issue and use the itemID from args for the confirmation.
	_, _, err = deleter.DeleteChecklistItem(cmd.Context(), issueKey, itemID)
	if err != nil {
		return api.MapAPIError(err)
	}

	w := cmd.OutOrStdout()

	if output.IsJSON() {
		result := map[string]any{"id": itemID, "deleted": true}
		if output.JQFilter != "" {
			return output.ApplyJQ(w, result, output.JQFilter)
		}
		return output.PrintJSON(w, result)
	}

	if output.IsQuiet() {
		output.PrintQuiet(w, itemID)
		return nil
	}

	// Table output: brief confirmation.
	_, err = fmt.Fprintf(w, "Checklist item %s deleted\n", itemID)
	return err
}
