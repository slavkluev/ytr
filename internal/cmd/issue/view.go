package issue

import (
	"io"
	"time"

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
	"authorId",
	"assignee",
	"assigneeId",
	"createdAt",
	"updatedAt",
	"description",
}

// Uses value types with json tags to avoid null fields from pointer types.
type issueDetail struct {
	Key         string `json:"key"`
	Summary     string `json:"summary"`
	Status      string `json:"status"`
	Priority    string `json:"priority,omitempty"`
	Type        string `json:"type,omitempty"`
	Author      string `json:"author,omitempty"`
	AuthorID    string `json:"authorId"`
	Assignee    string `json:"assignee,omitempty"`
	AssigneeID  string `json:"assigneeId"`
	CreatedAt   string `json:"createdAt,omitempty"`
	UpdatedAt   string `json:"updatedAt,omitempty"`
	Description string `json:"description,omitempty"`
}

func toIssueDetail(issue *tracker.Issue) issueDetail {
	detail := issueDetail{
		Key:        api.DerefString(issue.Key, ""),
		Summary:    api.DerefString(issue.Summary, ""),
		Status:     issueStatusDisplay(issue),
		Author:     api.DerefUser(issue.CreatedBy, ""),
		AuthorID:   api.DerefUserID(issue.CreatedBy, ""),
		Assignee:   api.DerefUser(issue.Assignee, ""),
		AssigneeID: api.DerefUserID(issue.Assignee, ""),
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

func newViewCmd() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "view ISSUE-KEY",
		Short: "View issue details",
		Long: `Display detailed information about a Yandex Tracker issue.

JSON FIELDS
  key, summary, status, priority, type, author, authorId, assignee, assigneeId, createdAt, updatedAt, description

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

func runView(cmd *cobra.Command, args []string) error {
	opts := output.FromContext(cmd.Context())

	if opts.WantsFieldHint(cmd.Flags().Changed("json")) {
		return output.PrintFieldHint(cmd.ErrOrStderr(), "issue view", IssueDetailFields)
	}

	if opts.JQFilter != "" && !opts.HasFieldSelection() {
		opts.JSONFields = IssueDetailFields
	}

	if opts.HasFieldSelection() {
		if err := output.ValidateFields(opts.JSONFields, IssueDetailFields); err != nil {
			return err
		}
		opts.JSONFields = output.NormalizeFields(opts.JSONFields, IssueDetailFields)
	}

	issueKey := args[0]

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

	return renderDetailOutput(cmd.OutOrStdout(), opts, issue)
}

func renderDetailOutput(w io.Writer, opts *output.Options, issue *tracker.Issue) error {
	if opts.IsJSON() {
		detail := toIssueDetail(issue)

		if opts.HasFieldSelection() {
			filtered := output.FilterFields(detail, opts.JSONFields)
			if opts.JQFilter != "" {
				return output.ApplyJQ(w, filtered, opts.JQFilter)
			}
			return opts.PrintJSON(w, filtered)
		}
		if opts.JQFilter != "" {
			return output.ApplyJQ(w, detail, opts.JQFilter)
		}
		return opts.PrintJSON(w, detail)
	}

	if opts.Quiet {
		output.PrintQuiet(w, api.DerefString(issue.Key, ""))
		return nil
	}

	return renderDetailTable(w, opts, issue)
}

func renderDetailTable(w io.Writer, opts *output.Options, issue *tracker.Issue) error {
	d := opts.NewDetail(w)

	d.Field("Key", api.DerefString(issue.Key, "-"))
	d.Field("Title", api.DerefString(issue.Summary, "-"))
	d.Field("Status", issueStatusDisplay(issue))

	priority := "-"
	if issue.Priority != nil {
		priority = api.DerefString(issue.Priority.Display, "-")
	}
	d.Field("Priority", priority)

	issueType := "-"
	if issue.Type != nil {
		issueType = api.DerefString(issue.Type.Display, "-")
	}
	d.Field("Type", issueType)

	d.Field("Author", api.DerefUser(issue.CreatedBy, "-"))
	d.Field("Assignee", api.DerefUser(issue.Assignee, "-"))

	created := "-"
	if issue.CreatedAt != nil {
		created = opts.FormatTime(issue.CreatedAt.Time)
	}
	d.Field("Created", created)

	updated := "-"
	if issue.UpdatedAt != nil {
		updated = opts.FormatTime(issue.UpdatedAt.Time)
	}
	d.Field("Updated", updated)

	if issue.Description != nil && *issue.Description != "" {
		d.Block("Description", *issue.Description)
	}

	return d.Err()
}
