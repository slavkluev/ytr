// Package bulk provides bulk change commands for the ytr CLI.
package bulk

import (
	"context"
	"time"

	"github.com/slavkluev/go-yandex-tracker/tracker"
	"github.com/spf13/cobra"

	"github.com/slavkluev/ytr/internal/api"
	"github.com/slavkluev/ytr/internal/config"
)

// bulkMover abstracts bulk move operations for testability.
type bulkMover interface {
	Move(
		ctx context.Context,
		move *tracker.BulkMoveRequest,
	) (*tracker.BulkChange, *tracker.Response, error)
}

// bulkUpdater abstracts bulk update operations for testability.
type bulkUpdater interface {
	Update(
		ctx context.Context,
		update *tracker.BulkUpdateRequest,
	) (*tracker.BulkChange, *tracker.Response, error)
}

// bulkTransitioner abstracts bulk transition operations for testability.
type bulkTransitioner interface {
	Transition(
		ctx context.Context,
		transition *tracker.BulkTransitionRequest,
	) (*tracker.BulkChange, *tracker.Response, error)
}

// bulkStatusGetter abstracts bulk status polling for testability.
type bulkStatusGetter interface {
	GetStatus(
		ctx context.Context,
		bulkChangeID string,
	) (*tracker.BulkChange, *tracker.Response, error)
}

// newBulkMover creates a bulkMover from resolved auth. Replaceable for testing.
var newBulkMover = func(auth *config.ResolvedAuth) bulkMover {
	return api.NewClient(auth).BulkChange
}

// newBulkUpdater creates a bulkUpdater from resolved auth. Replaceable for testing.
var newBulkUpdater = func(auth *config.ResolvedAuth) bulkUpdater {
	return api.NewClient(auth).BulkChange
}

// newBulkTransitioner creates a bulkTransitioner from resolved auth. Replaceable for testing.
var newBulkTransitioner = func(auth *config.ResolvedAuth) bulkTransitioner {
	return api.NewClient(auth).BulkChange
}

// newBulkStatusGetter creates a bulkStatusGetter from resolved auth. Replaceable for testing.
var newBulkStatusGetter = func(auth *config.ResolvedAuth) bulkStatusGetter {
	return api.NewClient(auth).BulkChange
}

// BulkStatusFields lists the available JSON field names for bulk status output.
var BulkStatusFields = []string{
	"id",
	"status",
	"statusText",
	"totalIssues",
	"totalCompletedIssues",
	"executionIssuePercent",
	"executionChunkPercent",
	"createdBy",
	"createdAt",
}

// bulkChangeDetail is a clean struct for JSON serialization of BulkChange data.
type bulkChangeDetail struct {
	ID                    string `json:"id"`
	Status                string `json:"status"`
	StatusText            string `json:"statusText"`
	TotalIssues           int    `json:"totalIssues"`
	TotalCompletedIssues  int    `json:"totalCompletedIssues"`
	ExecutionIssuePercent int    `json:"executionIssuePercent"`
	ExecutionChunkPercent int    `json:"executionChunkPercent"`
	CreatedBy             string `json:"createdBy"`
	CreatedAt             string `json:"createdAt"`
}

// toBulkChangeDetail converts a tracker.BulkChange to a clean JSON struct.
func toBulkChangeDetail(bc *tracker.BulkChange) bulkChangeDetail {
	detail := bulkChangeDetail{
		ID:                    api.DerefFlexString(bc.ID, ""),
		Status:                api.DerefString(bc.Status, ""),
		StatusText:            api.DerefString(bc.StatusText, ""),
		TotalIssues:           api.DerefInt(bc.TotalIssues, 0),
		TotalCompletedIssues:  api.DerefInt(bc.TotalCompletedIssues, 0),
		ExecutionIssuePercent: api.DerefInt(bc.ExecutionIssuePercent, 0),
		ExecutionChunkPercent: api.DerefInt(bc.ExecutionChunkPercent, 0),
		CreatedBy:             api.DerefUser(bc.CreatedBy, ""),
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
(one per line). Commands wait for completion by default with progress display.

SEE ALSO
  ytr bulk status      - Show bulk operation status
  ytr bulk move        - Move issues to another queue
  ytr bulk update      - Update fields on issues
  ytr bulk transition  - Transition issues to a new status`,
	}

	cmd.AddCommand(newStatusCmd())
	cmd.AddCommand(newMoveCmd())
	cmd.AddCommand(newUpdateCmd())
	cmd.AddCommand(newTransitionCmd())

	return cmd
}
