package priority

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

// PriorityListFields lists the available JSON field names for priority list output.
var PriorityListFields = []string{"id", "key", "name"}

// priorityItem is a clean struct for JSON serialization of priority data.
type priorityItem struct {
	ID   string `json:"id"`
	Key  string `json:"key"`
	Name string `json:"name"`
}

// newListCmd creates the "priority list" command.
func newListCmd() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "list",
		Short: "List priorities",
		Long: `List all priorities in Yandex Tracker.

JSON FIELDS
  id, key, name

SEE ALSO
  ytr status list      - List workflow statuses
  ytr resolution list  - List resolutions
  ytr issuetype list   - List issue types`,
		Example: `  # List all priorities
  ytr priority list

  # Get priorities as JSON
  ytr priority list --json id,key,name`,
		Args: cobra.NoArgs,
		RunE: func(cmd *cobra.Command, _ []string) error {
			return runList(cmd)
		},
	}

	jsonfields.Register("ytr priority list", PriorityListFields)

	return cmd
}

// runList executes the priority list logic.
func runList(cmd *cobra.Command) error {
	// Handle --json field hint (no fields specified, D-10).
	if output.IsJSON() && !output.HasFieldSelection() && output.JQFilter == "" {
		return output.PrintFieldHint(cmd.ErrOrStderr(), "priority list", PriorityListFields)
	}

	// If --jq without --json: auto-populate all fields (Pitfall 5).
	if output.JQFilter != "" && !output.HasFieldSelection() {
		output.JSONFields = PriorityListFields
	}

	// Validate requested fields.
	if output.HasFieldSelection() {
		if err := output.ValidateFields(output.JSONFields, PriorityListFields); err != nil {
			return err
		}
		output.JSONFields = output.NormalizeFields(output.JSONFields, PriorityListFields)
	}

	// Resolve auth from root persistent flags.
	tokenFlag, _ := cmd.Root().PersistentFlags().GetString("token")
	orgIDFlag, _ := cmd.Root().PersistentFlags().GetString("org-id")
	orgTypeFlag, _ := cmd.Root().PersistentFlags().GetString("org-type")

	auth, err := config.ResolveAuth(tokenFlag, orgIDFlag, orgTypeFlag)
	if err != nil {
		return err
	}

	lister := newPriorityLister(auth)

	priorities, _, err := lister.List(cmd.Context(), nil)
	if err != nil {
		return api.MapAPIError(err)
	}

	return renderOutput(cmd.OutOrStdout(), priorities)
}

// renderOutput handles JSON/quiet/table output for the priority list result.
func renderOutput(w io.Writer, priorities []*tracker.Priority) error {
	if output.IsJSON() {
		items := make([]priorityItem, len(priorities))
		for i, p := range priorities {
			items[i] = toPriorityItem(p)
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
		keys := make([]string, len(priorities))
		for i, p := range priorities {
			keys[i] = api.DerefString(p.Key, "")
		}
		output.PrintQuiet(w, keys...)
		return nil
	}

	// Table output.
	if len(priorities) == 0 {
		_, err := fmt.Fprintln(w, "No priorities found")
		return err
	}

	tbl := output.NewTable(w)
	tbl.AddHeader("ID", "KEY", "NAME")

	for _, p := range priorities {
		tbl.AddRow(
			api.DerefFlexString(p.ID, "-"),
			api.DerefString(p.Key, "-"),
			api.DerefString(p.Name, "-"),
		)
	}

	tbl.Render()
	return nil
}

// toPriorityItem converts a tracker.Priority to a clean JSON-serializable struct.
func toPriorityItem(p *tracker.Priority) priorityItem {
	return priorityItem{
		ID:   api.DerefFlexString(p.ID, ""),
		Key:  api.DerefString(p.Key, ""),
		Name: api.DerefString(p.Name, ""),
	}
}
