package issue

import (
	"context"

	"github.com/slavkluev/go-yandex-tracker/tracker"
	"github.com/spf13/cobra"

	"github.com/slavkluev/ytr/internal/cmd/runner"
)

func newUpdateCmd() *cobra.Command {
	return runner.Write[tracker.IssueRequest, *tracker.Issue, issueDetail]{
		Use:   "update ISSUE-KEY",
		Short: "Update an issue",
		Long:  `Update an existing Yandex Tracker issue. Only changed fields are sent to the API.`,
		Example: `  # Update issue summary
  ytr issue update PROJ-123 --summary "Updated title"

  # Change priority and assignee
  ytr issue update PROJ-123 --priority critical --assignee jane

  # Update from JSON
  ytr issue update PROJ-123 --from-json '{"summary": "New title"}'`,
		Args: []runner.Arg{runner.IssueKey},
		Flags: []runner.Flag{
			textFlag("summary", "New issue summary"),
			textFlag("description", "New issue description"),
			runner.Text("type", "New issue type key"),
			runner.Text("priority", "New priority key"),
			runner.Text("assignee", "New assignee user ID"),
			runner.Text("parent", "New parent issue key"),
		},
		FromJSON: issueFromJSON,
		Update:   true,
		Call: func(
			ctx context.Context, c *tracker.Client, args []string, req *tracker.IssueRequest,
		) (*tracker.Issue, error) {
			issue, _, err := c.Issues.Edit(ctx, args[0], req, nil)
			return issue, err
		},
		Item:   toIssueDetail,
		Quiet:  issueKey,
		Detail: writtenIssueCard,
	}.Command()
}
