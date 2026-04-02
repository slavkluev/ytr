package worklog

import (
	"fmt"

	"github.com/spf13/cobra"

	"github.com/slavkluev/ytr/internal/api"
	"github.com/slavkluev/ytr/internal/config"
	"github.com/slavkluev/ytr/internal/output"
	"github.com/slavkluev/ytr/internal/validate"
)

// newDeleteCmd creates the "worklog delete" command.
func newDeleteCmd() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "delete ISSUE-KEY WORKLOG-ID",
		Short: "Delete a worklog",
		Long: `Delete a worklog from a Yandex Tracker issue.

SEE ALSO
  ytr worklog list    - List worklogs on issue
  ytr worklog create  - Create a worklog
  ytr worklog edit    - Edit a worklog`,
		Example: `  # Delete worklog abc123 from PROJ-123
  ytr worklog delete PROJ-123 abc123

  # Delete and confirm via JSON
  ytr worklog delete PROJ-123 abc123 --jq '.deleted'`,
		Args: cobra.ExactArgs(2),
		PreRunE: func(cmd *cobra.Command, args []string) error {
			if err := validate.ValidateIssueKey(args[0]); err != nil {
				return err
			}
			_, err := validate.ValidateStringID(args[1], "worklog ID")
			return err
		},
		RunE: func(cmd *cobra.Command, args []string) error {
			worklogID, _ := validate.ValidateStringID(args[1], "worklog ID")
			return runDelete(cmd, args[0], worklogID)
		},
	}

	return cmd
}

// runDelete executes the worklog delete logic.
func runDelete(cmd *cobra.Command, issueKey, worklogID string) error {
	// Resolve auth from root persistent flags.
	tokenFlag, _ := cmd.Root().PersistentFlags().GetString("token")
	orgIDFlag, _ := cmd.Root().PersistentFlags().GetString("org-id")
	orgTypeFlag, _ := cmd.Root().PersistentFlags().GetString("org-type")

	auth, err := config.ResolveAuth(tokenFlag, orgIDFlag, orgTypeFlag)
	if err != nil {
		return err
	}

	deleter := newWorklogDeleter(auth)

	// API returns (*Response, error) -- no body on 204 No Content.
	_, err = deleter.DeleteWorklog(cmd.Context(), issueKey, worklogID)
	if err != nil {
		return api.MapAPIError(err)
	}

	w := cmd.OutOrStdout()

	if output.IsJSON() {
		result := map[string]any{"id": worklogID, "deleted": true}
		if output.JQFilter != "" {
			return output.ApplyJQ(w, result, output.JQFilter)
		}
		return output.PrintJSON(w, result)
	}

	if output.IsQuiet() {
		output.PrintQuiet(w, worklogID)
		return nil
	}

	// Table output: brief confirmation.
	_, err = fmt.Fprintf(w, "Worklog %s deleted\n", worklogID)
	return err
}
