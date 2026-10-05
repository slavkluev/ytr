package checklist

import (
	"context"

	"github.com/slavkluev/go-yandex-tracker/tracker"
	"github.com/spf13/cobra"

	"github.com/slavkluev/ytr/internal/api"
	"github.com/slavkluev/ytr/internal/cmd/runner"
)

func newCreateCmd() *cobra.Command {
	return runner.Write[tracker.ChecklistItemRequest, checklistItem, checklistItem]{
		Use:   "create ISSUE-KEY",
		Short: "Add checklist item to issue",
		Long: `Create a new checklist item on a Yandex Tracker issue.

--from-json takes the request body as one JSON object. Each flag is shorthand
for the body key of the same name; "deadline" has no flag.`,
		Example: `  # Create a checklist item
  ytr checklist create PROJ-123 --from-json '{"text":"Review PR"}'

  # Create with assignee
  ytr checklist create PROJ-123 --from-json '{"text":"Deploy","assignee":"12345"}'

  # Create with a deadline
  ytr checklist create PROJ-123 --from-json '{"text":"Review","deadline":{"date":"2026-04-01T00:00:00Z"}}'`,
		Args: []runner.Arg{runner.IssueKey},
		Flags: []runner.Flag{
			runner.Text("text", "Checklist item text (required)"),
			runner.Text("assignee", "Assignee user ID"),
		},
		Required: []string{"text"},
		Call: func(
			ctx context.Context, c *tracker.Client, args []string, req *tracker.ChecklistItemRequest,
		) (checklistItem, error) {
			issue, _, err := c.Issues.CreateChecklistItem(ctx, args[0], req)
			if err != nil {
				return checklistItem{}, err
			}

			// The item is created by now, so an answer that does not show it is
			// confirmed from the request: failing would make an agent retry and
			// create a duplicate.
			if created := extractCreatedItem(issue, req); created != nil {
				return toChecklistItem(created), nil
			}
			return requestedChecklistItem(req), nil
		},
		Item: sameItem,
	}.Command()
}

// The API echoes the full checklist and item order is not guaranteed, so it
// matches by the requested text (newest match wins on duplicates, since the API
// appends). When the text is empty it falls back to the last non-nil item.
// Returns nil only when no item is present.
func extractCreatedItem(issue *tracker.Issue, req *tracker.ChecklistItemRequest) *tracker.ChecklistItem {
	if issue == nil {
		return nil
	}
	if req != nil && req.Text != nil && *req.Text != "" {
		for i := len(issue.ChecklistItems) - 1; i >= 0; i-- {
			if item := issue.ChecklistItems[i]; item != nil &&
				api.DerefString(item.Text, "") == *req.Text {
				return item
			}
		}
	}
	// API appends new items at the end; fall back to the last non-nil item.
	for i := len(issue.ChecklistItems) - 1; i >= 0; i-- {
		if item := issue.ChecklistItems[i]; item != nil {
			return item
		}
	}
	return nil
}

func requestedChecklistItem(req *tracker.ChecklistItemRequest) checklistItem {
	if req == nil {
		return checklistItem{}
	}
	return checklistItem{
		Text:     api.DerefString(req.Text, ""),
		Checked:  api.DerefBool(req.Checked, false),
		Assignee: api.DerefString(req.Assignee, ""),
	}
}
