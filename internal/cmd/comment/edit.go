package comment

import (
	"context"

	"github.com/slavkluev/go-yandex-tracker/tracker"
	"github.com/spf13/cobra"

	"github.com/slavkluev/ytr/internal/cmd/runner"
)

func newEditCmd() *cobra.Command {
	return runner.Write[tracker.CommentRequest, *tracker.Comment, commentItem]{
		Use:   "edit ISSUE-KEY COMMENT-ID",
		Short: "Edit a comment",
		Long: `Edit an existing comment on a Yandex Tracker issue.

Provide the updated text via --body or full JSON via --from-json.`,
		Example: `  # Edit comment body
  ytr comment edit PROJ-123 42 --body "Updated text"

  # Edit via JSON input
  ytr comment edit PROJ-123 42 --from-json '{"text": "new body"}'

  # Edit and keep only the ID and body
  ytr comment edit PROJ-123 42 --body "Fixed" --json id,body`,
		Args:     []runner.Arg{runner.IssueKey, runner.NumericID("comment ID")},
		Flags:    []runner.Flag{bodyFlag("Updated comment text")},
		FromJSON: `JSON input: inline '{"text":"..."}', @file, or - for stdin`,
		Update:   true,
		Call: func(
			ctx context.Context, c *tracker.Client, args []string, req *tracker.CommentRequest,
		) (*tracker.Comment, error) {
			comment, _, err := c.Issues.EditComment(ctx, args[0], args[1], req)
			return comment, err
		},
		Item: toCommentItem,
	}.Command()
}
