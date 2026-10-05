package checklist

import (
	"context"

	"github.com/slavkluev/go-yandex-tracker/tracker"
	"github.com/spf13/cobra"

	"github.com/slavkluev/ytr/internal/api"
	"github.com/slavkluev/ytr/internal/cmd/runner"
)

func newEditCmd() *cobra.Command {
	return runner.Write[tracker.ChecklistItemRequest, checklistItem, checklistItem]{
		Use:   "edit ISSUE-KEY ITEM-ID",
		Short: "Edit a checklist item",
		Long: `Edit an existing checklist item on a Yandex Tracker issue.

--from-json takes the request body as one JSON object. Each flag is shorthand
for the body key of the same name; "deadline" has no flag.

Set "checked" to true to mark an item as done, to false to unmark it.`,
		Example: `  # Update checklist item text
  ytr checklist edit PROJ-123 item-1 --from-json '{"text":"Updated text"}'

  # Mark item as checked
  ytr checklist edit PROJ-123 item-1 --from-json '{"checked":true}'

  # Unmark item
  ytr checklist edit PROJ-123 item-1 --from-json '{"checked":false}'`,
		Args: []runner.Arg{runner.IssueKey, runner.StringID("checklist item ID")},
		Flags: []runner.Flag{
			runner.Text("text", "Checklist item text"),
			runner.Bool("checked", "Mark item as checked (--checked=false to unmark)"),
			runner.Text("assignee", "Assignee user ID"),
		},
		Update: true,
		Call: func(
			ctx context.Context, c *tracker.Client, args []string, req *tracker.ChecklistItemRequest,
		) (checklistItem, error) {
			issue, _, err := c.Issues.EditChecklistItem(ctx, args[0], args[1], req)
			if err != nil {
				return checklistItem{}, err
			}

			// The edit has happened by now and the item ID is known, so an answer
			// that does not show the item is confirmed from the request instead
			// of failing.
			if edited := extractEditedItem(issue, args[1]); edited != nil {
				return toChecklistItem(edited), nil
			}
			return editedChecklistItem(args[1], req), nil
		},
		Item: sameItem,
	}.Command()
}

func extractEditedItem(issue *tracker.Issue, itemID string) *tracker.ChecklistItem {
	if issue == nil {
		return nil
	}
	for _, item := range issue.ChecklistItems {
		if item == nil {
			continue
		}
		if api.DerefFlexString(item.ID, "") == itemID {
			return item
		}
	}
	return nil
}

func editedChecklistItem(itemID string, req *tracker.ChecklistItemRequest) checklistItem {
	item := checklistItem{ID: itemID}
	if req != nil {
		item.Text = api.DerefString(req.Text, "")
		item.Checked = api.DerefBool(req.Checked, false)
		item.Assignee = api.DerefString(req.Assignee, "")
	}
	return item
}
