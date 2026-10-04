package bulk

import (
	"context"
	"fmt"
	"time"

	"github.com/slavkluev/go-yandex-tracker/tracker"
	"github.com/spf13/cobra"

	"github.com/slavkluev/ytr/internal/api"
	"github.com/slavkluev/ytr/internal/cmd/runner"
	"github.com/slavkluev/ytr/internal/errors"
	"github.com/slavkluev/ytr/internal/output"
	"github.com/slavkluev/ytr/internal/validate"
)

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
  executionIssuePercent, executionChunkPercent, createdBy, createdById, createdAt`,
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
			return moveBody.CheckFlags(cmd.Flags().Changed)
		},
		RunE: func(cmd *cobra.Command, args []string) error {
			return runMove(cmd, args, queueFlag, fieldFlags, fromJSON, timeoutFlag)
		},
	}

	cmd.Flags().StringVar(&queueFlag, "queue", "", "Target queue key (required unless --from-json)")
	cmd.Flags().StringArrayVar(&fieldFlags, "field", nil, "Field to update (key=value, repeatable)")
	cmd.Flags().StringVar(&fromJSON, "from-json", "", "Full JSON request body (inline, @file, or - for stdin)")
	cmd.Flags().DurationVar(&timeoutFlag, "timeout", defaultTimeout, "Maximum time to wait for completion")

	runner.SetFields(cmd, BulkStatusFields)

	return cmd
}

var moveBody = validate.Body{
	Flags:    []validate.BodyFlag{{Name: "queue", Key: "queue"}, fieldFlag},
	Required: []string{"queue"},
	FromJSON: true,
}

func runMove(
	cmd *cobra.Command,
	args []string,
	queue string,
	fields []string,
	fromJSON string,
	timeout time.Duration,
) error {
	opts := output.FromContext(cmd.Context())

	if opts.WantsFieldHint(cmd.Flags().Changed("json")) {
		return output.PrintFieldHint(cmd.ErrOrStderr(), "bulk move", BulkStatusFields)
	}

	if opts.JQFilter != "" && !opts.HasFieldSelection() {
		opts.JSONFields = BulkStatusFields
	}

	if opts.HasFieldSelection() {
		if err := output.ValidateFields(opts.JSONFields, BulkStatusFields); err != nil {
			return err
		}
		opts.JSONFields = output.NormalizeFields(opts.JSONFields, BulkStatusFields)
	}

	client, err := runner.Client(cmd)
	if err != nil {
		return err
	}

	req, err := buildMoveRequest(cmd, args, queue, fields, fromJSON)
	if err != nil {
		return err
	}

	bc, _, err := client.BulkChange.Move(cmd.Context(), req)
	if err != nil {
		return api.MapAPIError(err)
	}

	return awaitBulkCompletion(cmd, opts, client.BulkChange, bc, timeout)
}

func buildMoveRequest(
	cmd *cobra.Command,
	args []string,
	queue string,
	fields []string,
	fromJSON string,
) (*tracker.BulkMoveRequest, error) {
	if cmd.Flags().Changed("from-json") {
		data, err := validate.ParseJSONInputFrom(fromJSON, cmd.InOrStdin())
		if err != nil {
			return nil, err
		}

		req := &tracker.BulkMoveRequest{}
		if err := moveBody.Decode(data, req); err != nil {
			return nil, err
		}

		if err := checkIssues(req.Issues); err != nil {
			return nil, err
		}

		return req, nil
	}

	keys, err := readIssueKeys(args, cmd.InOrStdin())
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

func handlePollError(ctx context.Context, err error, timeout time.Duration, operationID string) error {
	if ctx.Err() != nil {
		return errors.NewUserError(
			fmt.Sprintf("bulk operation timed out after %s (operation ID: %s)", timeout, operationID),
			"ytr bulk status "+operationID,
		)
	}

	return err
}
