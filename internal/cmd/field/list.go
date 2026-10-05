package field

import (
	"context"

	"github.com/slavkluev/go-yandex-tracker/tracker"
	"github.com/spf13/cobra"

	"github.com/slavkluev/ytr/internal/api"
	"github.com/slavkluev/ytr/internal/cmd/runner"
)

// Option values are []any so each keeps the JSON type Tracker sent.
type fieldItem struct {
	ID             string           `json:"id"`
	Key            string           `json:"key"`
	Name           string           `json:"name"`
	Schema         string           `json:"schema,omitempty"`
	Items          string           `json:"items,omitempty"`
	Readonly       bool             `json:"readonly"`
	Options        []any            `json:"options,omitempty"`
	QueueOptions   map[string][]any `json:"queueOptions,omitempty"`
	DefaultOptions []any            `json:"defaultOptions,omitempty"`
}

// Schema is left empty, and so omitted, when the field has no schema type,
// matching `field get`.
func toFieldItem(f *tracker.Field) fieldItem {
	schema, schemaItems := "", ""
	if f.Schema != nil {
		schema = api.DerefString(f.Schema.Type, "")
		schemaItems = api.DerefString(f.Schema.Items, "")
	}

	item := fieldItem{
		ID:       api.DerefFlexString(f.ID, ""),
		Key:      api.DerefString(f.Key, ""),
		Name:     api.DerefString(f.Name, ""),
		Schema:   schema,
		Items:    schemaItems,
		Readonly: api.DerefBool(f.Readonly, false),
	}

	if p := f.OptionsProvider; p != nil {
		item.Options = p.Values
		item.QueueOptions = p.QueueValues
		item.DefaultOptions = p.Defaults
	}

	return item
}

func newListCmd() *cobra.Command {
	var queue string

	cmd := runner.List[*tracker.Field, fieldItem]{
		Use:   "list",
		Short: "List available fields",
		Long: `List available fields in Yandex Tracker.

When --queue is specified, shows queue-local fields instead of global fields.

options lists a field's allowed values in the JSON type Tracker sent, so
numeric options stay numbers. When Tracker sets the values per queue, options
is omitted: queueOptions maps each queue key to its list, and defaultOptions
holds Tracker's defaults list.`,
		Example: `  # List all global fields
  ytr field list

  # List queue-local fields
  ytr field list --queue PROJ

  # Only IDs, keys, names, schemas and allowed values (id is the full field id needed to write the field)
  ytr field list --queue PROJ --json id,key,name,schema,options`,
		Call: func(ctx context.Context, c *tracker.Client, _ []string) ([]*tracker.Field, error) {
			if queue != "" {
				fields, _, err := c.Fields.ListLocal(ctx, queue)
				return fields, err
			}

			fields, _, err := c.Fields.List(ctx)
			return fields, err
		},
		Item: toFieldItem,
	}.Command()

	cmd.Flags().StringVar(&queue, "queue", "", "Queue key for local fields")

	return cmd
}
