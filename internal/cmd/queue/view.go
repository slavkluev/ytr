package queue

import (
	"context"

	"github.com/slavkluev/go-yandex-tracker/tracker"
	"github.com/spf13/cobra"

	"github.com/slavkluev/ytr/internal/api"
	"github.com/slavkluev/ytr/internal/cmd/runner"
	"github.com/slavkluev/ytr/internal/output"
)

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
		SeeAlso: `  ytr queue list    - List queues
  ytr issue list    - List issues in a queue`,
		Example: `  # View queue details
  ytr queue view PROJ

  # Get queue config as JSON
  ytr queue view PROJ --json key,name,lead,defaultType`,
		Args: []runner.Arg{runner.AnyArg},
		Call: func(ctx context.Context, c *tracker.Client, args []string) (*tracker.Queue, error) {
			q, _, err := c.Queues.Get(ctx, args[0], nil)
			return q, err
		},
		Item:   toQueueDetail,
		Detail: queueCard,
		Quiet:  func(q *tracker.Queue) string { return api.DerefString(q.Key, "") },
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

func queueCard(d *output.DetailPrinter, _ *output.Options, q *tracker.Queue) {
	d.Field("Key", api.DerefString(q.Key, "-"))
	d.Field("Name", api.DerefString(q.Name, "-"))
	d.Field("Lead", q.Lead.DisplayOr("-"))
	d.Field("Default Type", orDash(derefIssueType(q.DefaultType)))
	d.Field("Default Priority", orDash(derefPriority(q.DefaultPriority)))

	if q.Description != nil && *q.Description != "" {
		d.Block("Description", *q.Description)
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

func orDash(value string) string {
	if value == "" {
		return "-"
	}

	return value
}
