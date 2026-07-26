package comment

import (
	"context"
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
	// defaultLimit is the number of comments requested per page by default.
	defaultLimit = 50
	// maxLimit is the highest per-page value accepted from --limit.
	maxLimit = 1000
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
	var (
		limit  int
		cursor string
		all    bool
	)

	cmd := &cobra.Command{
		Use:   "list ISSUE-KEY",
		Short: "List comments on an issue",
		Long: `List comments on a Yandex Tracker issue.

Only the first page is returned by default. Use --all to fetch every page, or
--cursor with the value from a previous response to continue where you stopped.

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
  ytr comment list PROJ-123 --json body --jq '.items[].body'

  # Fetch all pages automatically
  ytr comment list PROJ-123 --all`,
		Args: cobra.ExactArgs(1),
		PreRunE: func(cmd *cobra.Command, args []string) error {
			return validate.ValidateIssueKey(args[0])
		},
		RunE: func(cmd *cobra.Command, args []string) error {
			return runList(cmd, args[0], limit, cursor, all)
		},
	}

	cmd.Flags().IntVar(&limit, "limit", defaultLimit, "Maximum number of comments per page")
	cmd.Flags().StringVar(&cursor, "cursor", "", "Cursor ID for pagination (from previous response)")
	cmd.Flags().BoolVar(&all, "all", false, "Fetch all remaining pages automatically, starting from --cursor if set")

	jsonfields.Register("ytr comment list", CommentFields)

	return cmd
}

// runList executes the comment list logic.
func runList(cmd *cobra.Command, issueKey string, limit int, cursor string, all bool) error {
	if output.IsJSON() && !output.HasFieldSelection() && output.JQFilter == "" {
		return output.PrintFieldHint(cmd.ErrOrStderr(), "comment list", CommentFields)
	}

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

	// Validate and cap limit.
	if limit < 1 {
		limit = defaultLimit
	}
	if limit > maxLimit {
		limit = maxLimit
	}

	comments, hasMore, nextCursor, err := fetchCommentPage(
		cmd.Context(), lister, issueKey, limit, cursor, all,
	)
	if err != nil {
		return err
	}

	if output.IsJSON() {
		return renderListJSON(cmd.OutOrStdout(), comments, hasMore, nextCursor)
	}

	return renderListNonJSON(cmd.OutOrStdout(), comments)
}

// fetchCommentPage fetches comments, either all pages or a single page.
func fetchCommentPage(
	ctx context.Context,
	lister commentLister,
	issueKey string,
	limit int,
	cursor string,
	all bool,
) ([]*tracker.Comment, bool, string, error) {
	if all {
		comments, err := fetchAllComments(ctx, lister, issueKey, limit, cursor)
		return comments, false, "", err
	}

	opts := &tracker.CommentListOptions{
		ID:      cursor,
		PerPage: limit,
	}
	comments, _, err := lister.ListComments(ctx, issueKey, opts)
	if err != nil {
		return nil, false, "", api.MapAPIError(err)
	}

	hasMore := len(comments) == limit
	var next string
	if hasMore && len(comments) > 0 {
		next = api.DerefFlexString(comments[len(comments)-1].ID, "")
	}
	return comments, hasMore, next, nil
}

// fetchAllComments auto-paginates through all comment pages using cursor-based
// pagination. When cursor is non-empty, paging resumes after that comment.
func fetchAllComments(
	ctx context.Context,
	lister commentLister,
	issueKey string,
	limit int,
	cursor string,
) ([]*tracker.Comment, error) {
	var all []*tracker.Comment
	currentCursor := cursor

	for {
		opts := &tracker.CommentListOptions{
			ID:      currentCursor,
			PerPage: limit,
		}
		comments, _, err := lister.ListComments(ctx, issueKey, opts)
		if err != nil {
			return nil, api.MapAPIError(err)
		}

		if len(comments) == 0 {
			break
		}

		all = append(all, comments...)

		if len(comments) < limit {
			break
		}

		lastID := api.DerefFlexString(comments[len(comments)-1].ID, "")
		if lastID == "" {
			break
		}
		currentCursor = lastID
	}

	return all, nil
}

// renderListJSON renders the comment list as paginated JSON.
func renderListJSON(w io.Writer, comments []*tracker.Comment, hasMore bool, nextCursor string) error {
	items := make([]commentItem, len(comments))
	for i, c := range comments {
		items[i] = toCommentItem(c)
	}

	var data any
	if output.HasFieldSelection() {
		filtered := make([]map[string]any, len(items))
		for i, item := range items {
			filtered[i] = output.FilterFields(item, output.JSONFields)
		}
		data = output.PaginatedResult{
			Items:      filtered,
			Pagination: output.PaginationMeta{Cursor: nextCursor, HasMore: hasMore},
		}
	} else {
		data = output.PaginatedResult{
			Items:      items,
			Pagination: output.PaginationMeta{Cursor: nextCursor, HasMore: hasMore},
		}
	}

	if output.JQFilter != "" {
		return output.ApplyJQ(w, data, output.JQFilter)
	}
	return output.PrintJSON(w, data)
}

// renderListNonJSON handles quiet and table output for the comment list result.
func renderListNonJSON(w io.Writer, comments []*tracker.Comment) error {
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
