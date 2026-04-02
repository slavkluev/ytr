package comment

import (
	"fmt"
	"io"
	"time"

	"github.com/slavkluev/go-yandex-tracker/tracker"
	"github.com/spf13/cobra"

	"github.com/slavkluev/ytr/internal/api"
	"github.com/slavkluev/ytr/internal/cmd/jsonfields"
	"github.com/slavkluev/ytr/internal/config"
	"github.com/slavkluev/ytr/internal/output"
	"github.com/slavkluev/ytr/internal/validate"
)

const (
	// commentTableReservedWidth is the space reserved for ID (8), author (15),
	// date (12), and padding (9) in table output.
	commentTableReservedWidth = 44
	// commentMinColumnWidth is the minimum width for truncated body column.
	commentMinColumnWidth = 10
)

// CommentFields lists the available JSON field names for comment output.
var CommentFields = []string{"id", "author", "body", "createdAt", "updatedAt"}

// commentItem is a clean struct for JSON serialization of comment data.
// Used by both list and create commands.
type commentItem struct {
	ID        string `json:"id"`
	Author    string `json:"author"`
	Body      string `json:"body"`
	CreatedAt string `json:"createdAt"`
	UpdatedAt string `json:"updatedAt,omitempty"`
}

// newListCmd creates the "comment list" command.
func newListCmd() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "list ISSUE-KEY",
		Short: "List comments on an issue",
		Long: `List all comments on a Yandex Tracker issue.

JSON FIELDS
  id, author, body, createdAt, updatedAt

SEE ALSO
  ytr comment create  - Add comment to issue
  ytr issue view      - View issue details`,
		Example: `  # List comments on an issue
  ytr comment list PROJ-123

  # Get comments as JSON
  ytr comment list PROJ-123 --json id,author,body

  # Extract comment bodies with jq
  ytr comment list PROJ-123 --json body --jq '.[].body'`,
		Args: cobra.ExactArgs(1),
		PreRunE: func(cmd *cobra.Command, args []string) error {
			return validate.ValidateIssueKey(args[0])
		},
		RunE: func(cmd *cobra.Command, args []string) error {
			return runList(cmd, args[0])
		},
	}

	jsonfields.Register("ytr comment list", CommentFields)

	return cmd
}

// runList executes the comment list logic.
func runList(cmd *cobra.Command, issueKey string) error {
	// Handle --json field hint (no fields specified, D-10).
	if output.IsJSON() && !output.HasFieldSelection() && output.JQFilter == "" {
		return output.PrintFieldHint(cmd.ErrOrStderr(), "comment list", CommentFields)
	}

	// If --jq without --json: auto-populate all fields (Pitfall 5).
	if output.JQFilter != "" && !output.HasFieldSelection() {
		output.JSONFields = CommentFields
	}

	// Validate requested fields.
	if output.HasFieldSelection() {
		if err := output.ValidateFields(output.JSONFields, CommentFields); err != nil {
			return err
		}
		output.JSONFields = output.NormalizeFields(output.JSONFields, CommentFields)
	}

	// Resolve auth from root persistent flags.
	tokenFlag, _ := cmd.Root().PersistentFlags().GetString("token")
	orgIDFlag, _ := cmd.Root().PersistentFlags().GetString("org-id")
	orgTypeFlag, _ := cmd.Root().PersistentFlags().GetString("org-type")

	auth, err := config.ResolveAuth(tokenFlag, orgIDFlag, orgTypeFlag)
	if err != nil {
		return err
	}

	lister := newCommentLister(auth)

	comments, _, err := lister.ListComments(cmd.Context(), issueKey, nil)
	if err != nil {
		return api.MapAPIError(err)
	}

	return renderListOutput(cmd.OutOrStdout(), comments)
}

// renderListOutput handles JSON/quiet/table output for the comment list result.
func renderListOutput(w io.Writer, comments []*tracker.Comment) error {
	if output.IsJSON() {
		items := make([]commentItem, len(comments))
		for i, c := range comments {
			items[i] = toCommentItem(c)
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
		ids := make([]string, len(comments))
		for i, c := range comments {
			ids[i] = api.DerefFlexString(c.ID, "")
		}
		output.PrintQuiet(w, ids...)
		return nil
	}

	// Table output.
	if len(comments) == 0 {
		_, err := fmt.Fprintln(w, "No comments found")
		return err
	}

	tbl := output.NewTable(w)
	tbl.AddHeader("ID", "AUTHOR", "DATE", "BODY")

	for _, c := range comments {
		id := api.DerefFlexString(c.ID, "")
		author := api.DerefUser(c.CreatedBy, "-")
		date := "-"
		if c.CreatedAt != nil {
			date = output.TimeAgo(c.CreatedAt.Time)
		}
		body := api.DerefString(c.Text, "")
		// Truncate body to fit terminal width.
		maxBody := max(output.TerminalWidth()-commentTableReservedWidth, commentMinColumnWidth)
		body = output.TruncateDisplay(body, maxBody)
		tbl.AddRow(id, author, date, body)
	}

	tbl.Render()
	return nil
}

// toCommentItem converts a tracker.Comment to a clean JSON-serializable struct.
func toCommentItem(c *tracker.Comment) commentItem {
	item := commentItem{
		ID:     api.DerefFlexString(c.ID, ""),
		Author: api.DerefUser(c.CreatedBy, ""),
		Body:   api.DerefString(c.Text, ""),
	}

	if c.CreatedAt != nil {
		item.CreatedAt = c.CreatedAt.Format(time.RFC3339)
	}
	if c.UpdatedAt != nil {
		item.UpdatedAt = c.UpdatedAt.Format(time.RFC3339)
	}

	return item
}
