package field

import (
	"context"
	"fmt"
	"sort"
	"strings"

	"github.com/slavkluev/go-yandex-tracker/tracker"
	"github.com/spf13/cobra"

	"github.com/slavkluev/ytr/internal/api"
	"github.com/slavkluev/ytr/internal/cmd/runner"
	"github.com/slavkluev/ytr/internal/output"
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

  # Get specific fields as JSON
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
		Item:   toFieldDetail,
		Detail: fieldCard,
		Quiet:  func(f *tracker.Field) string { return api.DerefString(f.Key, "") },
	}.Command()

	cmd.Flags().StringVar(&queue, "queue", "", "Queue key for local fields")

	return cmd
}

func fieldCard(d *output.DetailPrinter, _ *output.Options, field *tracker.Field) {
	d.Field("ID", api.DerefFlexString(field.ID, "-"))
	d.Field("Key", api.DerefString(field.Key, "-"))
	d.Field("Name", api.DerefString(field.Name, "-"))
	d.Field("Type", api.DerefString(field.Type, "-"))
	d.Field("Schema", formatSchema(field))
	d.Field("Readonly", formatBoolYesNo(api.DerefBool(field.Readonly, false)))

	renderOptionalFields(d, field)

	renderDescription(d, field)
}

func formatSchema(field *tracker.Field) string {
	if field.Schema == nil {
		return "-"
	}
	display := api.DerefString(field.Schema.Type, "-")
	if items := api.DerefString(field.Schema.Items, ""); items != "" {
		display += " of " + items
	}
	if api.DerefBool(field.Schema.Required, false) {
		display += " (required)"
	}
	return display
}

func formatBoolYesNo(val bool) string {
	if val {
		return "yes"
	}
	return "no"
}

func renderOptionalFields(d *output.DetailPrinter, field *tracker.Field) {
	if field.Category != nil {
		if display := api.DerefString(field.Category.Display, ""); display != "" {
			d.Field("Category", display)
		}
	}

	if field.Queue != nil {
		if queueKey := api.DerefString(field.Queue.Key, ""); queueKey != "" {
			d.Field("Queue", queueKey)
		}
	}

	if p := field.OptionsProvider; p != nil {
		renderOptions(d, p)
	}
}

func renderOptions(d *output.DetailPrinter, p *tracker.OptionsProvider) {
	if len(p.Values) > 0 {
		d.Field("Options", joinOptions(p.Values))
	}

	queues := make([]string, 0, len(p.QueueValues))
	for queue := range p.QueueValues {
		queues = append(queues, queue)
	}
	sort.Strings(queues)
	for _, queue := range queues {
		d.Field("Options ("+queue+")", joinOptions(p.QueueValues[queue]))
	}

	if len(p.Defaults) > 0 {
		d.Field("Default options", joinOptions(p.Defaults))
	}
}

func joinOptions(values []any) string {
	parts := make([]string, len(values))
	for i, v := range values {
		parts[i] = fmt.Sprint(v)
	}
	return strings.Join(parts, ", ")
}

func renderDescription(d *output.DetailPrinter, field *tracker.Field) {
	if field.Description == nil || *field.Description == "" {
		return
	}
	d.Block("Description", *field.Description)
}
