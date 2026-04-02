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

// newMoveCmd creates the "bulk move" command.
func newMoveCmd() *cobra.Command {
	var (
		queueFlag   string
		fieldFlags  []string
		fromJSON    string
		timeoutFlag time.Duration
	)

	cmd := &cobra.Command{
		Use:   "move [ISSUE-KEY...]",
		Short: "Move issues to another queue",
		Long: `Move multiple Yandex Tracker issues to a different queue in a single
bulk operation.

Issue keys can be provided as positional arguments or piped via stdin
(one per line). The command waits for the operation to complete by default.

JSON FIELDS
  id, status, statusText, totalIssues, totalCompletedIssues,
  executionIssuePercent, executionChunkPercent, createdBy, createdAt

SEE ALSO
  ytr bulk status      - Show bulk operation status
  ytr bulk update      - Update fields on issues
  ytr bulk transition  - Transition issues to a new status`,
		Example: `  # Move issues to another queue
  ytr bulk move PROJ-1 PROJ-2 PROJ-3 --queue TARGET

  # Move via stdin pipe
  ytr issue list --quiet | ytr bulk move --queue TARGET

  # Move with field updates
  ytr bulk move PROJ-1 PROJ-2 --queue TARGET --field priority=critical

  # Move via JSON (advanced options like moveAllFields)
  ytr bulk move --from-json '{"queue":"TARGET","issues":["PROJ-1"],"moveAllFields":true}'`,
		Args: cobra.ArbitraryArgs,
		PreRunE: func(cmd *cobra.Command, _ []string) error {
			return validateMoveFlags(cmd)
		},
		RunE: func(cmd *cobra.Command, args []string) error {
			return runMove(cmd, args, queueFlag, fieldFlags, fromJSON, timeoutFlag)
		},
	}

	cmd.Flags().StringVar(&queueFlag, "queue", "", "Target queue key (required unless --from-json)")
	cmd.Flags().StringArrayVar(&fieldFlags, "field", nil, "Field to update (key=value, repeatable)")
	cmd.Flags().StringVar(&fromJSON, "from-json", "", "Full JSON request body (inline, @file, or - for stdin)")
	cmd.Flags().DurationVar(&timeoutFlag, "timeout", defaultTimeout, "Maximum time to wait for completion")

	jsonfields.Register("ytr bulk move", BulkStatusFields)

	return cmd
}

// validateMoveFlags checks mutual exclusion and required flags for bulk move.
func validateMoveFlags(cmd *cobra.Command) error {
	if cmd.Flags().Changed("from-json") &&
		(cmd.Flags().Changed("queue") || cmd.Flags().Changed("field")) {
		return errors.NewUserError(
			"cannot use --queue/--field and --from-json together",
			"Use --queue and --field for individual flags, or --from-json for full JSON input",
		)
	}

	if !cmd.Flags().Changed("from-json") && !cmd.Flags().Changed("queue") {
		return errors.NewUserError(
			"--queue is required",
			"Provide --queue with the target queue key, or use --from-json for full JSON input",
		)
	}

	return nil
}

// runMove executes the bulk move logic.
func runMove(
	cmd *cobra.Command,
	args []string,
	queue string,
	fields []string,
	fromJSON string,
	timeout time.Duration,
) error {
	// Handle --json field hint (no fields specified, D-10).
	if output.IsJSON() && !output.HasFieldSelection() && output.JQFilter == "" {
		return output.PrintFieldHint(cmd.ErrOrStderr(), "bulk move", BulkStatusFields)
	}

	// If --jq without --json: auto-populate all fields (Pitfall 5).
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

	req, err := buildMoveRequest(cmd, args, queue, fields, fromJSON)
	if err != nil {
		return err
	}

	mover := newBulkMover(auth)

	bc, _, err := mover.Move(cmd.Context(), req)
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

// buildMoveRequest builds a BulkMoveRequest from flags or JSON input.
func buildMoveRequest(
	cmd *cobra.Command,
	args []string,
	queue string,
	fields []string,
	fromJSON string,
) (*tracker.BulkMoveRequest, error) {
	if cmd.Flags().Changed("from-json") {
		data, err := validate.ParseJSONInput(fromJSON)
		if err != nil {
			return nil, err
		}

		req := &tracker.BulkMoveRequest{}
		if err := json.Unmarshal(data, req); err != nil {
			return nil, errors.NewUserError(
				fmt.Sprintf("invalid JSON input: %s", err),
				"Provide valid JSON matching the BulkMoveRequest format",
			)
		}

		return req, nil
	}

	keys, err := readIssueKeys(args)
	if err != nil {
		return nil, err
	}

	var values map[string]any
	if len(fields) > 0 {
		values, err = parseFieldFlags(fields)
		if err != nil {
			return nil, err
		}
	}

	return &tracker.BulkMoveRequest{
		Queue:  &queue,
		Issues: keys,
		Values: values,
	}, nil
}

// handlePollError converts a poll error to a user-friendly error.
func handlePollError(ctx context.Context, err error, timeout time.Duration, operationID string) error {
	if ctx.Err() != nil {
		return errors.NewUserError(
			fmt.Sprintf("bulk operation timed out after %s (operation ID: %s)", timeout, operationID),
			"Check status with: ytr bulk status "+operationID,
		)
	}

	return err
}
