package issue

import (
	"context"

	"github.com/slavkluev/go-yandex-tracker/tracker"
	"github.com/spf13/cobra"

	"github.com/slavkluev/ytr/internal/api"
	"github.com/slavkluev/ytr/internal/cmd/runner"
	"github.com/slavkluev/ytr/internal/output"
	"github.com/slavkluev/ytr/internal/validate"
)

const issueFromJSON = "JSON input: inline string, @file, or - for stdin"

func newCreateCmd() *cobra.Command {
	return runner.Write[tracker.IssueRequest, *tracker.Issue, issueDetail]{
		Use:   "create",
		Short: "Create an issue",
		Long:  `Create a new Yandex Tracker issue with flags or raw JSON input.`,
		Example: `  # Create a simple issue
  ytr issue create --queue PROJ --summary "Fix login bug"

  # Create with all fields
  ytr issue create --queue PROJ --summary "Add feature" --type task --priority normal --assignee john

  # Create from JSON file
  ytr issue create --from-json @issue.json

  # Create and get the new key
  ytr issue create --queue PROJ --summary "Bug" --json key --jq '.key'`,
		Flags: []runner.Flag{
			runner.Text("queue", "Queue key (required unless --from-json)"),
			textFlag("summary", "Issue summary (required unless --from-json)"),
			textFlag("description", "Issue description"),
			runner.Text("type", "Issue type key"),
			runner.Text("priority", "Priority key"),
			runner.Text("assignee", "Assignee user ID"),
			runner.Text("parent", "Parent issue key"),
		},
		FromJSON: issueFromJSON,
		Required: []string{"queue", "summary"},
		Call: func(
			ctx context.Context, c *tracker.Client, _ []string, req *tracker.IssueRequest,
		) (*tracker.Issue, error) {
			issue, _, err := c.Issues.Create(ctx, req)
			return issue, err
		},
		Item:   toIssueDetail,
		Quiet:  issueKey,
		Detail: writtenIssueCard,
	}.Command()
}

// textFlag is a flag whose value may hold no control character but tab,
// newline and carriage return.
func textFlag(name, usage string) runner.Flag {
	return runner.Text(name, usage).Check(func(value string) error {
		return validate.ValidateNoControlChars(name, value)
	})
}

// writtenIssueCard is the short card of the issue Tracker answers a create or
// an update with.
func writtenIssueCard(d *output.DetailPrinter, _ *output.Options, issue *tracker.Issue) {
	d.Field("Key", api.DerefString(issue.Key, "-"))
	d.Field("Summary", api.DerefString(issue.Summary, "-"))
	d.Field("Status", issueStatusDisplay(issue))
}
