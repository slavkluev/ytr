package issuetype

import (
	"context"

	"github.com/slavkluev/go-yandex-tracker/tracker"
	"github.com/spf13/cobra"

	"github.com/slavkluev/ytr/internal/api"
	"github.com/slavkluev/ytr/internal/cmd/runner"
)

type item struct {
	ID   string `json:"id"`
	Key  string `json:"key"`
	Name string `json:"name"`
}

func newListCmd() *cobra.Command {
	return runner.List[*tracker.IssueType, item]{
		Use:   "list",
		Short: "List issue types",
		Long: `List all issue types in Yandex Tracker.

JSON FIELDS
  id, key, name

SEE ALSO
  ytr status list      - List workflow statuses
  ytr priority list    - List priorities
  ytr resolution list  - List resolutions`,
		Example: `  # List all issue types
  ytr issuetype list

  # Get issue types as JSON
  ytr issuetype list --json id,key,name`,
		Empty: "No issue types found",
		Call: func(ctx context.Context, c *tracker.Client) ([]*tracker.IssueType, error) {
			issueTypes, _, err := c.IssueTypes.List(ctx, nil)
			return issueTypes, err
		},
		Item:   toItem,
		Header: []string{"ID", "KEY", "NAME"},
		Row:    row,
		Quiet:  func(it *tracker.IssueType) string { return api.DerefString(it.Key, "") },
	}.Command()
}

func toItem(it *tracker.IssueType) item {
	return item{
		ID:   api.DerefFlexString(it.ID, ""),
		Key:  api.DerefString(it.Key, ""),
		Name: api.DerefString(it.Name, ""),
	}
}

func row(it *tracker.IssueType) []string {
	return []string{
		api.DerefFlexString(it.ID, "-"),
		api.DerefString(it.Key, "-"),
		api.DerefString(it.Name, "-"),
	}
}
