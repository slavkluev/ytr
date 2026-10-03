package link

import (
	"context"

	"github.com/slavkluev/go-yandex-tracker/tracker"
	"github.com/spf13/cobra"

	"github.com/slavkluev/ytr/internal/api"
	"github.com/slavkluev/ytr/internal/cmd/runner"
	"github.com/slavkluev/ytr/internal/output"
)

type linkItem struct {
	ID      string `json:"id"`
	Type    string `json:"type"`
	Issue   string `json:"issue"`
	Summary string `json:"summary"`
}

// LinkListFields are the --json fields of every link command.
var LinkListFields = runner.ItemFields[linkItem]()

func linkTypeDisplay(link *tracker.IssueLink) string {
	if link.Type == nil || link.Direction == nil {
		return "-"
	}

	switch api.DerefString(link.Direction, "") {
	case "inward":
		return api.DerefString(link.Type.Inward, "-")
	case "outward":
		return api.DerefString(link.Type.Outward, "-")
	default:
		return api.DerefFlexString(link.Type.ID, "-")
	}
}

func toLinkItem(link *tracker.IssueLink) linkItem {
	item := linkItem{
		ID:   api.DerefFlexString(link.ID, ""),
		Type: linkTypeDisplay(link),
	}

	if link.Object != nil {
		item.Issue = api.DerefString(link.Object.Key, "")
		item.Summary = api.DerefString(link.Object.Summary, "")
	}

	return item
}

func newListCmd() *cobra.Command {
	return runner.List[*tracker.IssueLink, linkItem]{
		Use:   "list ISSUE-KEY",
		Short: "List links on an issue",
		Long:  `List all links on a Yandex Tracker issue.`,
		SeeAlso: `  ytr link create  - Create a link to another issue
  ytr link delete  - Delete a link`,
		Example: `  # List links on an issue
  ytr link list PROJ-123

  # Get links as JSON
  ytr link list PROJ-123 --json id,type,issue

  # Extract link types with jq
  ytr link list PROJ-123 --json type --jq '.[].type'`,
		Args:  []runner.Arg{runner.IssueKey},
		Empty: "No links found",
		Call: func(ctx context.Context, c *tracker.Client, args []string) ([]*tracker.IssueLink, error) {
			links, _, err := c.Issues.GetLinks(ctx, args[0])
			return links, err
		},
		Item:   toLinkItem,
		Header: []string{"ID", "TYPE", "ISSUE", "SUMMARY"},
		Row: func(_ *output.Options, link *tracker.IssueLink) []string {
			item := toLinkItem(link)
			return []string{item.ID, item.Type, item.Issue, item.Summary}
		},
		Quiet: func(link *tracker.IssueLink) string { return api.DerefFlexString(link.ID, "") },
	}.Command()
}
