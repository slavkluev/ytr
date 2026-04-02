package component

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

// ComponentListFields lists the available JSON field names for component list output.
var ComponentListFields = []string{"id", "name", "queue", "lead", "description", "assignAuto"}

// componentItem is a clean struct for JSON serialization of component data.
type componentItem struct {
	ID          string `json:"id"`
	Name        string `json:"name"`
	Queue       string `json:"queue,omitempty"`
	Lead        string `json:"lead,omitempty"`
	Description string `json:"description,omitempty"`
	AssignAuto  bool   `json:"assignAuto"`
}

// toComponentItem converts a tracker.Component to a clean JSON-serializable struct.
func toComponentItem(c *tracker.Component) componentItem {
	queue := ""
	if c.Queue != nil {
		queue = api.DerefString(c.Queue.Key, "")
	}

	return componentItem{
		ID:          api.DerefFlexString(c.ID, ""),
		Name:        api.DerefString(c.Name, ""),
		Queue:       queue,
		Lead:        api.DerefUser(c.Lead, ""),
		Description: api.DerefString(c.Description, ""),
		AssignAuto:  api.DerefBool(c.AssignAuto, false),
	}
}

// newListCmd creates the "component list" command.
func newListCmd() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "list",
		Short: "List components",
		Long: `List all project components in Yandex Tracker.

JSON FIELDS
  id, name, queue, lead, description, assignAuto

SEE ALSO
  ytr component get     - Show component details
  ytr component create  - Create a component`,
		Example: `  # List all components
  ytr component list

  # Get components as JSON
  ytr component list --json id,name,queue`,
		Args: cobra.NoArgs,
		RunE: func(cmd *cobra.Command, _ []string) error {
			return runList(cmd)
		},
	}

	jsonfields.Register("ytr component list", ComponentListFields)

	return cmd
}

// runList executes the component list logic.
func runList(cmd *cobra.Command) error {
	// Handle --json field hint (no fields specified, D-10).
	if output.IsJSON() && !output.HasFieldSelection() && output.JQFilter == "" {
		return output.PrintFieldHint(cmd.ErrOrStderr(), "component list", ComponentListFields)
	}

	// If --jq without --json: auto-populate all fields (Pitfall 5).
	if output.JQFilter != "" && !output.HasFieldSelection() {
		output.JSONFields = ComponentListFields
	}

	// Validate requested fields.
	if output.HasFieldSelection() {
		if err := output.ValidateFields(output.JSONFields, ComponentListFields); err != nil {
			return err
		}
		output.JSONFields = output.NormalizeFields(output.JSONFields, ComponentListFields)
	}

	// Resolve auth from root persistent flags.
	tokenFlag, _ := cmd.Root().PersistentFlags().GetString("token")
	orgIDFlag, _ := cmd.Root().PersistentFlags().GetString("org-id")
	orgTypeFlag, _ := cmd.Root().PersistentFlags().GetString("org-type")

	auth, err := config.ResolveAuth(tokenFlag, orgIDFlag, orgTypeFlag)
	if err != nil {
		return err
	}

	lister := newComponentLister(auth)

	components, _, err := lister.List(cmd.Context())
	if err != nil {
		return api.MapAPIError(err)
	}

	return renderListOutput(cmd.OutOrStdout(), components)
}

// renderListOutput handles JSON/quiet/table output for the component list result.
func renderListOutput(w io.Writer, components []*tracker.Component) error {
	if output.IsJSON() {
		items := make([]componentItem, len(components))
		for i, c := range components {
			items[i] = toComponentItem(c)
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

	if output.IsQuiet() {
		ids := make([]string, len(components))
		for i, c := range components {
			ids[i] = api.DerefFlexString(c.ID, "")
		}
		output.PrintQuiet(w, ids...)
		return nil
	}

	// Table output.
	if len(components) == 0 {
		_, err := fmt.Fprintln(w, "No components found")
		return err
	}

	tbl := output.NewTable(w)
	tbl.AddHeader("ID", "NAME", "QUEUE", "LEAD")

	for _, c := range components {
		queue := "-"
		if c.Queue != nil {
			queue = api.DerefString(c.Queue.Key, "-")
		}

		tbl.AddRow(
			api.DerefFlexString(c.ID, ""),
			api.DerefString(c.Name, "-"),
			queue,
			api.DerefUser(c.Lead, "-"),
		)
	}

	tbl.Render()
	return nil
}
