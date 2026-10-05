package comment

import (
	"context"
	"time"

	"github.com/slavkluev/go-yandex-tracker/tracker"
	"github.com/spf13/cobra"

	"github.com/slavkluev/ytr/internal/api"
	"github.com/slavkluev/ytr/internal/cmd/runner"
)

// commentPageSize only saves requests where Tracker honours it: the list pages
// until an empty page, whatever size each page comes in.
const commentPageSize = 100

type commentItem struct {
	ID        string `json:"id"`
	Author    string `json:"author"`
	AuthorID  string `json:"authorId"`
	Body      string `json:"body"`
	CreatedAt string `json:"createdAt"`
	UpdatedAt string `json:"updatedAt,omitempty"`
}

func newListCmd() *cobra.Command {
	return runner.List[*tracker.Comment, commentItem]{
		Use:   "list ISSUE-KEY",
		Short: "List comments on an issue",
		Long:  `List all comments on a Yandex Tracker issue.`,
		Example: `  # List comments on an issue
  ytr comment list PROJ-123

  # Only IDs, authors and bodies
  ytr comment list PROJ-123 --json id,author,body

  # Extract comment bodies with jq
  ytr comment list PROJ-123 --json body --jq '.[].body'`,
		Args: []runner.Arg{runner.IssueKey},
		Call: func(ctx context.Context, c *tracker.Client, args []string) ([]*tracker.Comment, error) {
			opts := &tracker.CommentListOptions{PerPage: commentPageSize}
			return runner.Collect(c.Issues.ListCommentsIter(ctx, args[0], opts))
		},
		Item: toCommentItem,
	}.Command()
}

func toCommentItem(c *tracker.Comment) commentItem {
	item := commentItem{
		ID:       api.DerefFlexString(c.ID, ""),
		Author:   c.CreatedBy.DisplayOr(""),
		AuthorID: c.CreatedBy.IDOr(""),
		Body:     api.DerefString(c.Text, ""),
	}

	if c.CreatedAt != nil {
		item.CreatedAt = c.CreatedAt.Format(time.RFC3339)
	}
	if c.UpdatedAt != nil {
		item.UpdatedAt = c.UpdatedAt.Format(time.RFC3339)
	}

	return item
}
