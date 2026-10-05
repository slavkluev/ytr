package comment

import (
	"context"

	"github.com/slavkluev/go-yandex-tracker/tracker"
	"github.com/spf13/cobra"

	"github.com/slavkluev/ytr/internal/cmd/runner"
	"github.com/slavkluev/ytr/internal/validate"
)

func newCreateCmd() *cobra.Command {
	return runner.Write[tracker.CommentRequest, *tracker.Comment, commentItem]{
		Use:   "create ISSUE-KEY",
		Short: "Add comment to issue",
		Long:  `Create a new comment on a Yandex Tracker issue.`,
		Example: `  # Add a comment
  ytr comment create PROJ-123 --body "Fixed in commit abc123"

  # Add a comment and print only its ID
  ytr comment create PROJ-123 --body "Done" --json id --jq '.id'`,
		Args:     []runner.Arg{runner.IssueKey},
		Flags:    []runner.Flag{bodyFlag("Comment text (required)")},
		Required: []string{"text"},
		Call: func(
			ctx context.Context, c *tracker.Client, args []string, req *tracker.CommentRequest,
		) (*tracker.Comment, error) {
			comment, _, err := c.Issues.CreateComment(ctx, args[0], req)
			return comment, err
		},
		Item: toCommentItem,
	}.Command()
}

func bodyFlag(usage string) runner.Flag {
	return runner.Text("body", usage).Key("text").Check(func(body string) error {
		return validate.ValidateNoControlChars("body", body)
	})
}
