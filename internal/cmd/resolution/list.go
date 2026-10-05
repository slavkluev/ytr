package resolution

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
	return runner.List[*tracker.Resolution, item]{
		Use:   "list",
		Short: "List resolutions",
		Long:  `List all resolutions in Yandex Tracker.`,
		Example: `  # List all resolutions
  ytr resolution list

  # Only IDs, keys and names
  ytr resolution list --json id,key,name`,
		Call: func(ctx context.Context, c *tracker.Client, _ []string) ([]*tracker.Resolution, error) {
			return runner.Collect(c.Resolutions.ListIter(ctx, nil))
		},
		Item: toItem,
	}.Command()
}

func toItem(r *tracker.Resolution) item {
	return item{
		ID:   api.DerefFlexString(r.ID, ""),
		Key:  api.DerefString(r.Key, ""),
		Name: api.DerefString(r.Name, ""),
	}
}
