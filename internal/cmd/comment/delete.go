package comment

import (
	"fmt"

	"github.com/spf13/cobra"

	"github.com/slavkluev/ytr/internal/api"
	"github.com/slavkluev/ytr/internal/config"
	"github.com/slavkluev/ytr/internal/output"
	"github.com/slavkluev/ytr/internal/validate"
)

func newDeleteCmd() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "delete ISSUE-KEY COMMENT-ID",
		Short: "Delete a comment",
		Long: `Delete a comment from a Yandex Tracker issue.

SEE ALSO
  ytr comment list    - List comments on issue
  ytr comment create  - Add comment to issue
  ytr comment edit    - Edit a comment`,
		Example: `  # Delete comment 42 from PROJ-123
  ytr comment delete PROJ-123 42

  # Delete and confirm via JSON
  ytr comment delete PROJ-123 42 --json id`,
		Args: cobra.ExactArgs(2),
		PreRunE: func(cmd *cobra.Command, args []string) error {
			if err := validate.ValidateIssueKey(args[0]); err != nil {
				return err
			}
			_, err := validate.ValidateNumericID(args[1], "comment ID")
			return err
		},
		RunE: func(cmd *cobra.Command, args []string) error {
			return runDelete(cmd, args[0], args[1])
		},
	}

	return cmd
}

func runDelete(cmd *cobra.Command, issueKey string, commentID string) error {
	opts := output.FromContext(cmd.Context())

	tokenFlag, _ := cmd.Root().PersistentFlags().GetString("token")
	orgIDFlag, _ := cmd.Root().PersistentFlags().GetString("org-id")
	orgTypeFlag, _ := cmd.Root().PersistentFlags().GetString("org-type")

	auth, err := config.ResolveAuth(tokenFlag, orgIDFlag, orgTypeFlag)
	if err != nil {
		return err
	}

	deleter := newCommentDeleter(auth)

	// API returns (*Response, error) -- no body on 204 No Content.
	_, err = deleter.DeleteComment(cmd.Context(), issueKey, commentID)
	if err != nil {
		return api.MapAPIError(err)
	}

	w := cmd.OutOrStdout()

	if opts.IsJSON() {
		result := map[string]any{"id": commentID, "deleted": true}
		if opts.JQFilter != "" {
			return output.ApplyJQ(w, result, opts.JQFilter)
		}
		return opts.PrintJSON(w, result)
	}

	if opts.Quiet {
		output.PrintQuiet(w, commentID)
		return nil
	}

	_, err = fmt.Fprintf(w, "Comment %s deleted\n", commentID)
	return err
}
