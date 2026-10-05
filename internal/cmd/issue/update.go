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
		Long: `Update an existing Yandex Tracker issue. Only changed fields are sent to the API.

--from-json takes the request body as one JSON object. Each flag is shorthand
for the body key of the same name.`,
		Example: `  # Update issue summary
  ytr issue update PROJ-123 --from-json '{"summary":"Updated title"}'

  # Change priority and assignee
  ytr issue update PROJ-123 --from-json '{"priority":"critical","assignee":"jane"}'

  # Update from a JSON file
  ytr issue update PROJ-123 --from-json @update.json`,
		Args: []runner.Arg{runner.IssueKey},
		Flags: []runner.Flag{
			textFlag("summary", "New issue summary"),
			textFlag("description", "New issue description"),
			runner.Text("type", "New issue type key"),
			runner.Text("priority", "New priority key"),
			runner.Text("assignee", "New assignee user ID"),
			runner.Text("parent", "New parent issue key"),
		},
		Update: true,
		Call: func(
			ctx context.Context, c *tracker.Client, args []string, req *tracker.IssueRequest,
		) (*tracker.Issue, error) {
			issue, _, err := c.Issues.Edit(ctx, args[0], req, nil)
			return issue, err
		},
		Item: toIssueDetail,
	}.Command()
}
