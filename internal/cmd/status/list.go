package status

import (
	"context"

	"github.com/slavkluev/go-yandex-tracker/tracker"
	"github.com/spf13/cobra"

	"github.com/slavkluev/ytr/internal/api"
	"github.com/slavkluev/ytr/internal/cmd/runner"
	"github.com/slavkluev/ytr/internal/output"
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

  # Get statuses as JSON
  ytr status list --json id,key,name`,
		Empty: "No statuses found",
		Call: func(ctx context.Context, c *tracker.Client, _ []string) ([]*tracker.Status, error) {
			return runner.Collect(c.Statuses.ListIter(ctx, nil))
		},
		Item:   toItem,
		Header: []string{"ID", "KEY", "NAME"},
		Row:    row,
		Quiet:  func(s *tracker.Status) string { return api.DerefString(s.Key, "") },
	}.Command()
}

func toItem(s *tracker.Status) item {
	return item{
		ID:   api.DerefFlexString(s.ID, ""),
		Key:  api.DerefString(s.Key, ""),
		Name: api.DerefString(s.Name, ""),
	}
}

func row(_ *output.Options, s *tracker.Status) []string {
	return []string{
		api.DerefFlexString(s.ID, "-"),
		api.DerefString(s.Key, "-"),
		api.DerefString(s.Name, "-"),
	}
}
