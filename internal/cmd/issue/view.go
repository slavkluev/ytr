package issue

import (
	"fmt"
	"io"
	"time"

	"github.com/jedib0t/go-pretty/v6/text"
	"github.com/slavkluev/go-yandex-tracker/tracker"

	"github.com/spf13/cobra"

	"github.com/slavkluev/ytr/internal/api"
	"github.com/slavkluev/ytr/internal/cmd/jsonfields"
	"github.com/slavkluev/ytr/internal/config"
	"github.com/slavkluev/ytr/internal/output"
)

// IssueDetailFields lists the available JSON field names for issue detail output.
var IssueDetailFields = []string{
	"key",
	"summary",
	"status",
	"priority",
	"type",
	"author",
	"assignee",
	"createdAt",
	"updatedAt",
	"description",
}

// issueDetail is a clean struct for JSON serialization of a single issue.
// Uses value types with json tags to avoid null fields from pointer types.
type issueDetail struct {
	Key         string `json:"key"`
	Summary     string `json:"summary"`
	Status      string `json:"status"`
	Priority    string `json:"priority,omitempty"`
	Type        string `json:"type,omitempty"`
	Author      string `json:"author,omitempty"`
	Assignee    string `json:"assignee,omitempty"`
	CreatedAt   string `json:"createdAt,omitempty"`
	UpdatedAt   string `json:"updatedAt,omitempty"`
	Description string `json:"description,omitempty"`
}

// toIssueDetail converts a tracker.Issue into a clean issueDetail struct for JSON output.
func toIssueDetail(issue *tracker.Issue) issueDetail {
	detail := issueDetail{
		Key:      api.DerefString(issue.Key, ""),
		Summary:  api.DerefString(issue.Summary, ""),
		Status:   issueStatusDisplay(issue),
		Author:   api.DerefUser(issue.CreatedBy, ""),
		Assignee: api.DerefUser(issue.Assignee, ""),
	}
	if issue.Priority != nil {
		detail.Priority = api.DerefString(issue.Priority.Display, "")
	}
	if issue.Type != nil {
		detail.Type = api.DerefString(issue.Type.Display, "")
	}
	if issue.CreatedAt != nil {
		detail.CreatedAt = issue.CreatedAt.Format(time.RFC3339)
	}
	if issue.UpdatedAt != nil {
		detail.UpdatedAt = issue.UpdatedAt.Format(time.RFC3339)
	}
	if issue.Description != nil {
		detail.Description = *issue.Description
	}
	return detail
}

// newViewCmd creates the "issue view" command for displaying issue details.
func newViewCmd() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "view ISSUE-KEY",
		Short: "View issue details",
		Long: `Display detailed information about a Yandex Tracker issue.

JSON FIELDS
  key, summary, status, priority, type, author, assignee, createdAt, updatedAt, description

SEE ALSO
  ytr issue list        - List issues
  ytr issue update      - Update an issue
  ytr issue transition  - Transition issue status`,
		Example: `  # View issue details
  ytr issue view PROJ-123

  # Get specific fields as JSON
  ytr issue view PROJ-123 --json key,summary,status,assignee

  # Get just the description
  ytr issue view PROJ-123 --json description --jq '.description'`,
		Args: cobra.ExactArgs(1),
		RunE: runView,
	}

	jsonfields.Register("ytr issue view", IssueDetailFields)

	return cmd
}

// runView executes the issue view logic.
func runView(cmd *cobra.Command, args []string) error {
	// Handle --json field hint (no fields specified, D-10).
	if output.IsJSON() && !output.HasFieldSelection() && output.JQFilter == "" {
		return output.PrintFieldHint(cmd.ErrOrStderr(), "issue view", IssueDetailFields)
	}

	// If --jq without --json: auto-populate all fields (Pitfall 5).
	if output.JQFilter != "" && !output.HasFieldSelection() {
		output.JSONFields = IssueDetailFields
	}

	// Validate requested fields.
	if output.HasFieldSelection() {
		if err := output.ValidateFields(output.JSONFields, IssueDetailFields); err != nil {
			return err
		}
		output.JSONFields = output.NormalizeFields(output.JSONFields, IssueDetailFields)
	}

	issueKey := args[0]

	// Resolve auth from root persistent flags.
	tokenFlag, _ := cmd.Root().PersistentFlags().GetString("token")
	orgIDFlag, _ := cmd.Root().PersistentFlags().GetString("org-id")
	orgTypeFlag, _ := cmd.Root().PersistentFlags().GetString("org-type")

	auth, err := config.ResolveAuth(tokenFlag, orgIDFlag, orgTypeFlag)
	if err != nil {
		return err
	}

	getter := newGetter(auth)

	issue, _, err := getter.Get(cmd.Context(), issueKey, nil)
	if err != nil {
		return api.MapAPIError(err)
	}

	return renderDetailOutput(cmd.OutOrStdout(), issue)
}

// renderDetailOutput renders an issue in JSON, quiet, or table mode.
func renderDetailOutput(w io.Writer, issue *tracker.Issue) error {
	if output.IsJSON() {
		detail := toIssueDetail(issue)

		if output.HasFieldSelection() {
			filtered := output.FilterFields(detail, output.JSONFields)
			if output.JQFilter != "" {
				return output.ApplyJQ(w, filtered, output.JQFilter)
			}
			return output.PrintJSON(w, filtered)
		}
		if output.JQFilter != "" {
			return output.ApplyJQ(w, detail, output.JQFilter)
		}
		return output.PrintJSON(w, detail)
	}

	if output.IsQuiet() {
		output.PrintQuiet(w, api.DerefString(issue.Key, ""))
		return nil
	}

	return renderDetailTable(w, issue)
}

// renderDetailTable renders the issue as labeled key-value rows.
func renderDetailTable(w io.Writer, issue *tracker.Issue) error {
	bold := func(label string) string {
		if output.ColorsEnabled() {
			return text.Colors{text.Bold}.Sprint(label)
		}
		return label
	}

	// writeErr captures the first write error encountered.
	var writeErr error
	printField := func(label, value string) {
		if writeErr != nil {
			return
		}
		_, writeErr = fmt.Fprintf(w, "%s  %s\n", bold(label+":"), value)
	}

	printField("Key", api.DerefString(issue.Key, "-"))
	printField("Title", api.DerefString(issue.Summary, "-"))
	printField("Status", issueStatusDisplay(issue))

	priority := "-"
	if issue.Priority != nil {
		priority = api.DerefString(issue.Priority.Display, "-")
	}
	printField("Priority", priority)

	issueType := "-"
	if issue.Type != nil {
		issueType = api.DerefString(issue.Type.Display, "-")
	}
	printField("Type", issueType)

	printField("Author", api.DerefUser(issue.CreatedBy, "-"))
	printField("Assignee", api.DerefUser(issue.Assignee, "-"))

	created := "-"
	if issue.CreatedAt != nil {
		created = output.TimeAgo(issue.CreatedAt.Time)
	}
	printField("Created", created)

	updated := "-"
	if issue.UpdatedAt != nil {
		updated = output.TimeAgo(issue.UpdatedAt.Time)
	}
	printField("Updated", updated)

	if writeErr != nil {
		return writeErr
	}

	// Description with separator.
	if issue.Description != nil && *issue.Description != "" {
		if _, err := fmt.Fprintln(w); err != nil {
			return err
		}
		if _, err := fmt.Fprintf(w, "%s\n", bold("Description:")); err != nil {
			return err
		}
		if _, err := fmt.Fprintf(w, "  %s\n", *issue.Description); err != nil {
			return err
		}
	}

	return nil
}
