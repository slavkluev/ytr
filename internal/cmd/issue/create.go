package issue

import (
	"context"

	"github.com/slavkluev/go-yandex-tracker/tracker"
	"github.com/spf13/cobra"

	"github.com/slavkluev/ytr/internal/cmd/runner"
	"github.com/slavkluev/ytr/internal/validate"
)

func newCreateCmd() *cobra.Command {
	return runner.Write[tracker.IssueRequest, *tracker.Issue, issueDetail]{
		Use:   "create",
		Short: "Create an issue",
		Long: `Create a new Yandex Tracker issue.

--from-json takes the request body as one JSON object. Each flag is shorthand
for the body key of the same name.`,
		Example: `  # Create a simple issue
  ytr issue create --from-json '{"queue":"PROJ","summary":"Fix login bug"}'

  # Create with more fields
  ytr issue create --from-json '{"queue":"PROJ","summary":"Add feature","type":"task","assignee":"john"}'

  # Create from a JSON file
  ytr issue create --from-json @issue.json

  # Create and get the new key
  ytr issue create --from-json '{"queue":"PROJ","summary":"Bug"}' --json key --jq '.key'`,
		Flags: []runner.Flag{
			runner.Text("queue", "Queue key (required)"),
			textFlag("summary", "Issue summary (required)"),
			textFlag("description", "Issue description"),
			runner.Text("type", "Issue type key"),
			runner.Text("priority", "Priority key"),
			runner.Text("assignee", "Assignee user ID"),
			runner.Text("parent", "Parent issue key"),
		},
		Required: []string{"queue", "summary"},
		Call: func(
			ctx context.Context, c *tracker.Client, _ []string, req *tracker.IssueRequest,
		) (*tracker.Issue, error) {
			issue, _, err := c.Issues.Create(ctx, req)
			return issue, err
		},
		Item: toIssueDetail,
	}.Command()
}

// textFlag is a flag whose value may hold no control character but tab,
// newline and carriage return.
func textFlag(name, usage string) runner.Flag {
	return runner.Text(name, usage).Check(func(value string) error {
		return validate.ValidateNoControlChars(name, value)
	})
}
