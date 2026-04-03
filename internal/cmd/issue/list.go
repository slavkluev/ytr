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
		queue    string
		status   string
		assignee string
		limit    int
		cursor   string
		all      bool
	)

	cmd := &cobra.Command{
		Use:   "list",
		Short: "List issues",
		Long: `Search and list Yandex Tracker issues with filtering and pagination.

JSON FIELDS
  key, summary, status, priority, type, assignee, createdAt, updatedAt

SEE ALSO
  ytr issue view      - View issue details
  ytr issue create    - Create a new issue`,
		Example: `  # List all issues in a queue
  ytr issue list --queue PROJ

  # List open issues assigned to me
  ytr issue list --queue PROJ --status open --assignee me

  # Get issue keys and statuses as JSON
  ytr issue list --queue PROJ --json key,summary,status

  # Extract just keys with jq
  ytr issue list --queue PROJ --json key --jq '.items[].key'`,
		RunE: func(cmd *cobra.Command, args []string) error {
			return runList(cmd, queue, status, assignee, limit, cursor, all)
		},
	}

	cmd.Flags().StringVar(&queue, "queue", "", "Filter by queue key")
	cmd.Flags().StringVar(&status, "status", "", "Filter by status (comma-separated)")
	cmd.Flags().StringVar(&assignee, "assignee", "", "Filter by assignee")
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
func runList(cmd *cobra.Command, queue, status, assignee string, limit int, cursor string, all bool) error {
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

	// Build filter map.
	filter := buildFilter(queue, status, assignee)

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

	searchReq := &tracker.IssueSearchRequest{Filter: filter}

	result, err := fetchIssues(cmd, searcher, searchReq, limit, page, all)
	if err != nil {
		return err
	}

	return renderListOutput(cmd.OutOrStdout(), result)
}

// buildFilter constructs the issue search filter map from flag values.
func buildFilter(queue, status, assignee string) map[string]any {
	filter := map[string]any{}
	if queue != "" {
		filter["queue"] = queue
	}
	if status != "" {
		parts := strings.Split(status, ",")
		for i := range parts {
			parts[i] = strings.TrimSpace(parts[i])
		}
		if len(parts) == 1 {
			filter["status"] = parts[0]
		} else {
			filter["status"] = parts
		}
	}
	if assignee != "" {
		if assignee == "me" {
			assignee = "me()"
		}
		filter["assignee"] = assignee
	}
	return filter
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
