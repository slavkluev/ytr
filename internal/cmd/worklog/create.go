package worklog

import (
	"context"

	"github.com/slavkluev/go-yandex-tracker/tracker"
	"github.com/spf13/cobra"

	"github.com/slavkluev/ytr/internal/cmd/runner"
)

func newCreateCmd() *cobra.Command {
	return runner.Write[tracker.WorklogRequest, *tracker.Worklog, worklogItem]{
		Use:   "create ISSUE-KEY",
		Short: "Create a worklog",
		Long: `Create a new worklog on a Yandex Tracker issue.

Durations use ISO 8601 format: PT1H30M (1h30m), PT45M (45min), P1D (1 day),
P1DT2H (1 day 2 hours).

Tracker requires both duration and start time when creating a worklog.

--from-json takes the request body as one JSON object. Each flag is shorthand
for the body key of the same name.`,
		Example: `  # Log 1h30m of work
  ytr worklog create PROJ-123 --from-json '{"start":"2026-03-30T10:00:00Z","duration":"PT1H30M"}'

  # Log with a comment
  ytr worklog create PROJ-123 --from-json '{"start":"2026-03-30T10:00:00Z","duration":"PT2H","comment":"Code review"}'`,
		Args: []runner.Arg{runner.IssueKey},
		Flags: []runner.Flag{
			runner.Duration("duration", "Duration in ISO 8601 format (e.g., PT1H30M) (required)"),
			runner.Time("start", "Start time in RFC 3339 format (e.g., 2026-03-30T10:00:00Z) (required)"),
			runner.Text("comment", "Worklog comment"),
		},
		Required: []string{"start", "duration"},
		Call: func(
			ctx context.Context, c *tracker.Client, args []string, req *tracker.WorklogRequest,
		) (*tracker.Worklog, error) {
			wl, _, err := c.Issues.CreateWorklog(ctx, args[0], req)
			return wl, err
		},
		Item: toWorklogItem,
	}.Command()
}
