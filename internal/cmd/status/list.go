package status

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

// StatusListFields lists the available JSON field names for status list output.
var StatusListFields = []string{"id", "key", "name"}

// statusItem is a clean struct for JSON serialization of status data.
type statusItem struct {
	ID   string `json:"id"`
	Key  string `json:"key"`
	Name string `json:"name"`
}

// newListCmd creates the "status list" command.
func newListCmd() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "list",
		Short: "List workflow statuses",
		Long: `List all workflow statuses in Yandex Tracker.

JSON FIELDS
  id, key, name

SEE ALSO
  ytr priority list    - List priorities
  ytr resolution list  - List resolutions
  ytr issuetype list   - List issue types`,
		Example: `  # List all statuses
  ytr status list

  # Get statuses as JSON
  ytr status list --json id,key,name`,
		Args: cobra.NoArgs,
		RunE: func(cmd *cobra.Command, _ []string) error {
			return runList(cmd)
		},
	}

	jsonfields.Register("ytr status list", StatusListFields)

	return cmd
}

// runList executes the status list logic.
func runList(cmd *cobra.Command) error {
	if output.IsJSON() && !output.HasFieldSelection() && output.JQFilter == "" {
		return output.PrintFieldHint(cmd.ErrOrStderr(), "status list", StatusListFields)
	}

	if output.JQFilter != "" && !output.HasFieldSelection() {
		output.JSONFields = StatusListFields
	}

	// Validate requested fields.
	if output.HasFieldSelection() {
		if err := output.ValidateFields(output.JSONFields, StatusListFields); err != nil {
			return err
		}
		output.JSONFields = output.NormalizeFields(output.JSONFields, StatusListFields)
	}

	// Resolve auth from root persistent flags.
	tokenFlag, _ := cmd.Root().PersistentFlags().GetString("token")
	orgIDFlag, _ := cmd.Root().PersistentFlags().GetString("org-id")
	orgTypeFlag, _ := cmd.Root().PersistentFlags().GetString("org-type")

	auth, err := config.ResolveAuth(tokenFlag, orgIDFlag, orgTypeFlag)
	if err != nil {
		return err
	}

	lister := newStatusLister(auth)

	statuses, _, err := lister.List(cmd.Context())
	if err != nil {
		return api.MapAPIError(err)
	}

	return renderOutput(cmd.OutOrStdout(), statuses)
}

// renderOutput handles JSON/quiet/table output for the status list result.
func renderOutput(w io.Writer, statuses []*tracker.Status) error {
	if output.IsJSON() {
		items := make([]statusItem, len(statuses))
		for i, s := range statuses {
			items[i] = toStatusItem(s)
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
		keys := make([]string, len(statuses))
		for i, s := range statuses {
			keys[i] = api.DerefString(s.Key, "")
		}
		output.PrintQuiet(w, keys...)
		return nil
	}

	// Table output.
	if len(statuses) == 0 {
		_, err := fmt.Fprintln(w, "No statuses found")
		return err
	}

	tbl := output.NewTable(w)
	tbl.AddHeader("ID", "KEY", "NAME")

	for _, s := range statuses {
		tbl.AddRow(
			api.DerefFlexString(s.ID, "-"),
			api.DerefString(s.Key, "-"),
			api.DerefString(s.Name, "-"),
		)
	}

	tbl.Render()
	return nil
}

// toStatusItem converts a tracker.Status to a clean JSON-serializable struct.
func toStatusItem(s *tracker.Status) statusItem {
	return statusItem{
		ID:   api.DerefFlexString(s.ID, ""),
		Key:  api.DerefString(s.Key, ""),
		Name: api.DerefString(s.Name, ""),
	}
}
