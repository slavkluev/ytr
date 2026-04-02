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
var FieldListFields = []string{"key", "name", "schema", "readonly"}

// fieldItem is a clean struct for JSON serialization of field data.
type fieldItem struct {
	Key      string `json:"key"`
	Name     string `json:"name"`
	Schema   string `json:"schema"`
	Readonly bool   `json:"readonly"`
}

// toFieldItem converts a tracker.Field to a clean JSON-serializable struct.
func toFieldItem(f *tracker.Field) fieldItem {
	schema := "-"
	if f.Schema != nil {
		schema = api.DerefString(f.Schema.Type, "-")
	}

	return fieldItem{
		Key:      api.DerefString(f.Key, ""),
		Name:     api.DerefString(f.Name, ""),
		Schema:   schema,
		Readonly: api.DerefBool(f.Readonly, false),
	}
}

// newListCmd creates the "field list" command.
func newListCmd() *cobra.Command {
	var queueFlag string

	cmd := &cobra.Command{
		Use:   "list",
		Short: "List available fields",
		Long: `List available fields in Yandex Tracker.

When --queue is specified, shows queue-local fields instead of global fields.

JSON FIELDS
  key, name, schema, readonly

SEE ALSO
  ytr field get  - Show field details`,
		Example: `  # List all global fields
  ytr field list

  # List queue-local fields
  ytr field list --queue PROJ

  # Get fields as JSON
  ytr field list --json key,name,schema`,
		Args: cobra.NoArgs,
		RunE: func(cmd *cobra.Command, _ []string) error {
			return runList(cmd, queueFlag)
		},
	}

	cmd.Flags().StringVar(&queueFlag, "queue", "", "Queue key for local fields")

	jsonfields.Register("ytr field list", FieldListFields)

	return cmd
}

// runList executes the field list logic.
func runList(cmd *cobra.Command, queueFlag string) error {
	// Handle --json field hint (no fields specified, D-10).
	if output.IsJSON() && !output.HasFieldSelection() && output.JQFilter == "" {
		return output.PrintFieldHint(cmd.ErrOrStderr(), "field list", FieldListFields)
	}

	// If --jq without --json: auto-populate all fields (Pitfall 5).
	if output.JQFilter != "" && !output.HasFieldSelection() {
		output.JSONFields = FieldListFields
	}

	// Validate requested fields.
	if output.HasFieldSelection() {
		if err := output.ValidateFields(output.JSONFields, FieldListFields); err != nil {
			return err
		}
		output.JSONFields = output.NormalizeFields(output.JSONFields, FieldListFields)
	}

	// Resolve auth from root persistent flags.
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

	return renderOutput(cmd.OutOrStdout(), fields)
}

// renderOutput handles JSON/quiet/table output for the field list result.
func renderOutput(w io.Writer, fields []*tracker.Field) error {
	if output.IsJSON() {
		return renderJSON(w, fields)
	}

	if output.IsQuiet() {
		keys := make([]string, len(fields))
		for i, f := range fields {
			keys[i] = api.DerefString(f.Key, "")
		}
		output.PrintQuiet(w, keys...)
		return nil
	}

	return renderTable(w, fields)
}

// renderJSON renders the field list as JSON with field selection and JQ support.
func renderJSON(w io.Writer, fields []*tracker.Field) error {
	items := make([]fieldItem, len(fields))
	for i, f := range fields {
		items[i] = toFieldItem(f)
	}

	if output.HasFieldSelection() {
		filtered := make([]map[string]any, len(items))
		for i, item := range items {
			filtered[i] = output.FilterFields(item, output.JSONFields)
		}
		if output.JQFilter != "" {
			return output.ApplyJQ(w, filtered, output.JQFilter)
		}
		return output.PrintJSON(w, filtered)
	}
	if output.JQFilter != "" {
		return output.ApplyJQ(w, items, output.JQFilter)
	}
	return output.PrintJSON(w, items)
}

// renderTable renders the field list as a formatted table.
func renderTable(w io.Writer, fields []*tracker.Field) error {
	if len(fields) == 0 {
		_, err := fmt.Fprintln(w, "No fields found")
		return err
	}

	tbl := output.NewTable(w)
	tbl.AddHeader("KEY", "NAME", "SCHEMA", "READONLY")

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
			api.DerefString(f.Key, "-"),
			api.DerefString(f.Name, "-"),
			schema,
			readonly,
		)
	}

	tbl.Render()
	return nil
}
