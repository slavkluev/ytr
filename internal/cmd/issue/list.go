package issue

import (
	"context"
	"fmt"
	"iter"
	"strings"
	"time"

	"github.com/slavkluev/go-yandex-tracker/tracker"
	"github.com/spf13/cobra"
	"github.com/spf13/pflag"

	"github.com/slavkluev/ytr/internal/api"
	"github.com/slavkluev/ytr/internal/cmd/runner"
	ytrerrors "github.com/slavkluev/ytr/internal/errors"
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

  # Only keys, summaries and statuses
  ytr issue list --filter queue=PROJ --json key,summary,status

  # Extract just keys with jq
  ytr issue list --filter queue=PROJ --json key --jq '.items[].key'`,
		Check: func(set *pflag.FlagSet) error {
			var err error
			search, err = searchRequest(set)
			return err
		},
		Page: func(ctx context.Context, c *tracker.Client, o tracker.ListOptions) (
			[]*tracker.Issue, *tracker.Response, error,
		) {
			return c.Issues.Search(ctx, search, &tracker.IssueSearchOptions{ListOptions: o})
		},
		All: func(ctx context.Context, c *tracker.Client, o tracker.ListOptions) iter.Seq2[*tracker.Issue, error] {
			return c.Issues.SearchIter(ctx, search, &tracker.IssueSearchOptions{ListOptions: o})
		},
		Item: toListItem,
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
