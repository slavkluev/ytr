package queue

import (
	"context"

	"github.com/slavkluev/go-yandex-tracker/tracker"
	"github.com/spf13/cobra"

	"github.com/slavkluev/ytr/internal/api"
	"github.com/slavkluev/ytr/internal/cmd/runner"
)

// Uses value types with json tags to avoid null fields from pointer types.
type queueDetail struct {
	Key             string `json:"key"`
	Name            string `json:"name"`
	Description     string `json:"description,omitempty"`
	Lead            string `json:"lead,omitempty"`
	LeadID          string `json:"leadId"`
	DefaultType     string `json:"defaultType,omitempty"`
	DefaultPriority string `json:"defaultPriority,omitempty"`
	AssignAuto      bool   `json:"assignAuto"`
	AllowExternals  bool   `json:"allowExternals"`
}

func newViewCmd() *cobra.Command {
	return runner.Get[*tracker.Queue, queueDetail]{
		Use:   "view QUEUE-KEY",
		Short: "View queue details",
		Long:  `Display detailed information about a Yandex Tracker queue.`,
		Example: `  # View queue details
  ytr queue view PROJ

  # Only the key, name, lead and default type
  ytr queue view PROJ --json key,name,lead,defaultType`,
		Args: []runner.Arg{runner.StringID("queue key")},
		Call: func(ctx context.Context, c *tracker.Client, args []string) (*tracker.Queue, error) {
			q, _, err := c.Queues.Get(ctx, args[0], nil)
			return q, err
		},
		Item: toQueueDetail,
	}.Command()
}

func toQueueDetail(q *tracker.Queue) queueDetail {
	return queueDetail{
		Key:             api.DerefString(q.Key, ""),
		Name:            api.DerefString(q.Name, ""),
		Description:     api.DerefString(q.Description, ""),
		Lead:            q.Lead.DisplayOr(""),
		LeadID:          q.Lead.IDOr(""),
		DefaultType:     derefIssueType(q.DefaultType),
		DefaultPriority: derefPriority(q.DefaultPriority),
		AssignAuto:      api.DerefBool(q.AssignAuto, false),
		AllowExternals:  api.DerefBool(q.AllowExternals, false),
	}
}

func derefIssueType(t *tracker.IssueType) string {
	if t == nil {
		return ""
	}

	return firstSet(t.Display, t.Name, t.Key)
}

func derefPriority(p *tracker.Priority) string {
	if p == nil {
		return ""
	}

	return firstSet(p.Display, p.Name, p.Key)
}

// firstSet returns the first value Tracker sent, even an empty one, so a
// present but empty display still wins over the name.
func firstSet(values ...*string) string {
	for _, v := range values {
		if v != nil {
			return *v
		}
	}

	return ""
}
