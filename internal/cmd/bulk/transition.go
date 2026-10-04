package bulk

import (
	"time"

	"github.com/slavkluev/go-yandex-tracker/tracker"
	"github.com/spf13/cobra"

	"github.com/slavkluev/ytr/internal/api"
	"github.com/slavkluev/ytr/internal/cmd/runner"
	"github.com/slavkluev/ytr/internal/output"
	"github.com/slavkluev/ytr/internal/validate"
)

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
  executionIssuePercent, executionChunkPercent, createdBy, createdById, createdAt`,
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
			return transitionBody.CheckFlags(cmd.Flags().Changed)
		},
		RunE: func(cmd *cobra.Command, args []string) error {
			return runTransition(cmd, args, transitionFlag, fieldFlags, fromJSON, timeoutFlag)
		},
	}

	cmd.Flags().StringVar(&transitionFlag, "transition", "", "Transition ID or key (required unless --from-json)")
	cmd.Flags().StringArrayVar(&fieldFlags, "field", nil, "Field to update (key=value, repeatable)")
	cmd.Flags().StringVar(&fromJSON, "from-json", "", "Full JSON request body (inline, @file, or - for stdin)")
	cmd.Flags().DurationVar(&timeoutFlag, "timeout", defaultTimeout, "Maximum time to wait for completion")

	runner.SetFields(cmd, BulkStatusFields)

	return cmd
}

var transitionBody = validate.Body{
	Flags:    []validate.BodyFlag{{Name: "transition", Key: "transition"}, fieldFlag},
	Required: []string{"transition"},
	FromJSON: true,
}

func runTransition(
	cmd *cobra.Command,
	args []string,
	transition string,
	fields []string,
	fromJSON string,
	timeout time.Duration,
) error {
	opts := output.FromContext(cmd.Context())

	if opts.WantsFieldHint(cmd.Flags().Changed("json")) {
		return output.PrintFieldHint(cmd.ErrOrStderr(), "bulk transition", BulkStatusFields)
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

	req, err := buildTransitionRequest(cmd, args, transition, fields, fromJSON)
	if err != nil {
		return err
	}

	bc, _, err := client.BulkChange.Transition(cmd.Context(), req)
	if err != nil {
		return api.MapAPIError(err)
	}

	return awaitBulkCompletion(cmd, opts, client.BulkChange, bc, timeout)
}

func buildTransitionRequest(
	cmd *cobra.Command,
	args []string,
	transition string,
	fields []string,
	fromJSON string,
) (*tracker.BulkTransitionRequest, error) {
	if cmd.Flags().Changed("from-json") {
		data, err := validate.ParseJSONInputFrom(fromJSON, cmd.InOrStdin())
		if err != nil {
			return nil, err
		}

		req := &tracker.BulkTransitionRequest{}
		if err := transitionBody.Decode(data, req); err != nil {
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

	return &tracker.BulkTransitionRequest{
		Transition: &transition,
		Issues:     keys,
		Values:     values,
	}, nil
}
