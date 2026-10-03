package field

import (
	"fmt"
	"io"

	"github.com/slavkluev/go-yandex-tracker/tracker"
	"github.com/spf13/cobra"

	"github.com/slavkluev/ytr/internal/api"
	"github.com/slavkluev/ytr/internal/cmd/jsonfields"
	"github.com/slavkluev/ytr/internal/config"
	"github.com/slavkluev/ytr/internal/output"
)

// FieldListFields lists the available JSON field names for field list output.
var FieldListFields = []string{
	"id",
	"key",
	"name",
	"schema",
	"items",
	"readonly",
	"options",
	"queueOptions",
	"defaultOptions",
}

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

// Schema is left empty (and omitted from JSON) when the field has no schema
// type, matching `field get`; the "-" placeholder is a table-only convention.
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
	var queueFlag string

	cmd := &cobra.Command{
		Use:   "list",
		Short: "List available fields",
		Long: `List available fields in Yandex Tracker.

When --queue is specified, shows queue-local fields instead of global fields.

options lists a field's allowed values in the JSON type Tracker sent, so
numeric options stay numbers. When Tracker sets the values per queue, options
is omitted: queueOptions maps each queue key to its list, and defaultOptions
holds Tracker's defaults list.

JSON FIELDS
  id, key, name, schema, items, readonly, options, queueOptions, defaultOptions

SEE ALSO
  ytr field get  - Show field details`,
		Example: `  # List all global fields
  ytr field list

  # List queue-local fields
  ytr field list --queue PROJ

  # Get fields as JSON (id is the full field id needed to write the field)
  ytr field list --queue PROJ --json id,key,name,schema,options`,
		Args: cobra.NoArgs,
		RunE: func(cmd *cobra.Command, _ []string) error {
			return runList(cmd, queueFlag)
		},
	}

	cmd.Flags().StringVar(&queueFlag, "queue", "", "Queue key for local fields")

	jsonfields.Register("ytr field list", FieldListFields)

	return cmd
}

func runList(cmd *cobra.Command, queueFlag string) error {
	opts := output.FromContext(cmd.Context())

	if opts.WantsFieldHint(cmd.Flags().Changed("json")) {
		return output.PrintFieldHint(cmd.ErrOrStderr(), "field list", FieldListFields)
	}

	if opts.JQFilter != "" && !opts.HasFieldSelection() {
		opts.JSONFields = FieldListFields
	}

	if opts.HasFieldSelection() {
		if err := output.ValidateFields(opts.JSONFields, FieldListFields); err != nil {
			return err
		}
		opts.JSONFields = output.NormalizeFields(opts.JSONFields, FieldListFields)
	}

	tokenFlag, _ := cmd.Root().PersistentFlags().GetString("token")
	orgIDFlag, _ := cmd.Root().PersistentFlags().GetString("org-id")
	orgTypeFlag, _ := cmd.Root().PersistentFlags().GetString("org-type")

	auth, err := config.ResolveAuth(tokenFlag, orgIDFlag, orgTypeFlag)
	if err != nil {
		return err
	}

	lister := newFieldLister(auth)

	var fields []*tracker.Field
	if queueFlag != "" {
		fields, _, err = lister.ListLocal(cmd.Context(), queueFlag)
	} else {
		fields, _, err = lister.List(cmd.Context())
	}
	if err != nil {
		return api.MapAPIError(err)
	}

	return renderOutput(cmd.OutOrStdout(), opts, fields)
}

func renderOutput(w io.Writer, opts *output.Options, fields []*tracker.Field) error {
	if opts.IsJSON() {
		return renderJSON(w, opts, fields)
	}

	if opts.Quiet {
		keys := make([]string, len(fields))
		for i, f := range fields {
			keys[i] = api.DerefString(f.Key, "")
		}
		output.PrintQuiet(w, keys...)
		return nil
	}

	return renderTable(w, opts, fields)
}

func renderJSON(w io.Writer, opts *output.Options, fields []*tracker.Field) error {
	items := make([]fieldItem, len(fields))
	for i, f := range fields {
		items[i] = toFieldItem(f)
	}

	if opts.HasFieldSelection() {
		filtered := make([]map[string]any, len(items))
		for i, item := range items {
			filtered[i] = output.FilterFields(item, opts.JSONFields)
		}
		if opts.JQFilter != "" {
			return output.ApplyJQ(w, filtered, opts.JQFilter)
		}
		return opts.PrintJSON(w, filtered)
	}
	if opts.JQFilter != "" {
		return output.ApplyJQ(w, items, opts.JQFilter)
	}
	return opts.PrintJSON(w, items)
}

func renderTable(w io.Writer, opts *output.Options, fields []*tracker.Field) error {
	if len(fields) == 0 {
		_, err := fmt.Fprintln(w, "No fields found")
		return err
	}

	tbl := opts.NewTable(w)
	tbl.AddHeader("ID", "KEY", "NAME", "SCHEMA", "READONLY")

	for _, f := range fields {
		schema := "-"
		if f.Schema != nil {
			schema = api.DerefString(f.Schema.Type, "-")
		}

		readonly := "no"
		if api.DerefBool(f.Readonly, false) {
			readonly = "yes"
		}

		tbl.AddRow(
			api.DerefFlexString(f.ID, "-"),
			api.DerefString(f.Key, "-"),
			api.DerefString(f.Name, "-"),
			schema,
			readonly,
		)
	}

	tbl.Render()
	return nil
}
