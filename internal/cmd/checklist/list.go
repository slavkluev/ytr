package checklist

import (
	"context"

	"github.com/slavkluev/go-yandex-tracker/tracker"
	"github.com/spf13/cobra"

	"github.com/slavkluev/ytr/internal/api"
	"github.com/slavkluev/ytr/internal/cmd/runner"
	"github.com/slavkluev/ytr/internal/output"
)

type checklistItem struct {
	ID         string `json:"id"`
	Text       string `json:"text"`
	Checked    bool   `json:"checked"`
	Assignee   string `json:"assignee,omitempty"`
	AssigneeID string `json:"assigneeId"`
}

func newListCmd() *cobra.Command {
	return runner.List[*tracker.ChecklistItem, checklistItem]{
		Use:   "list ISSUE-KEY",
		Short: "List checklist items on an issue",
		Long:  `List all checklist items on a Yandex Tracker issue.`,
		SeeAlso: `  ytr checklist create  - Add checklist item to issue
  ytr checklist edit    - Edit a checklist item
  ytr checklist delete  - Delete a checklist item`,
		Example: `  # List checklist items on an issue
  ytr checklist list PROJ-123

  # Get checklist as JSON
  ytr checklist list PROJ-123 --json id,text,checked`,
		Args:  []runner.Arg{runner.IssueKey},
		Empty: "No checklist items found",
		Call: func(ctx context.Context, c *tracker.Client, args []string) ([]*tracker.ChecklistItem, error) {
			items, _, err := c.Issues.ListChecklistItems(ctx, args[0])
			return items, err
		},
		Item:   toChecklistItem,
		Header: []string{"ID", "TEXT", "CHECKED", "ASSIGNEE"},
		Row: func(_ *output.Options, c *tracker.ChecklistItem) []string {
			return []string{
				api.DerefFlexString(c.ID, ""),
				api.DerefString(c.Text, ""),
				checkedDisplay(api.DerefBool(c.Checked, false)),
				c.Assignee.DisplayOr("-"),
			}
		},
		Quiet: func(c *tracker.ChecklistItem) string { return api.DerefFlexString(c.ID, "") },
	}.Command()
}

func toChecklistItem(c *tracker.ChecklistItem) checklistItem {
	return checklistItem{
		ID:         api.DerefFlexString(c.ID, ""),
		Text:       api.DerefString(c.Text, ""),
		Checked:    api.DerefBool(c.Checked, false),
		Assignee:   c.Assignee.DisplayOr(""),
		AssigneeID: c.Assignee.IDOr(""),
	}
}

func checkedDisplay(checked bool) string {
	if checked {
		return "yes"
	}
	return "no"
}

// sameItem is the Item of a write, whose Call already answers with the item it
// prints.
func sameItem(item checklistItem) checklistItem {
	return item
}

func itemID(item checklistItem) string {
	return item.ID
}
