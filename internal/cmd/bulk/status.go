package bulk

import (
	"github.com/spf13/cobra"

	"github.com/slavkluev/ytr/internal/api"
	"github.com/slavkluev/ytr/internal/cmd/runner"
	"github.com/slavkluev/ytr/internal/validate"
)

func newStatusCmd() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "status OPERATION-ID",
		Short: "Show bulk operation status",
		Long: `Show the status of a bulk change operation.

Displays progress information including total issues, completed issues,
and completion percentage. Use the operation ID returned by bulk move,
bulk update, or bulk transition commands.

JSON FIELDS
  id, status, statusText, totalIssues, totalCompletedIssues,
  executionIssuePercent, executionChunkPercent, createdBy, createdById, createdAt`,
		Example: `  # Check operation status
  ytr bulk status 593cd211ef7e8a0000000001

  # Get status as JSON with specific fields
  ytr bulk status 593cd211ef7e8a0000000001 --json id,status,totalIssues

  # Get just the operation ID (quiet mode)
  ytr bulk status 593cd211ef7e8a0000000001 --quiet`,
		Args: cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			return runStatus(cmd, args[0])
		},
	}

	runner.SetFields(cmd, BulkStatusFields)

	return cmd
}

func runStatus(cmd *cobra.Command, arg string) error {
	operationID, err := validate.ValidateStringID(arg, "operation ID")
	if err != nil {
		return err
	}

	opts, err := runner.SelectFields(cmd, BulkStatusFields)
	if err != nil {
		return err
	}

	client, err := runner.Client(cmd)
	if err != nil {
		return err
	}

	bc, _, err := client.BulkChange.GetStatus(cmd.Context(), operationID)
	if err != nil {
		return api.MapAPIError(err)
	}

	return renderBulkOutput(cmd, opts, bc)
}
