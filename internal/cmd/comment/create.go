package comment

import (
	"fmt"

	"github.com/slavkluev/go-yandex-tracker/tracker"
	"github.com/spf13/cobra"

	"github.com/slavkluev/ytr/internal/api"
	"github.com/slavkluev/ytr/internal/cmd/runner"
	"github.com/slavkluev/ytr/internal/config"
	"github.com/slavkluev/ytr/internal/output"
	"github.com/slavkluev/ytr/internal/validate"
)

func newCreateCmd() *cobra.Command {
	var bodyFlag string

	cmd := &cobra.Command{
		Use:   "create ISSUE-KEY",
		Short: "Add comment to issue",
		Long: `Create a new comment on a Yandex Tracker issue.

JSON FIELDS
  id, author, authorId, body, createdAt, updatedAt

SEE ALSO
  ytr comment list  - List comments on issue
  ytr issue view    - View issue details`,
		Example: `  # Add a comment
  ytr comment create PROJ-123 --body "Fixed in commit abc123"

  # Add comment and get ID as JSON
  ytr comment create PROJ-123 --body "Done" --json id --jq '.id'`,
		Args: cobra.ExactArgs(1),
		PreRunE: func(cmd *cobra.Command, args []string) error {
			if err := validate.ValidateIssueKey(args[0]); err != nil {
				return err
			}
			return validate.ValidateNoControlChars("body", bodyFlag)
		},
		RunE: func(cmd *cobra.Command, args []string) error {
			return runCreate(cmd, args[0], bodyFlag)
		},
	}

	cmd.Flags().StringVar(&bodyFlag, "body", "", "Comment text (required)")
	_ = cmd.MarkFlagRequired("body")

	runner.SetFields(cmd, CommentFields)

	return cmd
}

func runCreate(cmd *cobra.Command, issueKey, body string) error {
	opts := output.FromContext(cmd.Context())

	if opts.WantsFieldHint(cmd.Flags().Changed("json")) {
		return output.PrintFieldHint(cmd.ErrOrStderr(), "comment create", CommentFields)
	}

	if opts.JQFilter != "" && !opts.HasFieldSelection() {
		opts.JSONFields = CommentFields
	}

	if opts.HasFieldSelection() {
		if err := output.ValidateFields(opts.JSONFields, CommentFields); err != nil {
			return err
		}
		opts.JSONFields = output.NormalizeFields(opts.JSONFields, CommentFields)
	}

	tokenFlag, _ := cmd.Root().PersistentFlags().GetString("token")
	orgIDFlag, _ := cmd.Root().PersistentFlags().GetString("org-id")
	orgTypeFlag, _ := cmd.Root().PersistentFlags().GetString("org-type")

	auth, err := config.ResolveAuth(tokenFlag, orgIDFlag, orgTypeFlag)
	if err != nil {
		return err
	}

	creator := newCommentCreator(auth)

	req := &tracker.CommentRequest{
		Text: new(body),
	}

	comment, _, err := creator.CreateComment(cmd.Context(), issueKey, req)
	if err != nil {
		return api.MapAPIError(err)
	}

	w := cmd.OutOrStdout()

	if opts.IsJSON() {
		item := toCommentItem(comment)
		if opts.HasFieldSelection() {
			filtered := output.FilterFields(item, opts.JSONFields)
			if opts.JQFilter != "" {
				return output.ApplyJQ(w, filtered, opts.JQFilter)
			}
			return opts.PrintJSON(w, filtered)
		}
		if opts.JQFilter != "" {
			return output.ApplyJQ(w, item, opts.JQFilter)
		}
		return opts.PrintJSON(w, item)
	}

	if opts.Quiet {
		output.PrintQuiet(w, api.DerefFlexString(comment.ID, ""))
		return nil
	}

	_, err = fmt.Fprintf(w, "Comment %s added to %s\n", api.DerefFlexString(comment.ID, ""), issueKey)
	return err
}
