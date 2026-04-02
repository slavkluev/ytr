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

// newTransitionCmd creates the "bulk transition" command.
func newTransitionCmd() *cobra.Command {
	var (
		transitionFlag string
		fieldFlags     []string
		fromJSON       string
		timeoutFlag    time.Duration
	)

	cmd := &cobra.Command{
		Use:   "transition [ISSUE-KEY...]",
		Short: "Transition multiple issues to a new status",
		Long: `Transition multiple Yandex Tracker issues to a new status in a single
bulk operation.

Issue keys can be provided as positional arguments or piped via stdin
(one per line). The command waits for the operation to complete by default.

JSON FIELDS
  id, status, statusText, totalIssues, totalCompletedIssues,
  executionIssuePercent, executionChunkPercent, createdBy, createdAt

SEE ALSO
  ytr bulk status      - Show bulk operation status
  ytr bulk move        - Move issues to another queue
  ytr bulk update      - Update fields on issues`,
		Example: `  # Transition issues to resolved
  ytr bulk transition PROJ-1 PROJ-2 --transition close

  # Transition with field updates
  ytr bulk transition PROJ-1 --transition close --field resolution=fixed

  # Transition via stdin pipe
  ytr issue list --quiet | ytr bulk transition --transition close

  # Transition via JSON
  ytr bulk transition --from-json '{"transition":"close","issues":["PROJ-1"]}'`,
		Args: cobra.ArbitraryArgs,
		PreRunE: func(cmd *cobra.Command, _ []string) error {
			return validateTransitionFlags(cmd)
		},
		RunE: func(cmd *cobra.Command, args []string) error {
			return runTransition(cmd, args, transitionFlag, fieldFlags, fromJSON, timeoutFlag)
		},
	}

	cmd.Flags().StringVar(&transitionFlag, "transition", "", "Transition ID or key (required unless --from-json)")
	cmd.Flags().StringArrayVar(&fieldFlags, "field", nil, "Field to update (key=value, repeatable)")
	cmd.Flags().StringVar(&fromJSON, "from-json", "", "Full JSON request body (inline, @file, or - for stdin)")
	cmd.Flags().DurationVar(&timeoutFlag, "timeout", defaultTimeout, "Maximum time to wait for completion")

	jsonfields.Register("ytr bulk transition", BulkStatusFields)

	return cmd
}

// validateTransitionFlags checks mutual exclusion and required flags for bulk transition.
func validateTransitionFlags(cmd *cobra.Command) error {
	if cmd.Flags().Changed("from-json") &&
		(cmd.Flags().Changed("transition") || cmd.Flags().Changed("field")) {
		return errors.NewUserError(
			"cannot use --transition/--field and --from-json together",
			"Use --transition and --field for individual flags, or --from-json for full JSON input",
		)
	}

	if !cmd.Flags().Changed("from-json") && !cmd.Flags().Changed("transition") {
		return errors.NewUserError(
			"--transition is required",
			"Provide --transition with the transition ID, or use --from-json for full JSON input",
		)
	}

	return nil
}

// runTransition executes the bulk transition logic.
func runTransition(
	cmd *cobra.Command,
	args []string,
	transition string,
	fields []string,
	fromJSON string,
	timeout time.Duration,
) error {
	// Handle --json field hint (no fields specified, D-10).
	if output.IsJSON() && !output.HasFieldSelection() && output.JQFilter == "" {
		return output.PrintFieldHint(cmd.ErrOrStderr(), "bulk transition", BulkStatusFields)
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

	req, err := buildTransitionRequest(cmd, args, transition, fields, fromJSON)
	if err != nil {
		return err
	}

	transitioner := newBulkTransitioner(auth)

	bc, _, err := transitioner.Transition(cmd.Context(), req)
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

// buildTransitionRequest builds a BulkTransitionRequest from flags or JSON input.
func buildTransitionRequest(
	cmd *cobra.Command,
	args []string,
	transition string,
	fields []string,
	fromJSON string,
) (*tracker.BulkTransitionRequest, error) {
	if cmd.Flags().Changed("from-json") {
		data, err := validate.ParseJSONInput(fromJSON)
		if err != nil {
			return nil, err
		}

		req := &tracker.BulkTransitionRequest{}
		if err := json.Unmarshal(data, req); err != nil {
			return nil, errors.NewUserError(
				fmt.Sprintf("invalid JSON input: %s", err),
				"Provide valid JSON matching the BulkTransitionRequest format",
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

	return &tracker.BulkTransitionRequest{
		Transition: &transition,
		Issues:     keys,
		Values:     values,
	}, nil
}
