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
		Long: `Create a new comment on a Yandex Tracker issue.

--from-json takes the request body as one JSON object. --body is shorthand for
its "text" key.`,
		Example: `  # Add a comment
  ytr comment create PROJ-123 --from-json '{"text":"Fixed in commit abc123"}'

  # Add a comment that summons a user
  ytr comment create PROJ-123 --from-json '{"text":"Please review","summonees":["uid-a"]}'

  # Add a comment whose body is in a file
  ytr comment create PROJ-123 --from-json @comment.json

  # Add a comment and print only its ID
  ytr comment create PROJ-123 --from-json '{"text":"Done"}' --json id --jq '.id'`,
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
