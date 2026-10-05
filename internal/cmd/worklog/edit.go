package worklog

import (
	"context"

	"github.com/slavkluev/go-yandex-tracker/tracker"
	"github.com/spf13/cobra"

	"github.com/slavkluev/ytr/internal/cmd/runner"
)

func newEditCmd() *cobra.Command {
	return runner.Write[tracker.WorklogRequest, *tracker.Worklog, worklogItem]{
		Use:   "edit ISSUE-KEY WORKLOG-ID",
		Short: "Edit a worklog",
		Long: `Edit an existing worklog on a Yandex Tracker issue.

--from-json takes the request body as one JSON object. Each flag is shorthand
for the body key of the same name.`,
		Example: `  # Update duration
  ytr worklog edit PROJ-123 abc123 --from-json '{"duration":"PT2H"}'

  # Update comment
  ytr worklog edit PROJ-123 abc123 --from-json '{"comment":"Updated notes"}'`,
		Args: []runner.Arg{runner.IssueKey, runner.StringID("worklog ID")},
		Flags: []runner.Flag{
			runner.Duration("duration", "Duration in ISO 8601 format (e.g., PT1H30M)"),
			runner.Text("comment", "Worklog comment"),
			runner.Time("start", "Start time in RFC 3339 format"),
		},
		Update: true,
		Call: func(
			ctx context.Context, c *tracker.Client, args []string, req *tracker.WorklogRequest,
		) (*tracker.Worklog, error) {
			wl, _, err := c.Issues.EditWorklog(ctx, args[0], args[1], req)
			return wl, err
		},
		Item: toWorklogItem,
	}.Command()
}
