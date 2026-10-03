package comment

import (
	"fmt"
	"io"

	"github.com/slavkluev/go-yandex-tracker/tracker"
	"github.com/spf13/cobra"

	"github.com/slavkluev/ytr/internal/api"
	"github.com/slavkluev/ytr/internal/cmd/jsonfields"
	"github.com/slavkluev/ytr/internal/config"
	"github.com/slavkluev/ytr/internal/errors"
	"github.com/slavkluev/ytr/internal/output"
	"github.com/slavkluev/ytr/internal/validate"
)

func newEditCmd() *cobra.Command {
	var (
		bodyFlag string
		fromJSON string
	)

	cmd := &cobra.Command{
		Use:   "edit ISSUE-KEY COMMENT-ID",
		Short: "Edit a comment",
		Long: `Edit an existing comment on a Yandex Tracker issue.

Provide the updated text via --body or full JSON via --from-json.

JSON FIELDS
  id, author, authorId, body, createdAt, updatedAt

SEE ALSO
  ytr comment list    - List comments on issue
  ytr comment create  - Add comment to issue
  ytr comment delete  - Delete a comment`,
		Example: `  # Edit comment body
  ytr comment edit PROJ-123 42 --body "Updated text"

  # Edit via JSON input
  ytr comment edit PROJ-123 42 --from-json '{"text": "new body"}'

  # Edit and get result as JSON
  ytr comment edit PROJ-123 42 --body "Fixed" --json id,body`,
		Args: cobra.ExactArgs(2),
		PreRunE: func(cmd *cobra.Command, args []string) error {
			if err := validate.ValidateIssueKey(args[0]); err != nil {
				return err
			}
			if _, err := validate.ValidateNumericID(args[1], "comment ID"); err != nil {
				return err
			}

			if cmd.Flags().Changed("body") && cmd.Flags().Changed("from-json") {
				return errors.NewUserError(
					"cannot use --body and --from-json together",
					"Use --body for simple text updates, or --from-json for full JSON input",
				)
			}

			if !cmd.Flags().Changed("body") && !cmd.Flags().Changed("from-json") {
				return errors.NewUserError(
					"either --body or --from-json is required",
					"Provide --body \"text\" or --from-json '{\"text\": \"...\"}'",
				)
			}

			if cmd.Flags().Changed("body") {
				return validate.ValidateNoControlChars("body", bodyFlag)
			}
			return nil
		},
		RunE: func(cmd *cobra.Command, args []string) error {
			return runEdit(cmd, args[0], args[1], bodyFlag, fromJSON)
		},
	}

	cmd.Flags().StringVar(&bodyFlag, "body", "", "Updated comment text")
	cmd.Flags().StringVar(&fromJSON, "from-json", "", `JSON input: inline '{"text":"..."}', @file, or - for stdin`)

	jsonfields.Register("ytr comment edit", CommentFields)

	return cmd
}

func runEdit(cmd *cobra.Command, issueKey string, commentID string, body, fromJSON string) error {
	opts := output.FromContext(cmd.Context())

	if opts.WantsFieldHint(cmd.Flags().Changed("json")) {
		return output.PrintFieldHint(cmd.ErrOrStderr(), "comment edit", CommentFields)
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

	var req *tracker.CommentRequest

	if cmd.Flags().Changed("from-json") {
		data, parseErr := validate.ParseJSONInput(fromJSON)
		if parseErr != nil {
			return parseErr
		}
		req = &tracker.CommentRequest{}
		if unmarshalErr := validate.UnmarshalRequestJSON(data, req); unmarshalErr != nil {
			return unmarshalErr
		}
	} else {
		req = &tracker.CommentRequest{Text: new(body)}
	}

	editor := newCommentEditor(auth)

	comment, _, err := editor.EditComment(cmd.Context(), issueKey, commentID, req)
	if err != nil {
		return api.MapAPIError(err)
	}

	return renderEditOutput(cmd.OutOrStdout(), opts, comment, commentID, issueKey)
}

func renderEditOutput(
	w io.Writer,
	opts *output.Options,
	comment *tracker.Comment,
	commentID string,
	issueKey string,
) error {
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

	_, err := fmt.Fprintf(w, "Comment %s updated on %s\n", commentID, issueKey)
	return err
}
