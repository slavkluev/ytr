package worklog

import (
	"context"
	"fmt"

	"github.com/slavkluev/go-yandex-tracker/tracker"
	"github.com/spf13/cobra"

	"github.com/slavkluev/ytr/internal/api"
	"github.com/slavkluev/ytr/internal/cmd/runner"
)

func newCreateCmd() *cobra.Command {
	return runner.Write[tracker.WorklogRequest, *tracker.Worklog, worklogItem]{
		Use:   "create ISSUE-KEY",
		Short: "Create a worklog",
		Long: `Create a new worklog on a Yandex Tracker issue.

Durations use ISO 8601 format: PT1H30M (1h30m), PT45M (45min), P1D (1 day),
P1DT2H (1 day 2 hours).

Tracker requires both duration and start time when creating a worklog.`,
		Example: `  # Log 1h30m of work
  ytr worklog create PROJ-123 --duration PT1H30M --start 2026-03-30T10:00:00Z

  # Log with comment and start time
  ytr worklog create PROJ-123 --duration PT2H --comment "Code review" --start 2026-03-30T10:00:00Z

  # Create via JSON
  ytr worklog create PROJ-123 --from-json '{"start":"2026-03-30T10:00:00Z","duration":"PT1H","comment":"Bug fix"}'`,
		Args: []runner.Arg{runner.IssueKey},
		Flags: []runner.Flag{
			runner.Duration("duration", "Duration in ISO 8601 format (e.g., PT1H30M) (required)"),
			runner.Time("start", "Start time in RFC 3339 format (e.g., 2026-03-30T10:00:00Z) (required)"),
			runner.Text("comment", "Worklog comment"),
		},
		FromJSON: `JSON input: inline '{"start":"2026-03-30T10:00:00Z","duration":"PT1H"}', @file, or - for stdin`,
		Required: []string{"start", "duration"},
		Call: func(
			ctx context.Context, c *tracker.Client, args []string, req *tracker.WorklogRequest,
		) (*tracker.Worklog, error) {
			wl, _, err := c.Issues.CreateWorklog(ctx, args[0], req)
			return wl, err
		},
		Item:  toWorklogItem,
		Quiet: worklogID,
		Confirm: func(args []string, wl *tracker.Worklog) string {
			return fmt.Sprintf("Worklog %s created on %s", worklogID(wl), args[0])
		},
	}.Command()
}

func worklogID(wl *tracker.Worklog) string {
	return api.DerefFlexString(wl.ID, "")
}
