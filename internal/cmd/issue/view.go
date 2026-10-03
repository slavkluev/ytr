package issue

import (
	"context"
	"time"

	"github.com/slavkluev/go-yandex-tracker/tracker"
	"github.com/spf13/cobra"

	"github.com/slavkluev/ytr/internal/api"
	"github.com/slavkluev/ytr/internal/cmd/runner"
	"github.com/slavkluev/ytr/internal/output"
)

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

// IssueDetailFields are the --json fields of issue view, create and update.
var IssueDetailFields = runner.ItemFields[issueDetail]()

func toIssueDetail(issue *tracker.Issue) issueDetail {
	detail := issueDetail{
		Key:        api.DerefString(issue.Key, ""),
		Summary:    api.DerefString(issue.Summary, ""),
		Status:     issueStatusDisplay(issue),
		Author:     issue.CreatedBy.DisplayOr(""),
		AuthorID:   issue.CreatedBy.IDOr(""),
		Assignee:   issue.Assignee.DisplayOr(""),
		AssigneeID: issue.Assignee.IDOr(""),
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
	return runner.Get[*tracker.Issue, issueDetail]{
		Use:   "view ISSUE-KEY",
		Short: "View issue details",
		Long:  `Display detailed information about a Yandex Tracker issue.`,
		Example: `  # View issue details
  ytr issue view PROJ-123

  # Get specific fields as JSON
  ytr issue view PROJ-123 --json key,summary,status,assignee

  # Get just the description
  ytr issue view PROJ-123 --json description --jq '.description'`,
		Args: []runner.Arg{runner.IssueKey},
		Call: func(ctx context.Context, c *tracker.Client, args []string) (*tracker.Issue, error) {
			issue, _, err := c.Issues.Get(ctx, args[0], nil)
			return issue, err
		},
		Item:   toIssueDetail,
		Detail: issueCard,
		Quiet:  func(issue *tracker.Issue) string { return api.DerefString(issue.Key, "") },
	}.Command()
}

func issueCard(d *output.DetailPrinter, opts *output.Options, issue *tracker.Issue) {
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

	d.Field("Author", issue.CreatedBy.DisplayOr("-"))
	d.Field("Assignee", issue.Assignee.DisplayOr("-"))

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
}
