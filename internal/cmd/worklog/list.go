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
var WorklogFields = []string{"id", "author", "duration", "start", "comment"}

// worklogItem is a clean struct for JSON serialization of worklog data.
// Used by list, create, and edit commands.
type worklogItem struct {
	ID       string `json:"id"`
	Author   string `json:"author"`
	Duration string `json:"duration"`
	Start    string `json:"start"`
	Comment  string `json:"comment,omitempty"`
}

// newListCmd creates the "worklog list" command.
func newListCmd() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "list ISSUE-KEY",
		Short: "List worklogs on an issue",
		Long: `List all worklogs on a Yandex Tracker issue.

JSON FIELDS
  id, author, duration, start, comment

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

// runList executes the worklog list logic.
func runList(cmd *cobra.Command, issueKey string) error {
	if output.IsJSON() && !output.HasFieldSelection() && output.JQFilter == "" {
		return output.PrintFieldHint(cmd.ErrOrStderr(), "worklog list", WorklogFields)
	}

	if output.JQFilter != "" && !output.HasFieldSelection() {
		output.JSONFields = WorklogFields
	}

	// Validate requested fields.
	if output.HasFieldSelection() {
		if err := output.ValidateFields(output.JSONFields, WorklogFields); err != nil {
			return err
		}
		output.JSONFields = output.NormalizeFields(output.JSONFields, WorklogFields)
	}

	// Resolve auth from root persistent flags.
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

	return renderListOutput(cmd.OutOrStdout(), worklogs)
}

// renderListOutput handles JSON/quiet/table output for the worklog list result.
func renderListOutput(w io.Writer, worklogs []*tracker.Worklog) error {
	if output.IsJSON() {
		items := make([]worklogItem, len(worklogs))
		for i, wl := range worklogs {
			items[i] = toWorklogItem(wl)
		}

		if output.HasFieldSelection() {
			filtered := make([]map[string]any, len(items))
			for i, item := range items {
				filtered[i] = output.FilterFields(item, output.JSONFields)
			}
			if output.JQFilter != "" {
				return output.ApplyJQ(w, filtered, output.JQFilter)
			}
			return output.PrintJSON(w, filtered)
		}
		if output.JQFilter != "" {
			return output.ApplyJQ(w, items, output.JQFilter)
		}
		return output.PrintJSON(w, items)
	}

	if output.IsQuiet() {
		ids := make([]string, len(worklogs))
		for i, wl := range worklogs {
			ids[i] = api.DerefFlexString(wl.ID, "")
		}
		output.PrintQuiet(w, ids...)
		return nil
	}

	// Table output.
	if len(worklogs) == 0 {
		_, err := fmt.Fprintln(w, "No worklogs found")
		return err
	}

	tbl := output.NewTable(w)
	tbl.AddHeader("ID", "AUTHOR", "DURATION", "START")

	for _, wl := range worklogs {
		id := api.DerefFlexString(wl.ID, "-")
		author := api.DerefUser(wl.CreatedBy, "-")
		duration := formatDuration(wl.Duration)
		start := "-"
		if wl.Start != nil {
			start = output.TimeAgo(wl.Start.Time)
		}
		tbl.AddRow(id, author, duration, start)
	}

	tbl.Render()
	return nil
}

// toWorklogItem converts a tracker.Worklog to a clean JSON-serializable struct.
func toWorklogItem(wl *tracker.Worklog) worklogItem {
	item := worklogItem{
		ID:       api.DerefFlexString(wl.ID, ""),
		Author:   api.DerefUser(wl.CreatedBy, ""),
		Duration: formatDuration(wl.Duration),
		Comment:  api.DerefString(wl.Comment, ""),
	}

	if wl.Start != nil {
		item.Start = wl.Start.Format(time.RFC3339)
	}

	return item
}

// formatDuration formats a tracker.Duration as ISO 8601 string (e.g., PT1H30M).
// Returns "-" if d is nil or on error.
func formatDuration(d *tracker.Duration) string {
	if d == nil {
		return "-"
	}

	data, err := d.MarshalJSON()
	if err != nil {
		return "-"
	}

	// MarshalJSON returns quoted string like "PT1H30M", strip quotes.
	s := string(data)
	if len(s) >= 2 && s[0] == '"' && s[len(s)-1] == '"' {
		return s[1 : len(s)-1]
	}

	return s
}
