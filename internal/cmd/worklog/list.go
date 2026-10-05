package worklog

import (
	"context"
	"time"

	"github.com/slavkluev/go-yandex-tracker/tracker"
	"github.com/spf13/cobra"

	"github.com/slavkluev/ytr/internal/api"
	"github.com/slavkluev/ytr/internal/cmd/runner"
)

type worklogItem struct {
	ID       string `json:"id"`
	Author   string `json:"author"`
	AuthorID string `json:"authorId"`
	Duration string `json:"duration"`
	Start    string `json:"start"`
	Comment  string `json:"comment,omitempty"`
}

func newListCmd() *cobra.Command {
	return runner.List[*tracker.Worklog, worklogItem]{
		Use:   "list ISSUE-KEY",
		Short: "List worklogs on an issue",
		Long:  `List all worklogs on a Yandex Tracker issue.`,
		Example: `  # List worklogs on an issue
  ytr worklog list PROJ-123

  # Only IDs, durations and start times
  ytr worklog list PROJ-123 --json id,duration,start

  # Extract durations with jq
  ytr worklog list PROJ-123 --jq '.[].duration'`,
		Args: []runner.Arg{runner.IssueKey},
		Call: func(ctx context.Context, c *tracker.Client, args []string) ([]*tracker.Worklog, error) {
			worklogs, _, err := c.Issues.ListWorklogs(ctx, args[0])
			return worklogs, err
		},
		Item: toWorklogItem,
	}.Command()
}

func toWorklogItem(wl *tracker.Worklog) worklogItem {
	item := worklogItem{
		ID:       api.DerefFlexString(wl.ID, ""),
		Author:   wl.CreatedBy.DisplayOr(""),
		AuthorID: wl.CreatedBy.IDOr(""),
		Duration: formatDuration(wl.Duration),
		Comment:  api.DerefString(wl.Comment, ""),
	}

	if wl.Start != nil {
		item.Start = wl.Start.Format(time.RFC3339)
	}

	return item
}

func formatDuration(d *tracker.Duration) string {
	if d == nil {
		return ""
	}

	return d.String()
}
