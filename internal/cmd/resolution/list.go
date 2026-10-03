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
		Long: `List all resolutions in Yandex Tracker.

JSON FIELDS
  id, key, name

SEE ALSO
  ytr status list    - List workflow statuses
  ytr priority list  - List priorities
  ytr issuetype list - List issue types`,
		Example: `  # List all resolutions
  ytr resolution list

  # Get resolutions as JSON
  ytr resolution list --json id,key,name`,
		Empty: "No resolutions found",
		Call: func(ctx context.Context, c *tracker.Client) ([]*tracker.Resolution, error) {
			resolutions, _, err := c.Resolutions.List(ctx)
			return resolutions, err
		},
		Item:   toItem,
		Header: []string{"ID", "KEY", "NAME"},
		Row:    row,
		Quiet:  func(r *tracker.Resolution) string { return api.DerefString(r.Key, "") },
	}.Command()
}

func toItem(r *tracker.Resolution) item {
	return item{
		ID:   api.DerefFlexString(r.ID, ""),
		Key:  api.DerefString(r.Key, ""),
		Name: api.DerefString(r.Name, ""),
	}
}

func row(r *tracker.Resolution) []string {
	return []string{
		api.DerefFlexString(r.ID, "-"),
		api.DerefString(r.Key, "-"),
		api.DerefString(r.Name, "-"),
	}
}
