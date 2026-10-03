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

type linkItem struct {
	ID      string `json:"id"`
	Type    string `json:"type"`
	Issue   string `json:"issue"`
	Summary string `json:"summary"`
}

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

func runList(cmd *cobra.Command, issueKey string) error {
	opts := output.FromContext(cmd.Context())

	if opts.WantsFieldHint(cmd.Flags().Changed("json")) {
		return output.PrintFieldHint(cmd.ErrOrStderr(), "link list", LinkListFields)
	}

	if opts.JQFilter != "" && !opts.HasFieldSelection() {
		opts.JSONFields = LinkListFields
	}

	if opts.HasFieldSelection() {
		if err := output.ValidateFields(opts.JSONFields, LinkListFields); err != nil {
			return err
		}
		opts.JSONFields = output.NormalizeFields(opts.JSONFields, LinkListFields)
	}

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

	return renderOutput(cmd.OutOrStdout(), opts, links)
}

func renderOutput(w io.Writer, opts *output.Options, links []*tracker.IssueLink) error {
	if opts.IsJSON() {
		items := make([]linkItem, len(links))
		for i, link := range links {
			items[i] = toLinkItem(link)
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
		ids := make([]string, len(links))
		for i, link := range links {
			ids[i] = api.DerefFlexString(link.ID, "")
		}
		output.PrintQuiet(w, ids...)
		return nil
	}

	if len(links) == 0 {
		_, err := fmt.Fprintln(w, "No links found")
		return err
	}

	tbl := opts.NewTable(w)
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
