package field

import (
	"context"

	"github.com/slavkluev/go-yandex-tracker/tracker"
	"github.com/spf13/cobra"

	"github.com/slavkluev/ytr/internal/api"
	"github.com/slavkluev/ytr/internal/cmd/runner"
)

// Option values are []any so each keeps the JSON type Tracker sent.
type fieldDetail struct {
	ID             string           `json:"id"`
	Key            string           `json:"key"`
	Name           string           `json:"name"`
	Type           string           `json:"type,omitempty"`
	Schema         string           `json:"schema,omitempty"`
	Items          string           `json:"items,omitempty"`
	Required       bool             `json:"required"`
	Readonly       bool             `json:"readonly"`
	Category       string           `json:"category,omitempty"`
	Queue          string           `json:"queue,omitempty"`
	Options        []any            `json:"options,omitempty"`
	QueueOptions   map[string][]any `json:"queueOptions,omitempty"`
	DefaultOptions []any            `json:"defaultOptions,omitempty"`
	Description    string           `json:"description,omitempty"`
}

func toFieldDetail(f *tracker.Field) fieldDetail {
	detail := fieldDetail{
		ID:       api.DerefFlexString(f.ID, ""),
		Key:      api.DerefString(f.Key, ""),
		Name:     api.DerefString(f.Name, ""),
		Type:     api.DerefString(f.Type, ""),
		Readonly: api.DerefBool(f.Readonly, false),
	}

	if f.Schema != nil {
		detail.Schema = api.DerefString(f.Schema.Type, "")
		detail.Items = api.DerefString(f.Schema.Items, "")
		detail.Required = api.DerefBool(f.Schema.Required, false)
	}

	if f.Category != nil {
		detail.Category = api.DerefString(f.Category.Display, "")
	}

	if f.Queue != nil {
		detail.Queue = api.DerefString(f.Queue.Key, "")
	}

	if p := f.OptionsProvider; p != nil {
		detail.Options = p.Values
		detail.QueueOptions = p.QueueValues
		detail.DefaultOptions = p.Defaults
	}

	detail.Description = api.DerefString(f.Description, "")

	return detail
}

func newGetCmd() *cobra.Command {
	var queue string

	cmd := runner.Get[*tracker.Field, fieldDetail]{
		Use:   "get FIELD-KEY",
		Short: "Show field details",
		Long: `Display detailed information about a Yandex Tracker field.

When --queue is specified, retrieves a queue-local field instead of a global field.

options lists the field's allowed values in the JSON type Tracker sent, so
numeric options stay numbers. When Tracker sets the values per queue, options
is omitted: queueOptions maps each queue key to its list, and defaultOptions
holds Tracker's defaults list.`,
		Example: `  # View global field details
  ytr field get summary

  # View local field details
  ytr field get custom-field --queue PROJ

  # Only the ID, key, name, schema and allowed values
  ytr field get priority --json id,key,name,schema,options`,
		Args: []runner.Arg{runner.StringID("field key")},
		Call: func(ctx context.Context, c *tracker.Client, args []string) (*tracker.Field, error) {
			if queue != "" {
				field, _, err := c.Fields.GetLocal(ctx, queue, args[0])
				return field, err
			}

			field, _, err := c.Fields.Get(ctx, args[0])
			return field, err
		},
		Item: toFieldDetail,
	}.Command()

	cmd.Flags().StringVar(&queue, "queue", "", "Queue key for local fields")

	return cmd
}
