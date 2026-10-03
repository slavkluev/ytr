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

type statusItem struct {
	ID   string `json:"id"`
	Key  string `json:"key"`
	Name string `json:"name"`
}

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

func runList(cmd *cobra.Command) error {
	opts := output.FromContext(cmd.Context())

	if opts.WantsFieldHint(cmd.Flags().Changed("json")) {
		return output.PrintFieldHint(cmd.ErrOrStderr(), "status list", StatusListFields)
	}

	if opts.JQFilter != "" && !opts.HasFieldSelection() {
		opts.JSONFields = StatusListFields
	}

	if opts.HasFieldSelection() {
		if err := output.ValidateFields(opts.JSONFields, StatusListFields); err != nil {
			return err
		}
		opts.JSONFields = output.NormalizeFields(opts.JSONFields, StatusListFields)
	}

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

	return renderOutput(cmd.OutOrStdout(), opts, statuses)
}

func renderOutput(w io.Writer, opts *output.Options, statuses []*tracker.Status) error {
	if opts.IsJSON() {
		items := make([]statusItem, len(statuses))
		for i, s := range statuses {
			items[i] = toStatusItem(s)
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
		keys := make([]string, len(statuses))
		for i, s := range statuses {
			keys[i] = api.DerefString(s.Key, "")
		}
		output.PrintQuiet(w, keys...)
		return nil
	}

	if len(statuses) == 0 {
		_, err := fmt.Fprintln(w, "No statuses found")
		return err
	}

	tbl := opts.NewTable(w)
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

func toStatusItem(s *tracker.Status) statusItem {
	return statusItem{
		ID:   api.DerefFlexString(s.ID, ""),
		Key:  api.DerefString(s.Key, ""),
		Name: api.DerefString(s.Name, ""),
	}
}
