package checklist

import (
	"context"

	"github.com/slavkluev/go-yandex-tracker/tracker"
	"github.com/spf13/cobra"

	"github.com/slavkluev/ytr/internal/api"
	"github.com/slavkluev/ytr/internal/cmd/runner"
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
		Example: `  # List checklist items on an issue
  ytr checklist list PROJ-123

  # Only IDs, texts and checked states
  ytr checklist list PROJ-123 --json id,text,checked`,
		Args: []runner.Arg{runner.IssueKey},
		Call: func(ctx context.Context, c *tracker.Client, args []string) ([]*tracker.ChecklistItem, error) {
			items, _, err := c.Issues.ListChecklistItems(ctx, args[0])
			return items, err
		},
		Item: toChecklistItem,
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

// sameItem is the Item of a write, whose Call already answers with the item it
// prints.
func sameItem(item checklistItem) checklistItem {
	return item
}
