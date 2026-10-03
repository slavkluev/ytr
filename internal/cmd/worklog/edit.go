package worklog

import (
	"fmt"
	"io"

	"github.com/slavkluev/go-yandex-tracker/tracker"
	"github.com/spf13/cobra"

	"github.com/slavkluev/ytr/internal/api"
	"github.com/slavkluev/ytr/internal/cmd/runner"
	"github.com/slavkluev/ytr/internal/config"
	"github.com/slavkluev/ytr/internal/output"
	"github.com/slavkluev/ytr/internal/validate"
)

var editBody = validate.Body{
	Flags: []validate.BodyFlag{
		{Name: "duration", Key: "duration"}, {Name: "comment", Key: "comment"}, {Name: "start", Key: "start"},
	},
	FromJSON: true,
	Update:   true,
}

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
  id, author, authorId, duration, start, comment

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

			if err := editBody.CheckFlags(cmd.Flags().Changed); err != nil {
				return err
			}

			return checkFlagValues(cmd, durationFlag, startFlag)
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

	runner.SetFields(cmd, WorklogFields)

	return cmd
}

func runEdit(
	cmd *cobra.Command,
	issueKey, worklogID, durationFlag, commentFlag, startFlag, fromJSON string,
) error {
	opts := output.FromContext(cmd.Context())

	if opts.WantsFieldHint(cmd.Flags().Changed("json")) {
		return output.PrintFieldHint(cmd.ErrOrStderr(), "worklog edit", WorklogFields)
	}

	if opts.JQFilter != "" && !opts.HasFieldSelection() {
		opts.JSONFields = WorklogFields
	}

	if opts.HasFieldSelection() {
		if err := output.ValidateFields(opts.JSONFields, WorklogFields); err != nil {
			return err
		}
		opts.JSONFields = output.NormalizeFields(opts.JSONFields, WorklogFields)
	}

	tokenFlag, _ := cmd.Root().PersistentFlags().GetString("token")
	orgIDFlag, _ := cmd.Root().PersistentFlags().GetString("org-id")
	orgTypeFlag, _ := cmd.Root().PersistentFlags().GetString("org-type")

	auth, err := config.ResolveAuth(tokenFlag, orgIDFlag, orgTypeFlag)
	if err != nil {
		return err
	}

	req, buildErr := buildEditRequest(cmd, durationFlag, commentFlag, startFlag, fromJSON)
	if buildErr != nil {
		return buildErr
	}

	editor := newWorklogEditor(auth)

	wl, _, err := editor.EditWorklog(cmd.Context(), issueKey, worklogID, req)
	if err != nil {
		return api.MapAPIError(err)
	}

	return renderEditOutput(cmd.OutOrStdout(), opts, wl, issueKey)
}

func renderEditOutput(w io.Writer, opts *output.Options, wl *tracker.Worklog, issueKey string) error {
	if opts.IsJSON() {
		item := toWorklogItem(wl)
		if opts.HasFieldSelection() {
			filtered := output.FilterFields(item, opts.JSONFields)
			if opts.JQFilter != "" {
				return output.ApplyJQ(w, filtered, opts.JQFilter)
			}
			return opts.PrintJSON(w, filtered)
		}
		if opts.JQFilter != "" {
			return output.ApplyJQ(w, item, opts.JQFilter)
		}
		return opts.PrintJSON(w, item)
	}

	if opts.Quiet {
		output.PrintQuiet(w, api.DerefFlexString(wl.ID, ""))
		return nil
	}

	_, err := fmt.Fprintf(w, "Worklog %s updated on %s\n", api.DerefFlexString(wl.ID, ""), issueKey)
	return err
}

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
		if decodeErr := editBody.Decode(data, req); decodeErr != nil {
			return nil, decodeErr
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
