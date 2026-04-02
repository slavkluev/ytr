package link

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

// LinkListFields lists the available JSON field names for link output.
var LinkListFields = []string{"id", "type", "issue", "summary"}

// linkItem is a clean struct for JSON serialization of link data.
// Used by both list and create commands.
type linkItem struct {
	ID      string `json:"id"`
	Type    string `json:"type"`
	Issue   string `json:"issue"`
	Summary string `json:"summary"`
}

// linkTypeDisplay returns the direction-aware link type label.
// For inward links it returns Type.Inward, for outward links Type.Outward.
// Falls back to Type.ID if direction is unknown, or "-" if Type is nil.
func linkTypeDisplay(link *tracker.IssueLink) string {
	if link.Type == nil || link.Direction == nil {
		return "-"
	}

	switch api.DerefString(link.Direction, "") {
	case "inward":
		return api.DerefString(link.Type.Inward, "-")
	case "outward":
		return api.DerefString(link.Type.Outward, "-")
	default:
		return api.DerefFlexString(link.Type.ID, "-")
	}
}

// toLinkItem converts a tracker.IssueLink to a clean JSON-serializable struct.
func toLinkItem(link *tracker.IssueLink) linkItem {
	item := linkItem{
		ID:   api.DerefFlexString(link.ID, ""),
		Type: linkTypeDisplay(link),
	}

	if link.Object != nil {
		item.Issue = api.DerefString(link.Object.Key, "")
		item.Summary = api.DerefString(link.Object.Summary, "")
	}

	return item
}

// newListCmd creates the "link list" command.
func newListCmd() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "list ISSUE-KEY",
		Short: "List links on an issue",
		Long: `List all links on a Yandex Tracker issue.

JSON FIELDS
  id, type, issue, summary

SEE ALSO
  ytr link create  - Create a link to another issue
  ytr link delete  - Delete a link`,
		Example: `  # List links on an issue
  ytr link list PROJ-123

  # Get links as JSON
  ytr link list PROJ-123 --json id,type,issue

  # Extract link types with jq
  ytr link list PROJ-123 --json type --jq '.[].type'`,
		Args: cobra.ExactArgs(1),
		PreRunE: func(cmd *cobra.Command, args []string) error {
			return validate.ValidateIssueKey(args[0])
		},
		RunE: func(cmd *cobra.Command, args []string) error {
			return runList(cmd, args[0])
		},
	}

	jsonfields.Register("ytr link list", LinkListFields)

	return cmd
}

// runList executes the link list logic.
func runList(cmd *cobra.Command, issueKey string) error {
	// Handle --json field hint (no fields specified, D-10).
	if output.IsJSON() && !output.HasFieldSelection() && output.JQFilter == "" {
		return output.PrintFieldHint(cmd.ErrOrStderr(), "link list", LinkListFields)
	}

	// If --jq without --json: auto-populate all fields (Pitfall 5).
	if output.JQFilter != "" && !output.HasFieldSelection() {
		output.JSONFields = LinkListFields
	}

	// Validate requested fields.
	if output.HasFieldSelection() {
		if err := output.ValidateFields(output.JSONFields, LinkListFields); err != nil {
			return err
		}
		output.JSONFields = output.NormalizeFields(output.JSONFields, LinkListFields)
	}

	// Resolve auth from root persistent flags.
	tokenFlag, _ := cmd.Root().PersistentFlags().GetString("token")
	orgIDFlag, _ := cmd.Root().PersistentFlags().GetString("org-id")
	orgTypeFlag, _ := cmd.Root().PersistentFlags().GetString("org-type")

	auth, err := config.ResolveAuth(tokenFlag, orgIDFlag, orgTypeFlag)
	if err != nil {
		return err
	}

	lister := newLinkLister(auth)

	links, _, err := lister.GetLinks(cmd.Context(), issueKey)
	if err != nil {
		return api.MapAPIError(err)
	}

	return renderOutput(cmd.OutOrStdout(), links)
}

// renderOutput handles JSON/quiet/table output for the link list result.
func renderOutput(w io.Writer, links []*tracker.IssueLink) error {
	if output.IsJSON() {
		items := make([]linkItem, len(links))
		for i, link := range links {
			items[i] = toLinkItem(link)
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
		ids := make([]string, len(links))
		for i, link := range links {
			ids[i] = api.DerefFlexString(link.ID, "")
		}
		output.PrintQuiet(w, ids...)
		return nil
	}

	// Table output.
	if len(links) == 0 {
		_, err := fmt.Fprintln(w, "No links found")
		return err
	}

	tbl := output.NewTable(w)
	tbl.AddHeader("ID", "TYPE", "ISSUE", "SUMMARY")

	for _, link := range links {
		id := api.DerefFlexString(link.ID, "")
		linkType := linkTypeDisplay(link)
		issueKey := ""
		summary := ""
		if link.Object != nil {
			issueKey = api.DerefString(link.Object.Key, "")
			summary = api.DerefString(link.Object.Summary, "")
		}
		tbl.AddRow(id, linkType, issueKey, summary)
	}

	tbl.Render()
	return nil
}
