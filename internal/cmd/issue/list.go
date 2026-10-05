package issue

import (
	"cmp"
	"context"
	"fmt"
	"iter"
	"strings"
	"time"

	"github.com/jedib0t/go-pretty/v6/text"
	"github.com/slavkluev/go-yandex-tracker/tracker"
	"github.com/spf13/cobra"
	"github.com/spf13/pflag"

	"github.com/slavkluev/ytr/internal/api"
	"github.com/slavkluev/ytr/internal/cmd/runner"
	ytrerrors "github.com/slavkluev/ytr/internal/errors"
	"github.com/slavkluev/ytr/internal/output"
)

const (
	// tableReservedWidth is the space reserved for key (12), status (15),
	// assignee (15), and padding (9) in table output.
	tableReservedWidth = 51

	minColumnWidth = 10
)

// Raw tracker.Issue fields are pointer types that produce nulls in JSON;
// this struct uses value types with proper json tags.
type issueListItem struct {
	Key        string `json:"key"`
	Summary    string `json:"summary"`
	Status     string `json:"status"`
	Priority   string `json:"priority,omitempty"`
	Type       string `json:"type,omitempty"`
	Assignee   string `json:"assignee,omitempty"`
	AssigneeID string `json:"assigneeId"`
	CreatedAt  string `json:"createdAt,omitempty"`
	UpdatedAt  string `json:"updatedAt,omitempty"`
}

func newListCmd() *cobra.Command {
	var search *tracker.IssueSearchRequest

	cmd := runner.Pages[*tracker.Issue, issueListItem]{
		Use:   "list",
		Short: "List issues",
		Long: `Search and list Yandex Tracker issues with filtering and pagination.

Supports two search modes:
  - Structured filters: use --filter key=value (repeatable) for field filtering
  - Query language: use --query for full Tracker query language expressions

The two modes are mutually exclusive: --query cannot be combined with --filter.`,
		Example: `  # Filter by queue
  ytr issue list --filter queue=PROJ

  # Multiple filters
  ytr issue list --filter queue=PROJ --filter status=open --filter 'assignee=me()'

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
		Check: func(set *pflag.FlagSet) error {
			var err error
			search, err = searchRequest(set)
			return err
		},
		Empty: "No issues found",
		Page: func(ctx context.Context, c *tracker.Client, o tracker.ListOptions) (
			[]*tracker.Issue, *tracker.Response, error,
		) {
			return c.Issues.Search(ctx, search, &tracker.IssueSearchOptions{ListOptions: o})
		},
		All: func(ctx context.Context, c *tracker.Client, o tracker.ListOptions) iter.Seq2[*tracker.Issue, error] {
			return c.Issues.SearchIter(ctx, search, &tracker.IssueSearchOptions{ListOptions: o})
		},
		Item:   toListItem,
		Header: []string{"KEY", "STATUS", "ASSIGNEE", "SUMMARY"},
		Row:    issueRow,
		Quiet:  func(issue *tracker.Issue) string { return api.DerefString(issue.Key, "") },
	}.Command()

	cmd.Flags().String("query", "", "Search using Tracker query language (mutually exclusive with --filter)")
	cmd.Flags().StringArray("filter", nil, "Filter by field (key=value, repeatable)")
	cmd.Flags().String("order-by", "", "Sort by field name (e.g., updated, created, priority)")
	cmd.Flags().Bool("order-asc", false, "Sort ascending (default: descending)")

	return cmd
}

func searchRequest(set *pflag.FlagSet) (*tracker.IssueSearchRequest, error) {
	if err := validateSearchFlags(set); err != nil {
		return nil, err
	}

	search := &tracker.IssueSearchRequest{}

	if query, _ := set.GetString("query"); query != "" {
		search.Query = &query
	} else if filters, _ := set.GetStringArray("filter"); len(filters) > 0 {
		filter, err := parseFilterFlags(filters)
		if err != nil {
			return nil, err
		}
		search.Filter = filter
	}

	if orderBy, _ := set.GetString("order-by"); orderBy != "" {
		prefix := "-"
		if asc, _ := set.GetBool("order-asc"); asc {
			prefix = "+"
		}
		order := prefix + orderBy
		search.Order = &order
	}

	return search, nil
}

func validateSearchFlags(set *pflag.FlagSet) error {
	queryChanged := set.Changed("query")
	orderByChanged := set.Changed("order-by")

	if queryChanged && set.Changed("filter") {
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

	if set.Changed("order-asc") && !orderByChanged {
		return ytrerrors.NewUserError(
			"--order-asc requires --order-by",
			"Use --order-by to specify the sort field (e.g., --order-by updated --order-asc)",
		)
	}

	return nil
}

// Splits on the first = sign only, so values may contain =.
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

func issueRow(opts *output.Options, issue *tracker.Issue) []string {
	status := cmp.Or(issueStatusDisplay(issue), "-")
	if opts.Colors {
		status = colorizeStatus(issue, status)
	}

	return []string{
		api.DerefString(issue.Key, "-"),
		status,
		issue.Assignee.DisplayOr("-"),
		opts.FitColumn(api.DerefString(issue.Summary, "-"), tableReservedWidth, minColumnWidth),
	}
}

func toListItem(issue *tracker.Issue) issueListItem {
	item := issueListItem{
		Key:        api.DerefString(issue.Key, ""),
		Summary:    api.DerefString(issue.Summary, ""),
		Status:     issueStatusDisplay(issue),
		Assignee:   issue.Assignee.DisplayOr(""),
		AssigneeID: issue.Assignee.IDOr(""),
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

func issueStatusDisplay(issue *tracker.Issue) string {
	if issue.Status == nil {
		return ""
	}
	if issue.Status.Display != nil {
		return *issue.Status.Display
	}
	return api.DerefString(issue.Status.Key, "")
}

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
