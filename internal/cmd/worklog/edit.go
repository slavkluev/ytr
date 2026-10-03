package worklog

import (
	"context"
	"fmt"

	"github.com/slavkluev/go-yandex-tracker/tracker"
	"github.com/spf13/cobra"

	"github.com/slavkluev/ytr/internal/cmd/runner"
)

func newEditCmd() *cobra.Command {
	return runner.Write[tracker.WorklogRequest, *tracker.Worklog, worklogItem]{
		Use:   "edit ISSUE-KEY WORKLOG-ID",
		Short: "Edit a worklog",
		Long: `Edit an existing worklog on a Yandex Tracker issue.

Provide one or more flags to update, or --from-json for full JSON input.`,
		Example: `  # Update duration
  ytr worklog edit PROJ-123 abc123 --duration PT2H

  # Update comment
  ytr worklog edit PROJ-123 abc123 --comment "Updated notes"

  # Update via JSON
  ytr worklog edit PROJ-123 abc123 --from-json '{"duration":"PT3H"}'`,
		Args: []runner.Arg{runner.IssueKey, runner.StringID("worklog ID")},
		Flags: []runner.Flag{
			runner.Duration("duration", "Duration in ISO 8601 format (e.g., PT1H30M)"),
			runner.Text("comment", "Worklog comment"),
			runner.Time("start", "Start time in RFC 3339 format"),
		},
		FromJSON: `JSON input: inline '{"duration":"PT1H"}', @file, or - for stdin`,
		Update:   true,
		Call: func(
			ctx context.Context, c *tracker.Client, args []string, req *tracker.WorklogRequest,
		) (*tracker.Worklog, error) {
			wl, _, err := c.Issues.EditWorklog(ctx, args[0], args[1], req)
			return wl, err
		},
		Item:  toWorklogItem,
		Quiet: worklogID,
		Confirm: func(args []string, wl *tracker.Worklog) string {
			return fmt.Sprintf("Worklog %s updated on %s", worklogID(wl), args[0])
		},
	}.Command()
}
