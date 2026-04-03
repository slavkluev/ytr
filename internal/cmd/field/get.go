package field

import (
	"fmt"
	"io"
	"strings"

	"github.com/jedib0t/go-pretty/v6/text"
	"github.com/slavkluev/go-yandex-tracker/tracker"
	"github.com/spf13/cobra"

	"github.com/slavkluev/ytr/internal/api"
	"github.com/slavkluev/ytr/internal/cmd/jsonfields"
	"github.com/slavkluev/ytr/internal/config"
	"github.com/slavkluev/ytr/internal/output"
)

// FieldGetFields lists the available JSON field names for field get output.
var FieldGetFields = []string{
	"key",
	"name",
	"type",
	"schema",
	"required",
	"readonly",
	"category",
	"queue",
	"options",
	"description",
}

// fieldDetail is a clean struct for JSON serialization of a single field.
type fieldDetail struct {
	Key         string   `json:"key"`
	Name        string   `json:"name"`
	Type        string   `json:"type,omitempty"`
	Schema      string   `json:"schema,omitempty"`
	Required    bool     `json:"required"`
	Readonly    bool     `json:"readonly"`
	Category    string   `json:"category,omitempty"`
	Queue       string   `json:"queue,omitempty"`
	Options     []string `json:"options,omitempty"`
	Description string   `json:"description,omitempty"`
}

// toFieldDetail converts a tracker.Field into a clean fieldDetail struct for JSON output.
func toFieldDetail(f *tracker.Field) fieldDetail {
	detail := fieldDetail{
		Key:      api.DerefString(f.Key, ""),
		Name:     api.DerefString(f.Name, ""),
		Type:     api.DerefString(f.Type, ""),
		Readonly: api.DerefBool(f.Readonly, false),
	}

	if f.Schema != nil {
		detail.Schema = api.DerefString(f.Schema.Type, "")
		detail.Required = api.DerefBool(f.Schema.Required, false)
	}

	if f.Category != nil {
		detail.Category = api.DerefString(f.Category.Display, "")
	}

	if f.Queue != nil {
		detail.Queue = api.DerefString(f.Queue.Key, "")
	}

	if f.OptionsProvider != nil && len(f.OptionsProvider.Values) > 0 {
		detail.Options = f.OptionsProvider.Values
	}

	detail.Description = api.DerefString(f.Description, "")

	return detail
}

// newGetCmd creates the "field get" command for displaying field details.
func newGetCmd() *cobra.Command {
	var queueFlag string

	cmd := &cobra.Command{
		Use:   "get FIELD-KEY",
		Short: "Show field details",
		Long: `Display detailed information about a Yandex Tracker field.

When --queue is specified, retrieves a queue-local field instead of a global field.

JSON FIELDS
  key, name, type, schema, required, readonly, category, queue, options, description

SEE ALSO
  ytr field list  - List available fields`,
		Example: `  # View global field details
  ytr field get summary

  # View local field details
  ytr field get custom-field --queue PROJ

  # Get specific fields as JSON
  ytr field get priority --json key,name,options`,
		Args: cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			return runGet(cmd, args[0], queueFlag)
		},
	}

	cmd.Flags().StringVar(&queueFlag, "queue", "", "Queue key for local fields")

	jsonfields.Register("ytr field get", FieldGetFields)

	return cmd
}

// runGet executes the field get logic.
func runGet(cmd *cobra.Command, fieldKey, queueFlag string) error {
	if output.IsJSON() && !output.HasFieldSelection() && output.JQFilter == "" {
		return output.PrintFieldHint(cmd.ErrOrStderr(), "field get", FieldGetFields)
	}

	if output.JQFilter != "" && !output.HasFieldSelection() {
		output.JSONFields = FieldGetFields
	}

	// Validate requested fields.
	if output.HasFieldSelection() {
		if err := output.ValidateFields(output.JSONFields, FieldGetFields); err != nil {
			return err
		}
		output.JSONFields = output.NormalizeFields(output.JSONFields, FieldGetFields)
	}

	// Resolve auth from root persistent flags.
	tokenFlag, _ := cmd.Root().PersistentFlags().GetString("token")
	orgIDFlag, _ := cmd.Root().PersistentFlags().GetString("org-id")
	orgTypeFlag, _ := cmd.Root().PersistentFlags().GetString("org-type")

	auth, err := config.ResolveAuth(tokenFlag, orgIDFlag, orgTypeFlag)
	if err != nil {
		return err
	}

	getter := newFieldGetter(auth)

	var field *tracker.Field
	if queueFlag != "" {
		field, _, err = getter.GetLocal(cmd.Context(), queueFlag, fieldKey)
	} else {
		field, _, err = getter.Get(cmd.Context(), fieldKey)
	}
	if err != nil {
		return api.MapAPIError(err)
	}

	w := cmd.OutOrStdout()

	if output.IsJSON() {
		detail := toFieldDetail(field)

		if output.HasFieldSelection() {
			filtered := output.FilterFields(detail, output.JSONFields)
			if output.JQFilter != "" {
				return output.ApplyJQ(w, filtered, output.JQFilter)
			}
			return output.PrintJSON(w, filtered)
		}
		if output.JQFilter != "" {
			return output.ApplyJQ(w, detail, output.JQFilter)
		}
		return output.PrintJSON(w, detail)
	}

	if output.IsQuiet() {
		output.PrintQuiet(w, api.DerefString(field.Key, ""))
		return nil
	}

	return renderFieldCard(w, field)
}

// renderFieldCard renders a bold-label detail card for a field.
func renderFieldCard(w io.Writer, field *tracker.Field) error {
	bold := func(label string) string {
		if output.ColorsEnabled() {
			return text.Colors{text.Bold}.Sprint(label)
		}
		return label
	}

	// writeErr captures the first write error encountered.
	var writeErr error
	printField := func(label, value string) {
		if writeErr != nil {
			return
		}
		_, writeErr = fmt.Fprintf(w, "%s  %s\n", bold(label+":"), value)
	}

	printField("Key", api.DerefString(field.Key, "-"))
	printField("Name", api.DerefString(field.Name, "-"))
	printField("Type", api.DerefString(field.Type, "-"))
	printField("Schema", formatSchema(field))
	printField("Readonly", formatBoolYesNo(api.DerefBool(field.Readonly, false)))

	// Optional fields: category, queue, options.
	renderOptionalFields(printField, field)

	if writeErr != nil {
		return writeErr
	}

	return renderDescription(w, bold, field)
}

// formatSchema returns the schema display string with required indicator.
func formatSchema(field *tracker.Field) string {
	if field.Schema == nil {
		return "-"
	}
	display := api.DerefString(field.Schema.Type, "-")
	if api.DerefBool(field.Schema.Required, false) {
		display += " (required)"
	}
	return display
}

// formatBoolYesNo returns "yes" for true and "no" for false.
func formatBoolYesNo(val bool) string {
	if val {
		return "yes"
	}
	return "no"
}

// renderOptionalFields prints category, queue, and options if present.
func renderOptionalFields(printField func(string, string), field *tracker.Field) {
	if field.Category != nil {
		if display := api.DerefString(field.Category.Display, ""); display != "" {
			printField("Category", display)
		}
	}

	if field.Queue != nil {
		if queueKey := api.DerefString(field.Queue.Key, ""); queueKey != "" {
			printField("Queue", queueKey)
		}
	}

	if field.OptionsProvider != nil && len(field.OptionsProvider.Values) > 0 {
		printField("Options", strings.Join(field.OptionsProvider.Values, ", "))
	}
}

// renderDescription prints the field description with a separator if present.
func renderDescription(w io.Writer, bold func(string) string, field *tracker.Field) error {
	if field.Description == nil || *field.Description == "" {
		return nil
	}
	if _, err := fmt.Fprintln(w); err != nil {
		return err
	}
	if _, err := fmt.Fprintf(w, "%s\n", bold("Description:")); err != nil {
		return err
	}
	_, err := fmt.Fprintf(w, "  %s\n", *field.Description)
	return err
}
