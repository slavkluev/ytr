package worklog

import (
	"encoding/json"
	"fmt"
	"io"

	"github.com/slavkluev/go-yandex-tracker/tracker"
	"github.com/spf13/cobra"

	"github.com/slavkluev/ytr/internal/api"
	"github.com/slavkluev/ytr/internal/cmd/jsonfields"
	"github.com/slavkluev/ytr/internal/config"
	"github.com/slavkluev/ytr/internal/errors"
	"github.com/slavkluev/ytr/internal/output"
	"github.com/slavkluev/ytr/internal/validate"
)

// newEditCmd creates the "worklog edit" command.
func newEditCmd() *cobra.Command {
	var (
		durationFlag string
		commentFlag  string
		startFlag    string
		fromJSON     string
	)

	cmd := &cobra.Command{
		Use:   "edit ISSUE-KEY WORKLOG-ID",
		Short: "Edit a worklog",
		Long: `Edit an existing worklog on a Yandex Tracker issue.

Provide one or more flags to update, or --from-json for full JSON input.

JSON FIELDS
  id, author, duration, start, comment

SEE ALSO
  ytr worklog list    - List worklogs on issue
  ytr worklog create  - Create a worklog
  ytr worklog delete  - Delete a worklog`,
		Example: `  # Update duration
  ytr worklog edit PROJ-123 abc123 --duration PT2H

  # Update comment
  ytr worklog edit PROJ-123 abc123 --comment "Updated notes"

  # Update via JSON
  ytr worklog edit PROJ-123 abc123 --from-json '{"duration":"PT3H"}'`,
		Args: cobra.ExactArgs(2),
		PreRunE: func(cmd *cobra.Command, args []string) error {
			if err := validate.ValidateIssueKey(args[0]); err != nil {
				return err
			}
			if _, err := validate.ValidateStringID(args[1], "worklog ID"); err != nil {
				return err
			}

			// Mutual exclusion: --from-json vs individual flags.
			if cmd.Flags().Changed("from-json") &&
				(cmd.Flags().Changed("duration") || cmd.Flags().Changed("comment") ||
					cmd.Flags().Changed("start")) {
				return errors.NewUserError(
					"cannot use individual flags and --from-json together",
					"Use --duration, --comment, --start for individual flags, or --from-json for full JSON input",
				)
			}

			// At least one flag or --from-json required.
			if !cmd.Flags().Changed("from-json") &&
				!cmd.Flags().Changed("duration") && !cmd.Flags().Changed("comment") &&
				!cmd.Flags().Changed("start") {
				return errors.NewUserError(
					"at least one of --duration, --comment, --start, or --from-json is required",
					"Provide at least one flag to update, or --from-json for JSON input",
				)
			}

			return nil
		},
		RunE: func(cmd *cobra.Command, args []string) error {
			worklogID, _ := validate.ValidateStringID(args[1], "worklog ID")
			return runEdit(cmd, args[0], worklogID, durationFlag, commentFlag, startFlag, fromJSON)
		},
	}

	cmd.Flags().StringVar(&durationFlag, "duration", "", "Duration in ISO 8601 format (e.g., PT1H30M)")
	cmd.Flags().StringVar(&commentFlag, "comment", "", "Worklog comment")
	cmd.Flags().StringVar(&startFlag, "start", "", "Start time in RFC 3339 format")
	cmd.Flags().StringVar(&fromJSON, "from-json", "", `JSON input: inline '{"duration":"PT1H"}', @file, or - for stdin`)

	jsonfields.Register("ytr worklog edit", WorklogFields)

	return cmd
}

// runEdit executes the worklog edit logic.
func runEdit(
	cmd *cobra.Command,
	issueKey, worklogID, durationFlag, commentFlag, startFlag, fromJSON string,
) error {
	// Handle --json field hint (no fields specified, D-10).
	if output.IsJSON() && !output.HasFieldSelection() && output.JQFilter == "" {
		return output.PrintFieldHint(cmd.ErrOrStderr(), "worklog edit", WorklogFields)
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

	// Build request from individual flags or --from-json.
	req, buildErr := buildEditRequest(cmd, durationFlag, commentFlag, startFlag, fromJSON)
	if buildErr != nil {
		return buildErr
	}

	editor := newWorklogEditor(auth)

	wl, _, err := editor.EditWorklog(cmd.Context(), issueKey, worklogID, req)
	if err != nil {
		return api.MapAPIError(err)
	}

	return renderEditOutput(cmd.OutOrStdout(), wl, issueKey)
}

// renderEditOutput handles JSON/quiet/table output for a worklog edit result.
func renderEditOutput(w io.Writer, wl *tracker.Worklog, issueKey string) error {
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
	_, err := fmt.Fprintf(w, "Worklog %s updated on %s\n", api.DerefFlexString(wl.ID, ""), issueKey)
	return err
}

// buildEditRequest constructs a WorklogRequest from flags or --from-json input.
func buildEditRequest(
	cmd *cobra.Command,
	durationFlag, commentFlag, startFlag, fromJSON string,
) (*tracker.WorklogRequest, error) {
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

	if cmd.Flags().Changed("duration") {
		dur, durErr := parseDuration(durationFlag)
		if durErr != nil {
			return nil, durErr
		}
		req.Duration = dur
	}

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
