package priority

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
	return runner.List[*tracker.Priority, item]{
		Use:   "list",
		Short: "List priorities",
		Long:  `List all priorities in Yandex Tracker.`,
		Example: `  # List all priorities
  ytr priority list

  # Get priorities as JSON
  ytr priority list --json id,key,name`,
		Empty: "No priorities found",
		Call: func(ctx context.Context, c *tracker.Client, _ []string) ([]*tracker.Priority, error) {
			return runner.Collect(c.Priorities.ListIter(ctx, nil))
		},
		Item:   toItem,
		Header: []string{"ID", "KEY", "NAME"},
		Row:    row,
		Quiet:  func(p *tracker.Priority) string { return api.DerefString(p.Key, "") },
	}.Command()
}

func toItem(p *tracker.Priority) item {
	return item{
		ID:   api.DerefFlexString(p.ID, ""),
		Key:  api.DerefString(p.Key, ""),
		Name: api.DerefString(p.Name, ""),
	}
}

func row(_ *output.Options, p *tracker.Priority) []string {
	return []string{
		api.DerefFlexString(p.ID, "-"),
		api.DerefString(p.Key, "-"),
		api.DerefString(p.Name, "-"),
	}
}
