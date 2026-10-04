package queue

import (
	"context"
	"iter"

	"github.com/slavkluev/go-yandex-tracker/tracker"
	"github.com/spf13/cobra"

	"github.com/slavkluev/ytr/internal/api"
	"github.com/slavkluev/ytr/internal/cmd/runner"
	"github.com/slavkluev/ytr/internal/output"
)

// Raw tracker.Queue fields are pointer types that produce nulls in JSON;
// this struct uses value types with proper json tags.
type queueItem struct {
	Key    string `json:"key"`
	Name   string `json:"name"`
	Lead   string `json:"lead,omitempty"`
	LeadID string `json:"leadId"`
}

func newListCmd() *cobra.Command {
	return runner.Pages[*tracker.Queue, queueItem]{
		Use:   "list",
		Short: "List queues",
		Long:  `List Yandex Tracker queues with pagination.`,
		Example: `  # List all queues
  ytr queue list

  # List queues as JSON
  ytr queue list --json key,name

  # Get all queue keys
  ytr queue list --all --json key --jq '.items[].key'`,
		Empty: "No queues found",
		Page: func(ctx context.Context, c *tracker.Client, o tracker.ListOptions) (
			[]*tracker.Queue, *tracker.Response, error,
		) {
			return c.Queues.List(ctx, &tracker.QueueListOptions{ListOptions: o})
		},
		All: func(ctx context.Context, c *tracker.Client, o tracker.ListOptions) iter.Seq2[*tracker.Queue, error] {
			return c.Queues.ListIter(ctx, &tracker.QueueListOptions{ListOptions: o})
		},
		Item:   toQueueItem,
		Header: []string{"KEY", "NAME", "LEAD"},
		Row:    queueRow,
		Quiet:  func(q *tracker.Queue) string { return api.DerefString(q.Key, "") },
	}.Command()
}

func queueRow(_ *output.Options, q *tracker.Queue) []string {
	return []string{api.DerefString(q.Key, "-"), api.DerefString(q.Name, "-"), q.Lead.DisplayOr("-")}
}

func toQueueItem(q *tracker.Queue) queueItem {
	return queueItem{
		Key:    api.DerefString(q.Key, ""),
		Name:   api.DerefString(q.Name, ""),
		Lead:   q.Lead.DisplayOr(""),
		LeadID: q.Lead.IDOr(""),
	}
}
