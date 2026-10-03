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

type priorityItem struct {
	ID   string `json:"id"`
	Key  string `json:"key"`
	Name string `json:"name"`
}

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

func runList(cmd *cobra.Command) error {
	opts := output.FromContext(cmd.Context())

	if opts.WantsFieldHint(cmd.Flags().Changed("json")) {
		return output.PrintFieldHint(cmd.ErrOrStderr(), "priority list", PriorityListFields)
	}

	if opts.JQFilter != "" && !opts.HasFieldSelection() {
		opts.JSONFields = PriorityListFields
	}

	if opts.HasFieldSelection() {
		if err := output.ValidateFields(opts.JSONFields, PriorityListFields); err != nil {
			return err
		}
		opts.JSONFields = output.NormalizeFields(opts.JSONFields, PriorityListFields)
	}

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

	return renderOutput(cmd.OutOrStdout(), opts, priorities)
}

func renderOutput(w io.Writer, opts *output.Options, priorities []*tracker.Priority) error {
	if opts.IsJSON() {
		items := make([]priorityItem, len(priorities))
		for i, p := range priorities {
			items[i] = toPriorityItem(p)
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

	if opts.Quiet {
		keys := make([]string, len(priorities))
		for i, p := range priorities {
			keys[i] = api.DerefString(p.Key, "")
		}
		output.PrintQuiet(w, keys...)
		return nil
	}

	if len(priorities) == 0 {
		_, err := fmt.Fprintln(w, "No priorities found")
		return err
	}

	tbl := opts.NewTable(w)
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

func toPriorityItem(p *tracker.Priority) priorityItem {
	return priorityItem{
		ID:   api.DerefFlexString(p.ID, ""),
		Key:  api.DerefString(p.Key, ""),
		Name: api.DerefString(p.Name, ""),
	}
}
