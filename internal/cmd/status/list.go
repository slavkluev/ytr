package status

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
	return runner.List[*tracker.Status, item]{
		Use:   "list",
		Short: "List workflow statuses",
		Long:  `List all workflow statuses in Yandex Tracker.`,
		Example: `  # List all statuses
  ytr status list

  # Only IDs, keys and names
  ytr status list --json id,key,name`,
		Call: func(ctx context.Context, c *tracker.Client, _ []string) ([]*tracker.Status, error) {
			return runner.Collect(c.Statuses.ListIter(ctx, nil))
		},
		Item: toItem,
	}.Command()
}

func toItem(s *tracker.Status) item {
	return item{
		ID:   api.DerefFlexString(s.ID, ""),
		Key:  api.DerefString(s.Key, ""),
		Name: api.DerefString(s.Name, ""),
	}
}
