package bulk

import (
	"time"

	"github.com/slavkluev/go-yandex-tracker/tracker"
	"github.com/spf13/cobra"

	"github.com/slavkluev/ytr/internal/api"
	"github.com/slavkluev/ytr/internal/cmd/runner"
	"github.com/slavkluev/ytr/internal/config"
	"github.com/slavkluev/ytr/internal/output"
	"github.com/slavkluev/ytr/internal/validate"
)

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
  executionIssuePercent, executionChunkPercent, createdBy, createdById, createdAt`,
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
			return updateBody.CheckFlags(cmd.Flags().Changed)
		},
		RunE: func(cmd *cobra.Command, args []string) error {
			return runUpdate(cmd, args, fieldFlags, fromJSON, timeoutFlag)
		},
	}

	cmd.Flags().StringArrayVar(&fieldFlags, "field", nil, "Field to update (key=value, repeatable)")
	cmd.Flags().StringVar(&fromJSON, "from-json", "", "Full JSON request body (inline, @file, or - for stdin)")
	cmd.Flags().DurationVar(&timeoutFlag, "timeout", defaultTimeout, "Maximum time to wait for completion")

	runner.SetFields(cmd, BulkStatusFields)

	return cmd
}

var updateBody = validate.Body{
	Flags:    []validate.BodyFlag{fieldFlag},
	Required: []string{fieldFlag.Key},
	FromJSON: true,
}

var fieldFlag = validate.BodyFlag{Name: "field", Key: "values"}

func runUpdate(
	cmd *cobra.Command,
	args []string,
	fields []string,
	fromJSON string,
	timeout time.Duration,
) error {
	opts := output.FromContext(cmd.Context())

	if opts.WantsFieldHint(cmd.Flags().Changed("json")) {
		return output.PrintFieldHint(cmd.ErrOrStderr(), "bulk update", BulkStatusFields)
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

	return awaitBulkCompletion(cmd, opts, newBulkStatusGetter(auth), bc, timeout)
}

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
		if err := validate.UnmarshalRequestJSON(data, req); err != nil {
			return nil, err
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
