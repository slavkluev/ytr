package bulk

import (
	"github.com/spf13/cobra"

	"github.com/slavkluev/ytr/internal/api"
	"github.com/slavkluev/ytr/internal/cmd/jsonfields"
	"github.com/slavkluev/ytr/internal/config"
	"github.com/slavkluev/ytr/internal/output"
	"github.com/slavkluev/ytr/internal/validate"
)

// newStatusCmd creates the "bulk status" command.
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
  executionIssuePercent, executionChunkPercent, createdBy, createdAt

SEE ALSO
  ytr bulk move        - Move issues to another queue
  ytr bulk update      - Update fields on issues
  ytr bulk transition  - Transition issues to a new status`,
		Example: `  # Check operation status
  ytr bulk status 593cd211ef7e8a0000000001

  # Get status as JSON with specific fields
  ytr bulk status 593cd211ef7e8a0000000001 --json id,status,totalIssues

  # Get just the operation ID (quiet mode)
  ytr bulk status 593cd211ef7e8a0000000001 --quiet`,
		Args: cobra.ExactArgs(1),
		PreRunE: func(cmd *cobra.Command, args []string) error {
			_, err := validate.ValidateStringID(args[0], "operation ID")
			return err
		},
		RunE: func(cmd *cobra.Command, args []string) error {
			return runStatus(cmd, args[0])
		},
	}

	jsonfields.Register("ytr bulk status", BulkStatusFields)

	return cmd
}

// runStatus executes the bulk status logic.
func runStatus(cmd *cobra.Command, operationID string) error {
	if output.IsJSON() && !output.HasFieldSelection() && output.JQFilter == "" {
		return output.PrintFieldHint(
			cmd.ErrOrStderr(), "bulk status", BulkStatusFields,
		)
	}

	if output.JQFilter != "" && !output.HasFieldSelection() {
		output.JSONFields = BulkStatusFields
	}

	// Validate requested fields.
	if output.HasFieldSelection() {
		if err := output.ValidateFields(output.JSONFields, BulkStatusFields); err != nil {
			return err
		}
		output.JSONFields = output.NormalizeFields(output.JSONFields, BulkStatusFields)
	}

	// Resolve auth from root persistent flags.
	tokenFlag, _ := cmd.Root().PersistentFlags().GetString("token")
	orgIDFlag, _ := cmd.Root().PersistentFlags().GetString("org-id")
	orgTypeFlag, _ := cmd.Root().PersistentFlags().GetString("org-type")

	auth, err := config.ResolveAuth(tokenFlag, orgIDFlag, orgTypeFlag)
	if err != nil {
		return err
	}

	getter := newBulkStatusGetter(auth)

	bc, _, err := getter.GetStatus(cmd.Context(), operationID)
	if err != nil {
		return api.MapAPIError(err)
	}

	return renderBulkOutput(cmd, bc)
}
