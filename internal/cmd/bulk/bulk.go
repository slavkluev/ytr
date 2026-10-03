// Package bulk provides bulk change commands for the ytr CLI.
package bulk

import (
	"context"
	"time"

	"github.com/slavkluev/go-yandex-tracker/tracker"
	"github.com/spf13/cobra"

	"github.com/slavkluev/ytr/internal/api"
	"github.com/slavkluev/ytr/internal/cmd/runner"
	"github.com/slavkluev/ytr/internal/config"
)

type bulkMover interface {
	Move(
		ctx context.Context,
		move *tracker.BulkMoveRequest,
	) (*tracker.BulkChange, *tracker.Response, error)
}

type bulkUpdater interface {
	Update(
		ctx context.Context,
		update *tracker.BulkUpdateRequest,
	) (*tracker.BulkChange, *tracker.Response, error)
}

type bulkTransitioner interface {
	Transition(
		ctx context.Context,
		transition *tracker.BulkTransitionRequest,
	) (*tracker.BulkChange, *tracker.Response, error)
}

type bulkStatusGetter interface {
	GetStatus(
		ctx context.Context,
		bulkChangeID string,
	) (*tracker.BulkChange, *tracker.Response, error)
}

var newBulkMover = func(auth *config.ResolvedAuth) bulkMover {
	return api.NewClient(auth).BulkChange
}

var newBulkUpdater = func(auth *config.ResolvedAuth) bulkUpdater {
	return api.NewClient(auth).BulkChange
}

var newBulkTransitioner = func(auth *config.ResolvedAuth) bulkTransitioner {
	return api.NewClient(auth).BulkChange
}

var newBulkStatusGetter = func(auth *config.ResolvedAuth) bulkStatusGetter {
	return api.NewClient(auth).BulkChange
}

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

	return detail
}

// NewCmd creates the parent "bulk" command with subcommands.
func NewCmd() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "bulk",
		Short: "Perform bulk operations on issues",
		Long: `Perform bulk operations on multiple Yandex Tracker issues at once.

Bulk commands accept issue keys as positional arguments or via stdin pipe
(one per line). Commands wait for completion by default with progress display.`,
	}

	cmd.AddCommand(newStatusCmd())
	cmd.AddCommand(newMoveCmd())
	cmd.AddCommand(newUpdateCmd())
	cmd.AddCommand(newTransitionCmd())

	return cmd
}
