package resolution

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

// ResolutionListFields lists the available JSON field names for resolution list output.
var ResolutionListFields = []string{"id", "key", "name"}

// resolutionItem is a clean struct for JSON serialization of resolution data.
type resolutionItem struct {
	ID   string `json:"id"`
	Key  string `json:"key"`
	Name string `json:"name"`
}

// newListCmd creates the "resolution list" command.
func newListCmd() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "list",
		Short: "List resolutions",
		Long: `List all resolutions in Yandex Tracker.

JSON FIELDS
  id, key, name

SEE ALSO
  ytr status list    - List workflow statuses
  ytr priority list  - List priorities
  ytr issuetype list - List issue types`,
		Example: `  # List all resolutions
  ytr resolution list

  # Get resolutions as JSON
  ytr resolution list --json id,key,name`,
		Args: cobra.NoArgs,
		RunE: func(cmd *cobra.Command, _ []string) error {
			return runList(cmd)
		},
	}

	jsonfields.Register("ytr resolution list", ResolutionListFields)

	return cmd
}

// runList executes the resolution list logic.
func runList(cmd *cobra.Command) error {
	// Handle --json field hint (no fields specified, D-10).
	if output.IsJSON() && !output.HasFieldSelection() && output.JQFilter == "" {
		return output.PrintFieldHint(cmd.ErrOrStderr(), "resolution list", ResolutionListFields)
	}

	// If --jq without --json: auto-populate all fields (Pitfall 5).
	if output.JQFilter != "" && !output.HasFieldSelection() {
		output.JSONFields = ResolutionListFields
	}

	// Validate requested fields.
	if output.HasFieldSelection() {
		if err := output.ValidateFields(output.JSONFields, ResolutionListFields); err != nil {
			return err
		}
		output.JSONFields = output.NormalizeFields(output.JSONFields, ResolutionListFields)
	}

	// Resolve auth from root persistent flags.
	tokenFlag, _ := cmd.Root().PersistentFlags().GetString("token")
	orgIDFlag, _ := cmd.Root().PersistentFlags().GetString("org-id")
	orgTypeFlag, _ := cmd.Root().PersistentFlags().GetString("org-type")

	auth, err := config.ResolveAuth(tokenFlag, orgIDFlag, orgTypeFlag)
	if err != nil {
		return err
	}

	lister := newResolutionLister(auth)

	resolutions, _, err := lister.List(cmd.Context())
	if err != nil {
		return api.MapAPIError(err)
	}

	return renderOutput(cmd.OutOrStdout(), resolutions)
}

// renderOutput handles JSON/quiet/table output for the resolution list result.
func renderOutput(w io.Writer, resolutions []*tracker.Resolution) error {
	if output.IsJSON() {
		items := make([]resolutionItem, len(resolutions))
		for i, r := range resolutions {
			items[i] = toResolutionItem(r)
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
		keys := make([]string, len(resolutions))
		for i, r := range resolutions {
			keys[i] = api.DerefString(r.Key, "")
		}
		output.PrintQuiet(w, keys...)
		return nil
	}

	// Table output.
	if len(resolutions) == 0 {
		_, err := fmt.Fprintln(w, "No resolutions found")
		return err
	}

	tbl := output.NewTable(w)
	tbl.AddHeader("ID", "KEY", "NAME")

	for _, r := range resolutions {
		tbl.AddRow(
			api.DerefFlexString(r.ID, "-"),
			api.DerefString(r.Key, "-"),
			api.DerefString(r.Name, "-"),
		)
	}

	tbl.Render()
	return nil
}

// toResolutionItem converts a tracker.Resolution to a clean JSON-serializable struct.
func toResolutionItem(r *tracker.Resolution) resolutionItem {
	return resolutionItem{
		ID:   api.DerefFlexString(r.ID, ""),
		Key:  api.DerefString(r.Key, ""),
		Name: api.DerefString(r.Name, ""),
	}
}
