package worklog

import (
	"encoding/json"
	"fmt"
	"io"
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

// newCreateCmd creates the "worklog create" command.
func newCreateCmd() *cobra.Command {
	var (
		durationFlag string
		startFlag    string
		commentFlag  string
		fromJSON     string
	)

	cmd := &cobra.Command{
		Use:   "create ISSUE-KEY",
		Short: "Create a worklog",
		Long: `Create a new worklog on a Yandex Tracker issue.

Durations use ISO 8601 format: PT1H30M (1h30m), PT45M (45min), P1D (1 day),
P1DT2H (1 day 2 hours).

Tracker requires both duration and start time when creating a worklog.

JSON FIELDS
  id, author, duration, start, comment

SEE ALSO
  ytr worklog list    - List worklogs on issue
  ytr worklog edit    - Edit a worklog
  ytr worklog delete  - Delete a worklog`,
		Example: `  # Log 1h30m of work
  ytr worklog create PROJ-123 --duration PT1H30M --start 2026-03-30T10:00:00Z

  # Log with comment and start time
  ytr worklog create PROJ-123 --duration PT2H --comment "Code review" --start 2026-03-30T10:00:00Z

  # Create via JSON
  ytr worklog create PROJ-123 --from-json '{"start":"2026-03-30T10:00:00Z","duration":"PT1H","comment":"Bug fix"}'`,
		Args: cobra.ExactArgs(1),
		PreRunE: func(cmd *cobra.Command, args []string) error {
			if err := validate.ValidateIssueKey(args[0]); err != nil {
				return err
			}

			// Mutual exclusion: --from-json vs individual flags.
			if cmd.Flags().Changed("from-json") &&
				(cmd.Flags().Changed("duration") || cmd.Flags().Changed("start") ||
					cmd.Flags().Changed("comment")) {
				return errors.NewUserError(
					"cannot use individual flags and --from-json together",
					"Use --duration, --start, --comment for individual flags, or --from-json for full JSON input",
				)
			}

			// Require --duration and --start unless using --from-json.
			if !cmd.Flags().Changed("from-json") && !cmd.Flags().Changed("duration") {
				return errors.NewUserError(
					"--duration is required",
					"Provide --duration with ISO 8601 format (e.g., PT1H30M), or --from-json for full JSON input",
				)
			}

			if !cmd.Flags().Changed("from-json") && !cmd.Flags().Changed("start") {
				return errors.NewUserError(
					"--start is required",
					"Provide --start in RFC 3339 format (e.g., 2026-03-30T10:00:00Z), or --from-json for full JSON input",
				)
			}

			return nil
		},
		RunE: func(cmd *cobra.Command, args []string) error {
			return runCreate(cmd, args[0], durationFlag, startFlag, commentFlag, fromJSON)
		},
	}

	cmd.Flags().StringVar(&durationFlag, "duration", "", "Duration in ISO 8601 format (e.g., PT1H30M) (required)")
	cmd.Flags().StringVar(
		&startFlag, "start", "",
		"Start time in RFC 3339 format (e.g., 2026-03-30T10:00:00Z) (required)",
	)
	cmd.Flags().StringVar(&commentFlag, "comment", "", "Worklog comment")
	cmd.Flags().StringVar(
		&fromJSON, "from-json", "",
		`JSON input: inline '{"start":"2026-03-30T10:00:00Z","duration":"PT1H"}', @file, or - for stdin`,
	)

	jsonfields.Register("ytr worklog create", WorklogFields)

	return cmd
}

// runCreate executes the worklog create logic.
func runCreate(
	cmd *cobra.Command,
	issueKey, durationFlag, startFlag, commentFlag, fromJSON string,
) error {
	// Handle --json field hint (no fields specified, D-10).
	if output.IsJSON() && !output.HasFieldSelection() && output.JQFilter == "" {
		return output.PrintFieldHint(cmd.ErrOrStderr(), "worklog create", WorklogFields)
	}

	// If --jq without --json: auto-populate all fields (Pitfall 5).
	if output.JQFilter != "" && !output.HasFieldSelection() {
		output.JSONFields = WorklogFields
	}

	// Validate requested fields.
	if output.HasFieldSelection() {
		if err := output.ValidateFields(output.JSONFields, WorklogFields); err != nil {
			return err
		}
		output.JSONFields = output.NormalizeFields(output.JSONFields, WorklogFields)
	}

	// Resolve auth from root persistent flags.
	tokenFlag, _ := cmd.Root().PersistentFlags().GetString("token")
	orgIDFlag, _ := cmd.Root().PersistentFlags().GetString("org-id")
	orgTypeFlag, _ := cmd.Root().PersistentFlags().GetString("org-type")

	auth, err := config.ResolveAuth(tokenFlag, orgIDFlag, orgTypeFlag)
	if err != nil {
		return err
	}

	req, err := buildCreateRequest(cmd, durationFlag, startFlag, commentFlag, fromJSON)
	if err != nil {
		return err
	}
	if validErr := validateCreateRequest(req); validErr != nil {
		return validErr
	}

	creator := newWorklogCreator(auth)

	wl, _, err := creator.CreateWorklog(cmd.Context(), issueKey, req)
	if err != nil {
		return api.MapAPIError(err)
	}

	return renderCreateOutput(cmd.OutOrStdout(), wl, issueKey)
}

// renderCreateOutput handles JSON/quiet/table output for a worklog create result.
func renderCreateOutput(w io.Writer, wl *tracker.Worklog, issueKey string) error {
	if output.IsJSON() {
		item := toWorklogItem(wl)
		if output.HasFieldSelection() {
			filtered := output.FilterFields(item, output.JSONFields)
			if output.JQFilter != "" {
				return output.ApplyJQ(w, filtered, output.JQFilter)
			}
			return output.PrintJSON(w, filtered)
		}
		if output.JQFilter != "" {
			return output.ApplyJQ(w, item, output.JQFilter)
		}
		return output.PrintJSON(w, item)
	}

	if output.IsQuiet() {
		output.PrintQuiet(w, api.DerefFlexString(wl.ID, ""))
		return nil
	}

	// Table output: brief confirmation.
	_, err := fmt.Fprintf(w, "Worklog %s created on %s\n", api.DerefFlexString(wl.ID, ""), issueKey)
	return err
}

// parseDuration parses an ISO 8601 duration string (e.g., PT1H30M) into a
// tracker.Duration. Uses the SDK's UnmarshalJSON for validation.
func parseDuration(s string) (*tracker.Duration, error) {
	var d tracker.Duration

	// Wrap in JSON quotes for UnmarshalJSON: "PT1H30M"
	if err := json.Unmarshal([]byte(`"`+s+`"`), &d); err != nil {
		return nil, errors.NewUserError(
			fmt.Sprintf("invalid ISO 8601 duration %q", s),
			"Use ISO 8601 format: PT1H30M (1h30m), PT45M (45min), P1D (1 day), P1DT2H (1 day 2 hours)",
		)
	}

	return &d, nil
}

// parseTimestamp parses an RFC 3339 timestamp string into a tracker.Timestamp.
func parseTimestamp(s string) (*tracker.Timestamp, error) {
	t, err := time.Parse(time.RFC3339, s)
	if err != nil {
		return nil, errors.NewUserError(
			fmt.Sprintf("invalid timestamp %q", s),
			"Use RFC 3339 format: 2026-03-30T10:00:00Z",
		)
	}

	return &tracker.Timestamp{Time: t}, nil
}

// buildCreateRequest constructs the WorklogRequest from flags or --from-json input.
func buildCreateRequest(cmd *cobra.Command, durationFlag, startFlag, commentFlag,
	fromJSON string) (*tracker.WorklogRequest, error) {
	if cmd.Flags().Changed("from-json") {
		data, parseErr := validate.ParseJSONInput(fromJSON)
		if parseErr != nil {
			return nil, parseErr
		}
		req := &tracker.WorklogRequest{}
		if unmarshalErr := json.Unmarshal(data, req); unmarshalErr != nil {
			return nil, errors.NewUserError(
				fmt.Sprintf("invalid JSON input: %s", unmarshalErr),
				"Provide valid JSON matching the WorklogRequest format",
			)
		}
		return req, nil
	}

	req := &tracker.WorklogRequest{}

	dur, durErr := parseDuration(durationFlag)
	if durErr != nil {
		return nil, durErr
	}
	req.Duration = dur

	if cmd.Flags().Changed("start") {
		ts, tsErr := parseTimestamp(startFlag)
		if tsErr != nil {
			return nil, tsErr
		}
		req.Start = ts
	}

	if cmd.Flags().Changed("comment") {
		req.Comment = &commentFlag
	}

	return req, nil
}

func validateCreateRequest(req *tracker.WorklogRequest) error {
	if req.Duration == nil {
		return errors.NewUserError(
			`missing required field "duration"`,
			`Provide --duration with ISO 8601 format, or include "duration" in --from-json`,
		)
	}

	if req.Start == nil {
		return errors.NewUserError(
			`missing required field "start"`,
			`Provide --start in RFC 3339 format, or include "start" in --from-json`,
		)
	}

	return nil
}
