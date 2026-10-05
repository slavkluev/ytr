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
		Long:  `List all issue types in Yandex Tracker.`,
		Example: `  # List all issue types
  ytr issuetype list

  # Only IDs, keys and names
  ytr issuetype list --json id,key,name`,
		Call: func(ctx context.Context, c *tracker.Client, _ []string) ([]*tracker.IssueType, error) {
			return runner.Collect(c.IssueTypes.ListIter(ctx, nil))
		},
		Item: toItem,
	}.Command()
}

func toItem(it *tracker.IssueType) item {
	return item{
		ID:   api.DerefFlexString(it.ID, ""),
		Key:  api.DerefString(it.Key, ""),
		Name: api.DerefString(it.Name, ""),
	}
}
