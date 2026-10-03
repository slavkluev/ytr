package issuetype

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

// IssueTypeListFields lists the available JSON field names for issue type list output.
var IssueTypeListFields = []string{"id", "key", "name"}

type issueTypeItem struct {
	ID   string `json:"id"`
	Key  string `json:"key"`
	Name string `json:"name"`
}

func newListCmd() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "list",
		Short: "List issue types",
		Long: `List all issue types in Yandex Tracker.

JSON FIELDS
  id, key, name

SEE ALSO
  ytr status list      - List workflow statuses
  ytr priority list    - List priorities
  ytr resolution list  - List resolutions`,
		Example: `  # List all issue types
  ytr issuetype list

  # Get issue types as JSON
  ytr issuetype list --json id,key,name`,
		Args: cobra.NoArgs,
		RunE: func(cmd *cobra.Command, _ []string) error {
			return runList(cmd)
		},
	}

	jsonfields.Register("ytr issuetype list", IssueTypeListFields)

	return cmd
}

func runList(cmd *cobra.Command) error {
	if output.WantsFieldHint(cmd.Flags().Changed("json")) {
		return output.PrintFieldHint(cmd.ErrOrStderr(), "issuetype list", IssueTypeListFields)
	}

	if output.JQFilter != "" && !output.HasFieldSelection() {
		output.JSONFields = IssueTypeListFields
	}

	if output.HasFieldSelection() {
		if err := output.ValidateFields(output.JSONFields, IssueTypeListFields); err != nil {
			return err
		}
		output.JSONFields = output.NormalizeFields(output.JSONFields, IssueTypeListFields)
	}

	tokenFlag, _ := cmd.Root().PersistentFlags().GetString("token")
	orgIDFlag, _ := cmd.Root().PersistentFlags().GetString("org-id")
	orgTypeFlag, _ := cmd.Root().PersistentFlags().GetString("org-type")

	auth, err := config.ResolveAuth(tokenFlag, orgIDFlag, orgTypeFlag)
	if err != nil {
		return err
	}

	lister := newIssueTypeLister(auth)

	issueTypes, _, err := lister.List(cmd.Context())
	if err != nil {
		return api.MapAPIError(err)
	}

	return renderOutput(cmd.OutOrStdout(), issueTypes)
}

func renderOutput(w io.Writer, issueTypes []*tracker.IssueType) error {
	if output.IsJSON() {
		items := make([]issueTypeItem, len(issueTypes))
		for i, it := range issueTypes {
			items[i] = toIssueTypeItem(it)
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
		keys := make([]string, len(issueTypes))
		for i, it := range issueTypes {
			keys[i] = api.DerefString(it.Key, "")
		}
		output.PrintQuiet(w, keys...)
		return nil
	}

	if len(issueTypes) == 0 {
		_, err := fmt.Fprintln(w, "No issue types found")
		return err
	}

	tbl := output.NewTable(w)
	tbl.AddHeader("ID", "KEY", "NAME")

	for _, it := range issueTypes {
		tbl.AddRow(
			api.DerefFlexString(it.ID, "-"),
			api.DerefString(it.Key, "-"),
			api.DerefString(it.Name, "-"),
		)
	}

	tbl.Render()
	return nil
}

func toIssueTypeItem(it *tracker.IssueType) issueTypeItem {
	return issueTypeItem{
		ID:   api.DerefFlexString(it.ID, ""),
		Key:  api.DerefString(it.Key, ""),
		Name: api.DerefString(it.Name, ""),
	}
}
