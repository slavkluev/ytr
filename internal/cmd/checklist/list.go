package checklist

import (
	"fmt"
	"io"

	"github.com/slavkluev/go-yandex-tracker/tracker"
	"github.com/spf13/cobra"

	"github.com/slavkluev/ytr/internal/api"
	"github.com/slavkluev/ytr/internal/cmd/jsonfields"
	"github.com/slavkluev/ytr/internal/config"
	"github.com/slavkluev/ytr/internal/output"
	"github.com/slavkluev/ytr/internal/validate"
)

// ChecklistFields lists the available JSON field names for checklist output.
var ChecklistFields = []string{"id", "text", "checked", "assignee"}

// checklistItem is a clean struct for JSON serialization of checklist data.
// Used by list, create, and edit commands.
type checklistItem struct {
	ID       string `json:"id"`
	Text     string `json:"text"`
	Checked  bool   `json:"checked"`
	Assignee string `json:"assignee,omitempty"`
}

// newListCmd creates the "checklist list" command.
func newListCmd() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "list ISSUE-KEY",
		Short: "List checklist items on an issue",
		Long: `List all checklist items on a Yandex Tracker issue.

JSON FIELDS
  id, text, checked, assignee

SEE ALSO
  ytr checklist create  - Add checklist item to issue
  ytr checklist edit    - Edit a checklist item
  ytr checklist delete  - Delete a checklist item`,
		Example: `  # List checklist items on an issue
  ytr checklist list PROJ-123

  # Get checklist as JSON
  ytr checklist list PROJ-123 --json id,text,checked`,
		Args: cobra.ExactArgs(1),
		PreRunE: func(cmd *cobra.Command, args []string) error {
			return validate.ValidateIssueKey(args[0])
		},
		RunE: func(cmd *cobra.Command, args []string) error {
			return runList(cmd, args[0])
		},
	}

	jsonfields.Register("ytr checklist list", ChecklistFields)

	return cmd
}

// runList executes the checklist list logic.
func runList(cmd *cobra.Command, issueKey string) error {
	if output.IsJSON() && !output.HasFieldSelection() && output.JQFilter == "" {
		return output.PrintFieldHint(cmd.ErrOrStderr(), "checklist list", ChecklistFields)
	}

	if output.JQFilter != "" && !output.HasFieldSelection() {
		output.JSONFields = ChecklistFields
	}

	// Validate requested fields.
	if output.HasFieldSelection() {
		if err := output.ValidateFields(output.JSONFields, ChecklistFields); err != nil {
			return err
		}
		output.JSONFields = output.NormalizeFields(output.JSONFields, ChecklistFields)
	}

	// Resolve auth from root persistent flags.
	tokenFlag, _ := cmd.Root().PersistentFlags().GetString("token")
	orgIDFlag, _ := cmd.Root().PersistentFlags().GetString("org-id")
	orgTypeFlag, _ := cmd.Root().PersistentFlags().GetString("org-type")

	auth, err := config.ResolveAuth(tokenFlag, orgIDFlag, orgTypeFlag)
	if err != nil {
		return err
	}

	lister := newChecklistLister(auth)

	items, _, err := lister.ListChecklistItems(cmd.Context(), issueKey)
	if err != nil {
		return api.MapAPIError(err)
	}

	return renderListOutput(cmd.OutOrStdout(), items)
}

// renderListOutput handles JSON/quiet/table output for the checklist list result.
func renderListOutput(w io.Writer, items []*tracker.ChecklistItem) error {
	if output.IsJSON() {
		result := make([]checklistItem, len(items))
		for i, c := range items {
			result[i] = toChecklistItem(c)
		}

		if output.HasFieldSelection() {
			filtered := make([]map[string]any, len(result))
			for i, item := range result {
				filtered[i] = output.FilterFields(item, output.JSONFields)
			}
			if output.JQFilter != "" {
				return output.ApplyJQ(w, filtered, output.JQFilter)
			}
			return output.PrintJSON(w, filtered)
		}
		if output.JQFilter != "" {
			return output.ApplyJQ(w, result, output.JQFilter)
		}
		return output.PrintJSON(w, result)
	}

	if output.IsQuiet() {
		ids := make([]string, len(items))
		for i, c := range items {
			ids[i] = api.DerefFlexString(c.ID, "")
		}
		output.PrintQuiet(w, ids...)
		return nil
	}

	// Table output.
	if len(items) == 0 {
		_, err := fmt.Fprintln(w, "No checklist items found")
		return err
	}

	tbl := output.NewTable(w)
	tbl.AddHeader("ID", "TEXT", "CHECKED", "ASSIGNEE")

	for _, c := range items {
		id := api.DerefFlexString(c.ID, "")
		text := api.DerefString(c.Text, "")
		checked := checkedDisplay(api.DerefBool(c.Checked, false))
		assignee := api.DerefUser(c.Assignee, "-")
		tbl.AddRow(id, text, checked, assignee)
	}

	tbl.Render()
	return nil
}

// toChecklistItem converts a tracker.ChecklistItem to a clean JSON struct.
func toChecklistItem(c *tracker.ChecklistItem) checklistItem {
	return checklistItem{
		ID:       api.DerefFlexString(c.ID, ""),
		Text:     api.DerefString(c.Text, ""),
		Checked:  api.DerefBool(c.Checked, false),
		Assignee: api.DerefUser(c.Assignee, ""),
	}
}

// checkedDisplay returns "yes" for true and "no" for false, used in table output.
func checkedDisplay(checked bool) string {
	if checked {
		return "yes"
	}
	return "no"
}
