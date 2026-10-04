package issue

import (
	"fmt"
	"io"

	"github.com/slavkluev/go-yandex-tracker/tracker"
	"github.com/spf13/cobra"

	"github.com/slavkluev/ytr/internal/api"
	"github.com/slavkluev/ytr/internal/cmd/runner"
	"github.com/slavkluev/ytr/internal/output"
	"github.com/slavkluev/ytr/internal/validate"
)

const defaultLimit = 50

// IssueChangelogFields are the --json fields of issue changelog.
var IssueChangelogFields = runner.ItemFields[changelogEntry]()

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
	cmd.Flags().IntVar(&limit, "limit", defaultLimit, "Maximum number of changelog entries per page (max 1000)")
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
	if err := validate.ConflictingAllAndCursor(
		cmd.Flags().Changed("all"), cmd.Flags().Changed("cursor"),
	); err != nil {
		return err
	}

	if err := validate.ValidatePageLimit(limit); err != nil {
		return err
	}

	opts, err := runner.SelectFields(cmd, IssueChangelogFields)
	if err != nil {
		return err
	}

	client, err := runner.Client(cmd)
	if err != nil {
		return err
	}

	query := &tracker.ChangelogOptions{ID: cursor, PerPage: limit, Field: fieldFilter, Type: typeFilter}

	var (
		entries []*tracker.Changelog
		page    output.PaginationMeta
	)
	if all {
		entries, err = runner.Collect(client.Issues.GetChangelogIter(cmd.Context(), args[0], query))
	} else {
		entries, _, err = client.Issues.GetChangelog(cmd.Context(), args[0], query)
		page = changelogPagination(entries, limit)
	}
	if err != nil {
		return api.MapAPIError(err)
	}

	if opts.IsJSON() {
		return runner.PrintPage(cmd, opts, normalizeChangelog(entries), page)
	}

	return runner.PrintText(cmd, func(w io.Writer) error {
		return renderChangelogNonJSON(w, opts, flattenChangelog(entries))
	})
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
