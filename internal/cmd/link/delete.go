package link

import (
	"fmt"

	"github.com/spf13/cobra"

	"github.com/slavkluev/ytr/internal/api"
	"github.com/slavkluev/ytr/internal/config"
	"github.com/slavkluev/ytr/internal/output"
	"github.com/slavkluev/ytr/internal/validate"
)

// newDeleteCmd creates the "link delete" command.
func newDeleteCmd() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "delete ISSUE-KEY LINK-ID",
		Short: "Delete a link",
		Long: `Delete a link from a Yandex Tracker issue.

SEE ALSO
  ytr link list    - List links on issue
  ytr link create  - Create a link to another issue`,
		Example: `  # Delete link 456 from PROJ-123
  ytr link delete PROJ-123 456

  # Delete and confirm via JSON
  ytr link delete PROJ-123 456 --json id`,
		Args: cobra.ExactArgs(2),
		PreRunE: func(cmd *cobra.Command, args []string) error {
			if err := validate.ValidateIssueKey(args[0]); err != nil {
				return err
			}
			_, err := validate.ValidateStringID(args[1], "link ID")
			return err
		},
		RunE: func(cmd *cobra.Command, args []string) error {
			linkID, _ := validate.ValidateStringID(args[1], "link ID")
			return runDelete(cmd, args[0], linkID)
		},
	}

	return cmd
}

// runDelete executes the link delete logic.
func runDelete(cmd *cobra.Command, issueKey, linkID string) error {
	// Resolve auth from root persistent flags.
	tokenFlag, _ := cmd.Root().PersistentFlags().GetString("token")
	orgIDFlag, _ := cmd.Root().PersistentFlags().GetString("org-id")
	orgTypeFlag, _ := cmd.Root().PersistentFlags().GetString("org-type")

	auth, err := config.ResolveAuth(tokenFlag, orgIDFlag, orgTypeFlag)
	if err != nil {
		return err
	}

	deleter := newLinkDeleter(auth)

	// API returns (*Response, error) -- no body on 204 No Content.
	_, err = deleter.DeleteLink(cmd.Context(), issueKey, linkID)
	if err != nil {
		return api.MapAPIError(err)
	}

	w := cmd.OutOrStdout()

	if output.IsJSON() {
		result := map[string]any{"id": linkID, "deleted": true}
		if output.JQFilter != "" {
			return output.ApplyJQ(w, result, output.JQFilter)
		}
		return output.PrintJSON(w, result)
	}

	if output.IsQuiet() {
		output.PrintQuiet(w, linkID)
		return nil
	}

	// Table output: brief confirmation.
	_, err = fmt.Fprintf(w, "Link %s deleted\n", linkID)
	return err
}
