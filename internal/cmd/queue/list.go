package queue

import (
	"fmt"
	"io"
	"strconv"

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
)

// QueueListFields lists the available JSON field names for queue list output.
var QueueListFields = []string{"key", "name", "lead"}

// queueItem is a clean struct for JSON serialization of queue list items.
// Raw tracker.Queue fields are pointer types that produce nulls in JSON;
// this struct uses value types with proper json tags.
type queueItem struct {
	Key  string `json:"key"`
	Name string `json:"name"`
	Lead string `json:"lead,omitempty"`
}

// newListCmd creates the "queue list" command with pagination.
func newListCmd() *cobra.Command {
	var (
		limit  int
		cursor string
		all    bool
	)

	cmd := &cobra.Command{
		Use:   "list",
		Short: "List queues",
		Long: `List Yandex Tracker queues with pagination.

JSON FIELDS
  key, name, lead

SEE ALSO
  ytr queue view    - View queue details`,
		Example: `  # List all queues
  ytr queue list

  # List queues as JSON
  ytr queue list --json key,name

  # Get all queue keys
  ytr queue list --all --json key --jq '.items[].key'`,
		RunE: func(cmd *cobra.Command, args []string) error {
			return runList(cmd, limit, cursor, all)
		},
	}

	cmd.Flags().IntVar(&limit, "limit", defaultLimit, "Maximum number of results per page (max 1000)")
	cmd.Flags().StringVar(&cursor, "cursor", "", "Page number for pagination")
	cmd.Flags().BoolVar(&all, "all", false, "Fetch all pages automatically")

	jsonfields.Register("ytr queue list", QueueListFields)

	return cmd
}

// queueSearchResult holds the pagination state from a list operation.
type queueSearchResult struct {
	queues     []*tracker.Queue
	totalCount int
	hasMore    bool
	nextCursor string
}

// runList executes the queue list logic.
func runList(cmd *cobra.Command, limit int, cursor string, all bool) error {
	if output.IsJSON() && !output.HasFieldSelection() && output.JQFilter == "" {
		return output.PrintFieldHint(cmd.ErrOrStderr(), "queue list", QueueListFields)
	}

	if output.JQFilter != "" && !output.HasFieldSelection() {
		output.JSONFields = QueueListFields
	}

	// Validate requested fields.
	if output.HasFieldSelection() {
		if err := output.ValidateFields(output.JSONFields, QueueListFields); err != nil {
			return err
		}
		output.JSONFields = output.NormalizeFields(output.JSONFields, QueueListFields)
	}

	// Resolve auth from root persistent flags.
	tokenFlag, _ := cmd.Root().PersistentFlags().GetString("token")
	orgIDFlag, _ := cmd.Root().PersistentFlags().GetString("org-id")
	orgTypeFlag, _ := cmd.Root().PersistentFlags().GetString("org-type")

	auth, err := config.ResolveAuth(tokenFlag, orgIDFlag, orgTypeFlag)
	if err != nil {
		return err
	}

	lister := newLister(auth)

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

	result, err := fetchQueues(cmd, lister, limit, page, all)
	if err != nil {
		return err
	}

	return renderListOutput(cmd.OutOrStdout(), result)
}

// fetchQueues retrieves queues with optional auto-pagination.
func fetchQueues(cmd *cobra.Command, lister queueLister, limit, page int,
	all bool) (*queueSearchResult, error) {
	if all {
		return fetchAllQueuePages(cmd, lister, limit)
	}

	opts := &tracker.QueueListOptions{}
	opts.Page = page
	opts.PerPage = limit

	queues, resp, err := lister.List(cmd.Context(), opts)
	if err != nil {
		return nil, api.MapAPIError(err)
	}

	result := &queueSearchResult{queues: queues}
	if resp != nil {
		result.totalCount = resp.TotalCount
	}
	result.hasMore = len(queues) == limit
	if result.hasMore {
		result.nextCursor = strconv.Itoa(page + 1)
	}
	return result, nil
}

// fetchAllQueuePages auto-paginates through all queue pages.
func fetchAllQueuePages(cmd *cobra.Command, lister queueLister,
	limit int) (*queueSearchResult, error) {
	var allQueues []*tracker.Queue
	var totalCount int

	currentPage := 1
	for {
		opts := &tracker.QueueListOptions{}
		opts.Page = currentPage
		opts.PerPage = limit

		queues, resp, err := lister.List(cmd.Context(), opts)
		if err != nil {
			return nil, api.MapAPIError(err)
		}

		allQueues = append(allQueues, queues...)
		if resp != nil {
			totalCount = resp.TotalCount
		}

		if len(queues) < limit {
			break
		}
		currentPage++
	}

	return &queueSearchResult{queues: allQueues, totalCount: totalCount}, nil
}

// renderListOutput renders the queue list in JSON, quiet, or table mode.
func renderListOutput(w io.Writer, result *queueSearchResult) error {
	if output.IsJSON() {
		return renderListJSON(w, result)
	}

	if output.IsQuiet() {
		keys := make([]string, len(result.queues))
		for i, q := range result.queues {
			keys[i] = api.DerefString(q.Key, "")
		}
		output.PrintQuiet(w, keys...)
		return nil
	}

	return renderListTable(w, result.queues)
}

// renderListJSON renders the queue list as paginated JSON.
func renderListJSON(w io.Writer, result *queueSearchResult) error {
	items := make([]queueItem, len(result.queues))
	for i, q := range result.queues {
		items[i] = toQueueItem(q)
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

// renderListTable renders the queue list as a formatted table.
func renderListTable(w io.Writer, queues []*tracker.Queue) error {
	if len(queues) == 0 {
		_, err := fmt.Fprintln(w, "No queues found")
		return err
	}

	tbl := output.NewTable(w)
	tbl.AddHeader("KEY", "NAME", "LEAD")

	for _, q := range queues {
		key := api.DerefString(q.Key, "-")
		name := api.DerefString(q.Name, "-")
		lead := api.DerefUser(q.Lead, "-")

		tbl.AddRow(key, name, lead)
	}

	tbl.Render()
	return nil
}

// toQueueItem converts a tracker.Queue to a clean JSON-serializable struct.
func toQueueItem(q *tracker.Queue) queueItem {
	return queueItem{
		Key:  api.DerefString(q.Key, ""),
		Name: api.DerefString(q.Name, ""),
		Lead: api.DerefUser(q.Lead, ""),
	}
}
