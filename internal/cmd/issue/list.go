package issue

import (
	"fmt"
	"io"
	"strconv"
	"strings"
	"time"

	"github.com/jedib0t/go-pretty/v6/text"
	"github.com/slavkluev/go-yandex-tracker/tracker"
	"github.com/spf13/cobra"

	"github.com/slavkluev/ytr/internal/api"
	"github.com/slavkluev/ytr/internal/cmd/jsonfields"
	"github.com/slavkluev/ytr/internal/config"
	ytrerrors "github.com/slavkluev/ytr/internal/errors"
	"github.com/slavkluev/ytr/internal/output"
	"github.com/slavkluev/ytr/internal/validate"
)

const (
	defaultLimit = 50
	maxLimit     = 1000

	// tableReservedWidth is the space reserved for key (12), status (15),
	// assignee (15), and padding (9) in table output.
	tableReservedWidth = 51
	// minColumnWidth is the minimum width for truncated text columns.
	minColumnWidth = 10
)

// IssueListFields lists the available JSON field names for issue list output.
var IssueListFields = []string{"key", "summary", "status", "priority", "type", "assignee", "createdAt", "updatedAt"}

// issueListItem is a clean struct for JSON serialization of issue list items.
// Raw tracker.Issue fields are pointer types that produce nulls in JSON;
// this struct uses value types with proper json tags.
type issueListItem struct {
	Key       string `json:"key"`
	Summary   string `json:"summary"`
	Status    string `json:"status"`
	Priority  string `json:"priority,omitempty"`
	Type      string `json:"type,omitempty"`
	Assignee  string `json:"assignee,omitempty"`
	CreatedAt string `json:"createdAt,omitempty"`
	UpdatedAt string `json:"updatedAt,omitempty"`
}

// newListCmd creates the "issue list" command with filters and pagination.
func newListCmd() *cobra.Command {
	var (
		query       string
		filterFlags []string
		orderBy     string
		orderAsc    bool
		limit       int
		cursor      string
		all         bool
	)

	cmd := &cobra.Command{
		Use:   "list",
		Short: "List issues",
		Long: `Search and list Yandex Tracker issues with filtering and pagination.

Supports two search modes:
  - Structured filters: use --filter key=value (repeatable) for field filtering
  - Query language: use --query for full Tracker query language expressions

The two modes are mutually exclusive: --query cannot be combined with --filter.

JSON FIELDS
  key, summary, status, priority, type, assignee, createdAt, updatedAt

SEE ALSO
  ytr issue view      - View issue details
  ytr issue create    - Create a new issue`,
		Example: `  # Filter by queue
  ytr issue list --filter queue=PROJ

  # Multiple filters
  ytr issue list --filter queue=PROJ --filter status=open --filter assignee=me()

  # Search with Tracker query language
  ytr issue list --query 'Queue: PROJ AND Status: open "Sort By": Updated DESC'

  # Filter by priority
  ytr issue list --filter queue=PROJ --filter priority=critical

  # Sort results (descending by default)
  ytr issue list --filter queue=PROJ --order-by updatedAt

  # Sort ascending
  ytr issue list --filter queue=PROJ --order-by createdAt --order-asc

  # Get issue keys and statuses as JSON
  ytr issue list --filter queue=PROJ --json key,summary,status

  # Extract just keys with jq
  ytr issue list --filter queue=PROJ --json key --jq '.items[].key'`,
		RunE: func(cmd *cobra.Command, args []string) error {
			return runList(cmd, query, filterFlags, orderBy, orderAsc, limit, cursor, all)
		},
	}

	cmd.Flags().StringVar(&query, "query", "", "Search using Tracker query language (mutually exclusive with --filter)")
	cmd.Flags().StringArrayVar(&filterFlags, "filter", nil, "Filter by field (key=value, repeatable)")
	cmd.Flags().StringVar(&orderBy, "order-by", "", "Sort by field name (e.g., updated, created, priority)")
	cmd.Flags().BoolVar(&orderAsc, "order-asc", false, "Sort ascending (default: descending)")
	cmd.Flags().IntVar(&limit, "limit", defaultLimit, "Maximum number of results per page (max 1000)")
	cmd.Flags().StringVar(&cursor, "cursor", "", "Page number for pagination")
	cmd.Flags().BoolVar(&all, "all", false, "Fetch all pages automatically")

	jsonfields.Register("ytr issue list", IssueListFields)

	return cmd
}

// issueSearchResult holds the pagination state from a search operation.
type issueSearchResult struct {
	issues     []*tracker.Issue
	totalCount int
	hasMore    bool
	nextCursor string
}

// runList executes the issue list logic.
func runList(cmd *cobra.Command, query string, filterFlags []string, orderBy string, orderAsc bool, limit int, cursor string, all bool) error {
	// Validate flag conflicts before any API work.
	if err := validateSearchFlags(cmd); err != nil {
		return err
	}

	if output.IsJSON() && !output.HasFieldSelection() && output.JQFilter == "" {
		return output.PrintFieldHint(cmd.ErrOrStderr(), "issue list", IssueListFields)
	}

	if output.JQFilter != "" && !output.HasFieldSelection() {
		output.JSONFields = IssueListFields
	}

	// Validate requested fields.
	if output.HasFieldSelection() {
		if err := output.ValidateFields(output.JSONFields, IssueListFields); err != nil {
			return err
		}
		output.JSONFields = output.NormalizeFields(output.JSONFields, IssueListFields)
	}

	// Resolve auth from root persistent flags.
	tokenFlag, _ := cmd.Root().PersistentFlags().GetString("token")
	orgIDFlag, _ := cmd.Root().PersistentFlags().GetString("org-id")
	orgTypeFlag, _ := cmd.Root().PersistentFlags().GetString("org-type")

	auth, err := config.ResolveAuth(tokenFlag, orgIDFlag, orgTypeFlag)
	if err != nil {
		return err
	}

	searcher := newSearcher(auth)

	// Validate and cap limit.
	if limit < 1 {
		limit = defaultLimit
	}
	if limit > maxLimit {
		limit = maxLimit
	}

	// Parse cursor as page number.
	page, err := validate.ParsePageCursor(cursor)
	if err != nil {
		return err
	}

	// Build the search request.
	searchReq := &tracker.IssueSearchRequest{}

	if query != "" {
		// Query mode: set Query field only.
		searchReq.Query = tracker.Ptr(query)
	} else if len(filterFlags) > 0 {
		// Filter mode: parse --filter key=value entries.
		filter, err := parseFilterFlags(filterFlags)
		if err != nil {
			return err
		}
		searchReq.Filter = filter
	}

	// Apply ordering.
	if orderBy != "" {
		prefix := "-"
		if orderAsc {
			prefix = "+"
		}
		searchReq.Order = tracker.Ptr(prefix + orderBy)
	}

	result, err := fetchIssues(cmd, searcher, searchReq, limit, page, all)
	if err != nil {
		return err
	}

	return renderListOutput(cmd.OutOrStdout(), result)
}

// validateSearchFlags checks for mutually exclusive flag combinations.
func validateSearchFlags(cmd *cobra.Command) error {
	queryChanged := cmd.Flags().Changed("query")
	orderByChanged := cmd.Flags().Changed("order-by")

	if queryChanged && cmd.Flags().Changed("filter") {
		return ytrerrors.NewUserError(
			"cannot combine --query with --filter",
			"Use --query for Tracker query language, or --filter for structured search, but not both",
		)
	}

	if orderByChanged && queryChanged {
		return ytrerrors.NewUserError(
			"--order-by cannot be used with --query",
			`Include sorting in the query string: '"Sort By": fieldName ASC'`,
		)
	}

	if cmd.Flags().Changed("order-asc") && !orderByChanged {
		return ytrerrors.NewUserError(
			"--order-asc requires --order-by",
			"Use --order-by to specify the sort field (e.g., --order-by updated --order-asc)",
		)
	}

	return nil
}

// parseFilterFlags parses --filter key=value flags into a map.
// Splits on the first = sign only, so values may contain =.
// Duplicate keys accumulate into []string slices.
func parseFilterFlags(flags []string) (map[string]any, error) {
	result := make(map[string]any, len(flags))

	for _, f := range flags {
		idx := strings.Index(f, "=")
		if idx < 1 {
			return nil, ytrerrors.NewUserError(
				fmt.Sprintf("invalid filter format %q: expected key=value", f),
				"Use --filter key=value (e.g., --filter priority=critical)",
			)
		}

		key := f[:idx]
		val := f[idx+1:]

		existing, exists := result[key]
		if !exists {
			result[key] = val
		} else {
			switch ev := existing.(type) {
			case string:
				result[key] = []string{ev, val}
			case []string:
				result[key] = append(ev, val)
			}
		}
	}

	return result, nil
}


// fetchIssues retrieves issues with optional auto-pagination.
func fetchIssues(cmd *cobra.Command, searcher issueSearcher, searchReq *tracker.IssueSearchRequest,
	limit, page int, all bool) (*issueSearchResult, error) {
	if all {
		return fetchAllIssuePages(cmd, searcher, searchReq, limit)
	}

	opts := &tracker.IssueSearchOptions{}
	opts.Page = page
	opts.PerPage = limit

	issues, resp, err := searcher.Search(cmd.Context(), searchReq, opts)
	if err != nil {
		return nil, api.MapAPIError(err)
	}

	result := &issueSearchResult{issues: issues}
	if resp != nil {
		result.totalCount = resp.TotalCount
	}
	result.hasMore = len(issues) == limit
	if result.hasMore {
		result.nextCursor = strconv.Itoa(page + 1)
	}
	return result, nil
}

// fetchAllIssuePages auto-paginates through all issue pages.
func fetchAllIssuePages(cmd *cobra.Command, searcher issueSearcher,
	searchReq *tracker.IssueSearchRequest, limit int) (*issueSearchResult, error) {
	var allIssues []*tracker.Issue
	var totalCount int

	currentPage := 1
	for {
		opts := &tracker.IssueSearchOptions{}
		opts.Page = currentPage
		opts.PerPage = limit

		issues, resp, err := searcher.Search(cmd.Context(), searchReq, opts)
		if err != nil {
			return nil, api.MapAPIError(err)
		}

		allIssues = append(allIssues, issues...)
		if resp != nil {
			totalCount = resp.TotalCount
		}

		if len(issues) < limit {
			break
		}
		currentPage++
	}

	return &issueSearchResult{issues: allIssues, totalCount: totalCount}, nil
}

// renderListOutput renders the issue list in JSON, quiet, or table mode.
func renderListOutput(w io.Writer, result *issueSearchResult) error {
	if output.IsJSON() {
		return renderListJSON(w, result)
	}

	if output.IsQuiet() {
		keys := make([]string, len(result.issues))
		for i, issue := range result.issues {
			keys[i] = api.DerefString(issue.Key, "")
		}
		output.PrintQuiet(w, keys...)
		return nil
	}

	return renderListTable(w, result.issues)
}

// renderListJSON renders the issue list as paginated JSON.
func renderListJSON(w io.Writer, result *issueSearchResult) error {
	items := make([]issueListItem, len(result.issues))
	for i, issue := range result.issues {
		items[i] = toListItem(issue)
	}

	var data any
	if output.HasFieldSelection() {
		filtered := make([]map[string]any, len(items))
		for i, item := range items {
			filtered[i] = output.FilterFields(item, output.JSONFields)
		}
		data = output.PaginatedResult{
			Items: filtered,
			Pagination: output.PaginationMeta{
				Cursor: result.nextCursor, HasMore: result.hasMore, Total: result.totalCount,
			},
		}
	} else {
		data = output.PaginatedResult{
			Items: items,
			Pagination: output.PaginationMeta{
				Cursor: result.nextCursor, HasMore: result.hasMore, Total: result.totalCount,
			},
		}
	}

	if output.JQFilter != "" {
		return output.ApplyJQ(w, data, output.JQFilter)
	}
	return output.PrintJSON(w, data)
}

// renderListTable renders the issue list as a formatted table.
func renderListTable(w io.Writer, issues []*tracker.Issue) error {
	if len(issues) == 0 {
		_, err := fmt.Fprintln(w, "No issues found")
		return err
	}

	tbl := output.NewTable(w)
	tbl.AddHeader("KEY", "STATUS", "ASSIGNEE", "SUMMARY")

	for _, issue := range issues {
		key := api.DerefString(issue.Key, "-")
		statusVal := issueStatusDisplay(issue)
		assigneeVal := api.DerefUser(issue.Assignee, "-")
		summary := api.DerefString(issue.Summary, "-")

		// Truncate summary to fit terminal width.
		maxSummary := max(output.TerminalWidth()-tableReservedWidth, minColumnWidth)
		summary = output.TruncateDisplay(summary, maxSummary)

		// Apply status color if colors are enabled.
		if output.ColorsEnabled() {
			statusVal = colorizeStatus(issue, statusVal)
		}

		tbl.AddRow(key, statusVal, assigneeVal, summary)
	}

	tbl.Render()
	return nil
}

// toListItem converts a tracker.Issue to a clean JSON-serializable struct.
func toListItem(issue *tracker.Issue) issueListItem {
	item := issueListItem{
		Key:      api.DerefString(issue.Key, ""),
		Summary:  api.DerefString(issue.Summary, ""),
		Status:   issueStatusDisplay(issue),
		Assignee: api.DerefUser(issue.Assignee, ""),
	}

	if issue.Priority != nil {
		item.Priority = api.DerefString(issue.Priority.Display, "")
	}
	if issue.Type != nil {
		item.Type = api.DerefString(issue.Type.Display, "")
	}
	if issue.CreatedAt != nil {
		item.CreatedAt = issue.CreatedAt.Format(time.RFC3339)
	}
	if issue.UpdatedAt != nil {
		item.UpdatedAt = issue.UpdatedAt.Format(time.RFC3339)
	}

	return item
}

// issueStatusDisplay extracts the display name from an issue's status.
// Returns "-" if the status or its display fields are nil.
func issueStatusDisplay(issue *tracker.Issue) string {
	if issue.Status == nil {
		return "-"
	}
	if issue.Status.Display != nil {
		return *issue.Status.Display
	}
	if issue.Status.Key != nil {
		return *issue.Status.Key
	}
	return "-"
}

// colorizeStatus applies semantic color to a status string based on status type.
func colorizeStatus(issue *tracker.Issue, statusText string) string {
	if issue.Status == nil || issue.Status.Key == nil {
		return statusText
	}
	key := strings.ToLower(*issue.Status.Key)

	switch key {
	case "closed", "done", "resolved":
		return text.Colors{text.FgGreen}.Sprint(statusText)
	case "inprogress", "in_progress":
		return text.Colors{text.FgYellow}.Sprint(statusText)
	case "cancelled", "blocked":
		return text.Colors{text.FgRed}.Sprint(statusText)
	default:
		return statusText
	}
}
