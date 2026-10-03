package issue

import (
	"context"
	"fmt"
	"io"

	"github.com/slavkluev/go-yandex-tracker/tracker"
	"github.com/spf13/cobra"

	"github.com/slavkluev/ytr/internal/api"
	"github.com/slavkluev/ytr/internal/cmd/runner"
	"github.com/slavkluev/ytr/internal/config"
	"github.com/slavkluev/ytr/internal/output"
	"github.com/slavkluev/ytr/internal/validate"
)

// IssueChangelogFields lists the available JSON field names for changelog output.
var IssueChangelogFields = []string{
	"date", "author", "authorId", "type", "transport",
	"fields", "comments", "links", "attachments", "worklog", "relatedResolutions",
}

func newChangelogCmd() *cobra.Command {
	var (
		fieldFilter string
		typeFilter  string
		limit       int
		cursor      string
		all         bool
	)

	cmd := &cobra.Command{
		Use:   "changelog ISSUE-KEY",
		Short: "Show issue change history",
		Long: `Display the change history of a Yandex Tracker issue.

Each changelog entry represents an atomic event (field change, comment, link, etc.)
with the date, author, type, and structured details.

JSON FIELDS
  date, author, authorId, type, transport, fields, comments, links, attachments, worklog, relatedResolutions`,
		Example: `  # Show all changes for an issue
  ytr issue changelog PROJ-123

  # Filter to only status transitions
  ytr issue changelog PROJ-123 --field status

  # Get as JSON with all fields
  ytr issue changelog PROJ-123 --json date,author,type,fields,comments,links

  # Extract status transitions using jq
  ytr issue changelog PROJ-123 --json date,type,fields --jq '.items[] | select(.type=="IssueWorkflow")'

  # Fetch all pages automatically
  ytr issue changelog PROJ-123 --all`,
		Args: cobra.ExactArgs(1),
		PreRunE: func(cmd *cobra.Command, args []string) error {
			return validate.ValidateIssueKey(args[0])
		},
		RunE: func(cmd *cobra.Command, args []string) error {
			return runChangelog(cmd, args, fieldFilter, typeFilter, limit, cursor, all)
		},
	}

	cmd.Flags().StringVar(&fieldFilter, "field", "", "Filter changes by field ID (case-sensitive, e.g. storyPoints)")
	cmd.Flags().StringVar(&typeFilter, "type", "", "Filter by change type (e.g., IssueWorkflow, IssueCommentAdded)")
	cmd.Flags().IntVar(&limit, "limit", defaultLimit, "Maximum number of changelog entries per page")
	cmd.Flags().StringVar(&cursor, "cursor", "", "Cursor ID for pagination (from previous response)")
	cmd.Flags().BoolVar(&all, "all", false, "Fetch all pages automatically")

	runner.SetFields(cmd, IssueChangelogFields)

	return cmd
}

func runChangelog(
	cmd *cobra.Command,
	args []string,
	fieldFilter, typeFilter string,
	limit int,
	cursor string,
	all bool,
) error {
	opts := output.FromContext(cmd.Context())

	if opts.WantsFieldHint(cmd.Flags().Changed("json")) {
		return output.PrintFieldHint(cmd.ErrOrStderr(), "issue changelog", IssueChangelogFields)
	}

	if err := validate.ConflictingAllAndCursor(
		cmd.Flags().Changed("all"), cmd.Flags().Changed("cursor"),
	); err != nil {
		return err
	}

	if opts.JQFilter != "" && !opts.HasFieldSelection() {
		opts.JSONFields = IssueChangelogFields
	}

	if opts.HasFieldSelection() {
		if err := output.ValidateFields(opts.JSONFields, IssueChangelogFields); err != nil {
			return err
		}
		opts.JSONFields = output.NormalizeFields(opts.JSONFields, IssueChangelogFields)
	}

	issueKey := args[0]

	tokenFlag, _ := cmd.Root().PersistentFlags().GetString("token")
	orgIDFlag, _ := cmd.Root().PersistentFlags().GetString("org-id")
	orgTypeFlag, _ := cmd.Root().PersistentFlags().GetString("org-type")

	auth, err := config.ResolveAuth(tokenFlag, orgIDFlag, orgTypeFlag)
	if err != nil {
		return err
	}

	getter := newChangelogGetter(auth)

	if limit < 1 {
		limit = defaultLimit
	}
	if limit > maxLimit {
		limit = maxLimit
	}

	entries, page, err := fetchChangelogPage(
		cmd.Context(), getter, issueKey, limit, cursor, all, fieldFilter, typeFilter,
	)
	if err != nil {
		return err
	}

	if opts.IsJSON() {
		return renderChangelogJSON(cmd.OutOrStdout(), opts, changelogDocument(entries, opts.JSONFields, page))
	}

	return renderChangelogNonJSON(cmd.OutOrStdout(), opts, flattenChangelog(entries))
}

func fetchChangelogPage(
	ctx context.Context,
	getter changelogGetter,
	issueKey string,
	limit int,
	cursor string,
	all bool,
	fieldFilter string,
	typeFilter string,
) ([]*tracker.Changelog, output.PaginationMeta, error) {
	if all {
		entries, err := fetchAllChangelog(ctx, getter, issueKey, limit, fieldFilter, typeFilter)
		return entries, output.PaginationMeta{}, err
	}

	opts := &tracker.ChangelogOptions{
		ID:      cursor,
		PerPage: limit,
		Field:   fieldFilter,
		Type:    typeFilter,
	}
	entries, _, err := getter.GetChangelog(ctx, issueKey, opts)
	if err != nil {
		return nil, output.PaginationMeta{}, api.MapAPIError(err)
	}

	return entries, changelogPagination(entries, limit), nil
}

func renderChangelogJSON(w io.Writer, opts *output.Options, doc output.PaginatedResult) error {
	if opts.JQFilter != "" {
		return output.ApplyJQ(w, doc, opts.JQFilter)
	}
	return opts.PrintJSON(w, doc)
}

func renderChangelogNonJSON(w io.Writer, opts *output.Options, items []changelogItem) error {
	if opts.Quiet {
		for _, item := range items {
			output.PrintQuiet(w, fmt.Sprintf("%s: %s -> %s", item.Field, item.From, item.To))
		}
		return nil
	}

	return renderChangelogTable(w, opts, items)
}

func fetchAllChangelog(
	ctx context.Context,
	getter changelogGetter,
	issueKey string,
	limit int,
	fieldFilter string,
	typeFilter string,
) ([]*tracker.Changelog, error) {
	var all []*tracker.Changelog
	currentCursor := ""

	for {
		opts := &tracker.ChangelogOptions{
			ID:      currentCursor,
			PerPage: limit,
			Field:   fieldFilter,
			Type:    typeFilter,
		}
		entries, _, err := getter.GetChangelog(ctx, issueKey, opts)
		if err != nil {
			return nil, api.MapAPIError(err)
		}

		if len(entries) == 0 {
			break
		}

		all = append(all, entries...)

		if len(entries) < limit {
			break
		}

		lastID := lastChangelogCursorID(entries)
		if lastID == "" {
			break
		}
		currentCursor = lastID
	}

	return all, nil
}

func renderChangelogTable(w io.Writer, opts *output.Options, items []changelogItem) error {
	if len(items) == 0 {
		_, err := fmt.Fprintln(w, "No changes found")
		return err
	}

	tbl := opts.NewTable(w)
	tbl.AddHeader("DATE", "AUTHOR", "FIELD", "FROM", "TO")

	for _, item := range items {
		tbl.AddRow(item.Date, item.Author, item.Field, item.From, item.To)
	}

	tbl.Render()
	return nil
}
