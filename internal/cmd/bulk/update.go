package bulk

import (
	"context"
	"encoding/json"
	"fmt"
	"time"

	"github.com/slavkluev/go-yandex-tracker/tracker"
	"github.com/spf13/cobra"

	"github.com/slavkluev/ytr/internal/api"
	"github.com/slavkluev/ytr/internal/cmd/jsonfields"
	"github.com/slavkluev/ytr/internal/config"
	"github.com/slavkluev/ytr/internal/errors"
	"github.com/slavkluev/ytr/internal/output"
	"github.com/slavkluev/ytr/internal/validate"
)

// newUpdateCmd creates the "bulk update" command.
func newUpdateCmd() *cobra.Command {
	var (
		fieldFlags  []string
		fromJSON    string
		timeoutFlag time.Duration
	)

	cmd := &cobra.Command{
		Use:   "update [ISSUE-KEY...]",
		Short: "Update fields on multiple issues",
		Long: `Update fields on multiple Yandex Tracker issues in a single bulk operation.

Issue keys can be provided as positional arguments or piped via stdin
(one per line). The command waits for the operation to complete by default.

JSON FIELDS
  id, status, statusText, totalIssues, totalCompletedIssues,
  executionIssuePercent, executionChunkPercent, createdBy, createdAt

SEE ALSO
  ytr bulk status      - Show bulk operation status
  ytr bulk move        - Move issues to another queue
  ytr bulk transition  - Transition issues to a new status`,
		Example: `  # Update priority on multiple issues
  ytr bulk update PROJ-1 PROJ-2 --field priority=critical

  # Update multiple fields
  ytr bulk update PROJ-1 PROJ-2 --field priority=critical --field assignee=user123

  # Update via stdin pipe
  ytr issue list --quiet | ytr bulk update --field status=done

  # Update via JSON
  ytr bulk update --from-json '{"issues":["PROJ-1"],"values":{"priority":"critical"}}'`,
		Args: cobra.ArbitraryArgs,
		PreRunE: func(cmd *cobra.Command, _ []string) error {
			return validateUpdateFlags(cmd)
		},
		RunE: func(cmd *cobra.Command, args []string) error {
			return runUpdate(cmd, args, fieldFlags, fromJSON, timeoutFlag)
		},
	}

	cmd.Flags().StringArrayVar(&fieldFlags, "field", nil, "Field to update (key=value, repeatable)")
	cmd.Flags().StringVar(&fromJSON, "from-json", "", "Full JSON request body (inline, @file, or - for stdin)")
	cmd.Flags().DurationVar(&timeoutFlag, "timeout", defaultTimeout, "Maximum time to wait for completion")

	jsonfields.Register("ytr bulk update", BulkStatusFields)

	return cmd
}

// validateUpdateFlags checks mutual exclusion and required flags for bulk update.
func validateUpdateFlags(cmd *cobra.Command) error {
	if cmd.Flags().Changed("from-json") && cmd.Flags().Changed("field") {
		return errors.NewUserError(
			"cannot use --field and --from-json together",
			"Use --field for individual flags, or --from-json for full JSON input",
		)
	}

	if !cmd.Flags().Changed("from-json") && !cmd.Flags().Changed("field") {
		return errors.NewUserError(
			"--field is required",
			"Provide at least one --field key=value, or use --from-json for full JSON input",
		)
	}

	return nil
}

// runUpdate executes the bulk update logic.
func runUpdate(
	cmd *cobra.Command,
	args []string,
	fields []string,
	fromJSON string,
	timeout time.Duration,
) error {
	if output.IsJSON() && !output.HasFieldSelection() && output.JQFilter == "" {
		return output.PrintFieldHint(cmd.ErrOrStderr(), "bulk update", BulkStatusFields)
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

	req, err := buildUpdateRequest(cmd, args, fields, fromJSON)
	if err != nil {
		return err
	}

	updater := newBulkUpdater(auth)

	bc, _, err := updater.Update(cmd.Context(), req)
	if err != nil {
		return api.MapAPIError(err)
	}

	operationID := api.DerefFlexString(bc.ID, "")

	ctx, cancel := context.WithTimeout(cmd.Context(), timeout)
	defer cancel()

	result, err := pollUntilDone(ctx, newBulkStatusGetter(auth), operationID, cmd.ErrOrStderr())
	if err != nil {
		return handlePollError(ctx, err, timeout, operationID)
	}

	return renderBulkOutput(cmd, result)
}

// buildUpdateRequest builds a BulkUpdateRequest from flags or JSON input.
func buildUpdateRequest(
	cmd *cobra.Command,
	args []string,
	fields []string,
	fromJSON string,
) (*tracker.BulkUpdateRequest, error) {
	if cmd.Flags().Changed("from-json") {
		data, err := validate.ParseJSONInput(fromJSON)
		if err != nil {
			return nil, err
		}

		req := &tracker.BulkUpdateRequest{}
		if err := json.Unmarshal(data, req); err != nil {
			return nil, errors.NewUserError(
				fmt.Sprintf("invalid JSON input: %s", err),
				"Provide valid JSON matching the BulkUpdateRequest format",
			)
		}

		return req, nil
	}

	keys, err := readIssueKeys(args)
	if err != nil {
		return nil, err
	}

	values, err := parseFieldFlags(fields)
	if err != nil {
		return nil, err
	}

	return &tracker.BulkUpdateRequest{
		Issues: keys,
		Values: values,
	}, nil
}
