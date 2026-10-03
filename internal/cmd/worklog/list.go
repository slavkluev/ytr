package worklog

import (
	"fmt"
	"io"
	"time"

	"github.com/slavkluev/go-yandex-tracker/tracker"
	"github.com/spf13/cobra"

	"github.com/slavkluev/ytr/internal/api"
	"github.com/slavkluev/ytr/internal/cmd/jsonfields"
	"github.com/slavkluev/ytr/internal/config"
	"github.com/slavkluev/ytr/internal/output"
	"github.com/slavkluev/ytr/internal/validate"
)

// WorklogFields lists the available JSON field names for worklog output.
var WorklogFields = []string{"id", "author", "authorId", "duration", "start", "comment"}

type worklogItem struct {
	ID       string `json:"id"`
	Author   string `json:"author"`
	AuthorID string `json:"authorId"`
	Duration string `json:"duration"`
	Start    string `json:"start"`
	Comment  string `json:"comment,omitempty"`
}

func newListCmd() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "list ISSUE-KEY",
		Short: "List worklogs on an issue",
		Long: `List all worklogs on a Yandex Tracker issue.

JSON FIELDS
  id, author, authorId, duration, start, comment

SEE ALSO
  ytr worklog create  - Create a worklog
  ytr worklog edit    - Edit a worklog
  ytr worklog delete  - Delete a worklog`,
		Example: `  # List worklogs on an issue
  ytr worklog list PROJ-123

  # Get worklogs as JSON
  ytr worklog list PROJ-123 --json id,duration,start

  # Extract durations with jq
  ytr worklog list PROJ-123 --jq '.[].duration'`,
		Args: cobra.ExactArgs(1),
		PreRunE: func(cmd *cobra.Command, args []string) error {
			return validate.ValidateIssueKey(args[0])
		},
		RunE: func(cmd *cobra.Command, args []string) error {
			return runList(cmd, args[0])
		},
	}

	jsonfields.Register("ytr worklog list", WorklogFields)

	return cmd
}

func runList(cmd *cobra.Command, issueKey string) error {
	opts := output.FromContext(cmd.Context())

	if opts.WantsFieldHint(cmd.Flags().Changed("json")) {
		return output.PrintFieldHint(cmd.ErrOrStderr(), "worklog list", WorklogFields)
	}

	if opts.JQFilter != "" && !opts.HasFieldSelection() {
		opts.JSONFields = WorklogFields
	}

	if opts.HasFieldSelection() {
		if err := output.ValidateFields(opts.JSONFields, WorklogFields); err != nil {
			return err
		}
		opts.JSONFields = output.NormalizeFields(opts.JSONFields, WorklogFields)
	}

	tokenFlag, _ := cmd.Root().PersistentFlags().GetString("token")
	orgIDFlag, _ := cmd.Root().PersistentFlags().GetString("org-id")
	orgTypeFlag, _ := cmd.Root().PersistentFlags().GetString("org-type")

	auth, err := config.ResolveAuth(tokenFlag, orgIDFlag, orgTypeFlag)
	if err != nil {
		return err
	}

	lister := newWorklogLister(auth)

	worklogs, _, err := lister.ListWorklogs(cmd.Context(), issueKey)
	if err != nil {
		return api.MapAPIError(err)
	}

	return renderListOutput(cmd.OutOrStdout(), opts, worklogs)
}

func renderListOutput(w io.Writer, opts *output.Options, worklogs []*tracker.Worklog) error {
	if opts.IsJSON() {
		items := make([]worklogItem, len(worklogs))
		for i, wl := range worklogs {
			items[i] = toWorklogItem(wl)
		}

		if opts.HasFieldSelection() {
			filtered := make([]map[string]any, len(items))
			for i, item := range items {
				filtered[i] = output.FilterFields(item, opts.JSONFields)
			}
			if opts.JQFilter != "" {
				return output.ApplyJQ(w, filtered, opts.JQFilter)
			}
			return opts.PrintJSON(w, filtered)
		}
		if opts.JQFilter != "" {
			return output.ApplyJQ(w, items, opts.JQFilter)
		}
		return opts.PrintJSON(w, items)
	}

	if opts.Quiet {
		ids := make([]string, len(worklogs))
		for i, wl := range worklogs {
			ids[i] = api.DerefFlexString(wl.ID, "")
		}
		output.PrintQuiet(w, ids...)
		return nil
	}

	if len(worklogs) == 0 {
		_, err := fmt.Fprintln(w, "No worklogs found")
		return err
	}

	tbl := opts.NewTable(w)
	tbl.AddHeader("ID", "AUTHOR", "DURATION", "START")

	for _, wl := range worklogs {
		id := api.DerefFlexString(wl.ID, "-")
		author := wl.CreatedBy.DisplayOr("-")
		duration := formatDuration(wl.Duration)
		start := "-"
		if wl.Start != nil {
			start = opts.FormatTime(wl.Start.Time)
		}
		tbl.AddRow(id, author, duration, start)
	}

	tbl.Render()
	return nil
}

func toWorklogItem(wl *tracker.Worklog) worklogItem {
	item := worklogItem{
		ID:       api.DerefFlexString(wl.ID, ""),
		Author:   wl.CreatedBy.DisplayOr(""),
		AuthorID: wl.CreatedBy.IDOr(""),
		Duration: formatDuration(wl.Duration),
		Comment:  api.DerefString(wl.Comment, ""),
	}

	if wl.Start != nil {
		item.Start = wl.Start.Format(time.RFC3339)
	}

	return item
}

func formatDuration(d *tracker.Duration) string {
	if d == nil {
		return "-"
	}

	return d.String()
}
