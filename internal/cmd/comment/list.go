package comment

import (
	"context"
	"fmt"
	"io"
	"time"

	"github.com/slavkluev/go-yandex-tracker/tracker"
	"github.com/spf13/cobra"

	"github.com/slavkluev/ytr/internal/api"
	"github.com/slavkluev/ytr/internal/cmd/runner"
	"github.com/slavkluev/ytr/internal/config"
	"github.com/slavkluev/ytr/internal/output"
	"github.com/slavkluev/ytr/internal/validate"
)

const (
	// commentTableReservedWidth is the space reserved for ID (8), author (15),
	// date (12), and padding (9) in table output.
	commentTableReservedWidth = 44

	commentMinColumnWidth = 10
	// commentPageSize is the per-page count used when auto-paginating comments.
	// The server default is 50; comment list pages explicitly so all comments
	// are returned instead of being silently truncated at the first page.
	commentPageSize = 100
)

type commentItem struct {
	ID        string `json:"id"`
	Author    string `json:"author"`
	AuthorID  string `json:"authorId"`
	Body      string `json:"body"`
	CreatedAt string `json:"createdAt"`
	UpdatedAt string `json:"updatedAt,omitempty"`
}

// CommentFields are the --json fields of every comment command.
var CommentFields = runner.ItemFields[commentItem]()

func newListCmd() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "list ISSUE-KEY",
		Short: "List comments on an issue",
		Long: `List all comments on a Yandex Tracker issue.

JSON FIELDS
  id, author, authorId, body, createdAt, updatedAt`,
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

	runner.SetFields(cmd, CommentFields)

	return cmd
}

func runList(cmd *cobra.Command, issueKey string) error {
	opts := output.FromContext(cmd.Context())

	if opts.WantsFieldHint(cmd.Flags().Changed("json")) {
		return output.PrintFieldHint(cmd.ErrOrStderr(), "comment list", CommentFields)
	}

	if opts.JQFilter != "" && !opts.HasFieldSelection() {
		opts.JSONFields = CommentFields
	}

	if opts.HasFieldSelection() {
		if err := output.ValidateFields(opts.JSONFields, CommentFields); err != nil {
			return err
		}
		opts.JSONFields = output.NormalizeFields(opts.JSONFields, CommentFields)
	}

	tokenFlag, _ := cmd.Root().PersistentFlags().GetString("token")
	orgIDFlag, _ := cmd.Root().PersistentFlags().GetString("org-id")
	orgTypeFlag, _ := cmd.Root().PersistentFlags().GetString("org-type")

	auth, err := config.ResolveAuth(tokenFlag, orgIDFlag, orgTypeFlag)
	if err != nil {
		return err
	}

	lister := newCommentLister(auth)

	comments, err := fetchAllComments(cmd.Context(), lister, issueKey)
	if err != nil {
		return err
	}

	return renderListOutput(cmd.OutOrStdout(), opts, comments)
}

// fetchAllComments retrieves every comment on an issue by following the
// comment-ID cursor until a short (or empty) page is returned. The Tracker
// list endpoint caps a single page at the server default (~50).
func fetchAllComments(
	ctx context.Context,
	lister commentLister,
	issueKey string,
) ([]*tracker.Comment, error) {
	var all []*tracker.Comment
	cursor := ""

	for {
		opts := &tracker.CommentListOptions{ID: cursor, PerPage: commentPageSize}
		comments, _, err := lister.ListComments(ctx, issueKey, opts)
		if err != nil {
			return nil, api.MapAPIError(err)
		}

		if len(comments) == 0 {
			break
		}

		all = append(all, comments...)

		if len(comments) < commentPageSize {
			break
		}

		lastID := api.DerefFlexString(comments[len(comments)-1].ID, "")
		// Stop on a missing or non-advancing cursor to avoid looping forever.
		if lastID == "" || lastID == cursor {
			break
		}
		cursor = lastID
	}

	return all, nil
}

func renderListOutput(w io.Writer, opts *output.Options, comments []*tracker.Comment) error {
	if opts.IsJSON() {
		items := make([]commentItem, len(comments))
		for i, c := range comments {
			items[i] = toCommentItem(c)
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
		ids := make([]string, len(comments))
		for i, c := range comments {
			ids[i] = api.DerefFlexString(c.ID, "")
		}
		output.PrintQuiet(w, ids...)
		return nil
	}

	if len(comments) == 0 {
		_, err := fmt.Fprintln(w, "No comments found")
		return err
	}

	tbl := opts.NewTable(w)
	tbl.AddHeader("ID", "AUTHOR", "DATE", "BODY")

	for _, c := range comments {
		id := api.DerefFlexString(c.ID, "")
		author := c.CreatedBy.DisplayOr("-")
		date := "-"
		if c.CreatedAt != nil {
			date = opts.FormatTime(c.CreatedAt.Time)
		}
		body := opts.FitColumn(api.DerefString(c.Text, ""), commentTableReservedWidth, commentMinColumnWidth)
		tbl.AddRow(id, author, date, body)
	}

	tbl.Render()
	return nil
}

func toCommentItem(c *tracker.Comment) commentItem {
	item := commentItem{
		ID:       api.DerefFlexString(c.ID, ""),
		Author:   c.CreatedBy.DisplayOr(""),
		AuthorID: c.CreatedBy.IDOr(""),
		Body:     api.DerefString(c.Text, ""),
	}

	if c.CreatedAt != nil {
		item.CreatedAt = c.CreatedAt.Format(time.RFC3339)
	}
	if c.UpdatedAt != nil {
		item.UpdatedAt = c.UpdatedAt.Format(time.RFC3339)
	}

	return item
}
