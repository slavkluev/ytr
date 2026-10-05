// Package bulk provides bulk change commands for the ytr CLI.
package bulk

import (
	"time"

	"github.com/slavkluev/go-yandex-tracker/tracker"
	"github.com/spf13/cobra"

	"github.com/slavkluev/ytr/internal/api"
	"github.com/slavkluev/ytr/internal/cmd/runner"
)

type bulkChangeDetail struct {
	ID                    string `json:"id"`
	Status                string `json:"status"`
	StatusText            string `json:"statusText"`
	TotalIssues           int    `json:"totalIssues"`
	TotalCompletedIssues  int    `json:"totalCompletedIssues"`
	ExecutionIssuePercent int    `json:"executionIssuePercent"`
	ExecutionChunkPercent int    `json:"executionChunkPercent"`
	CreatedBy             string `json:"createdBy"`
	CreatedByID           string `json:"createdById"`
	CreatedAt             string `json:"createdAt"`
	Suggestion            string `json:"suggestion"`
}

// BulkStatusFields are the --json fields of every bulk command.
var BulkStatusFields = runner.ItemFields[bulkChangeDetail]()

func toBulkChangeDetail(bc *tracker.BulkChange) bulkChangeDetail {
	detail := bulkChangeDetail{
		ID:                    api.DerefFlexString(bc.ID, ""),
		Status:                api.DerefString(bc.Status, ""),
		StatusText:            api.DerefString(bc.StatusText, ""),
		TotalIssues:           api.DerefInt(bc.TotalIssues, 0),
		TotalCompletedIssues:  api.DerefInt(bc.TotalCompletedIssues, 0),
		ExecutionIssuePercent: api.DerefInt(bc.ExecutionIssuePercent, 0),
		ExecutionChunkPercent: api.DerefInt(bc.ExecutionChunkPercent, 0),
		CreatedBy:             bc.CreatedBy.DisplayOr(""),
		CreatedByID:           bc.CreatedBy.IDOr(""),
	}

	if bc.CreatedAt != nil {
		detail.CreatedAt = bc.CreatedAt.Format(time.RFC3339)
	}

	if !finished(detail.Status) && detail.ID != "" {
		detail.Suggestion = "ytr bulk status " + detail.ID
	}

	return detail
}

// NewCmd creates the parent "bulk" command with subcommands.
func NewCmd() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "bulk",
		Short: "Perform bulk operations on issues",
		Long: `Perform bulk operations on multiple Yandex Tracker issues at once.

Bulk commands accept issue keys or 24-character hexadecimal issue IDs as
positional arguments or via stdin pipe (one per line). With --from-json, the
body's "issues" is the only source of issues: issue arguments are refused and
stdin is not read for issues. Commands wait up to --timeout for the operation
to finish, showing progress on a terminal.`,
	}

	cmd.AddCommand(newStatusCmd())
	cmd.AddCommand(newMoveCmd())
	cmd.AddCommand(newUpdateCmd())
	cmd.AddCommand(newTransitionCmd())

	return cmd
}
