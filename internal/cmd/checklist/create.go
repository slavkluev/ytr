package checklist

import (
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

func newCreateCmd() *cobra.Command {
	var (
		textFlag     string
		assigneeFlag string
		fromJSON     string
	)

	cmd := &cobra.Command{
		Use:   "create ISSUE-KEY",
		Short: "Add checklist item to issue",
		Long: `Create a new checklist item on a Yandex Tracker issue.

Deadline is supported only via --from-json (not as a separate flag).

JSON FIELDS
  id, text, checked, assignee, assigneeId

SEE ALSO
  ytr checklist list    - List checklist items on issue
  ytr checklist edit    - Edit a checklist item
  ytr checklist delete  - Delete a checklist item`,
		Example: `  # Create a checklist item
  ytr checklist create PROJ-123 --text "Review PR"

  # Create with assignee
  ytr checklist create PROJ-123 --text "Deploy" --assignee 12345

  # Create via JSON (supports deadline)
  ytr checklist create PROJ-123 --from-json '{"text":"Review","deadline":{"date":"2026-04-01T00:00:00Z"}}'`,
		Args: cobra.ExactArgs(1),
		PreRunE: func(cmd *cobra.Command, args []string) error {
			if err := validate.ValidateIssueKey(args[0]); err != nil {
				return err
			}

			if cmd.Flags().Changed("from-json") &&
				(cmd.Flags().Changed("text") || cmd.Flags().Changed("assignee")) {
				return errors.NewUserError(
					"cannot use individual flags and --from-json together",
					"Use --text and --assignee for individual flags, or --from-json for full JSON input",
				)
			}

			if !cmd.Flags().Changed("text") && !cmd.Flags().Changed("from-json") {
				return errors.NewUserError(
					"--text or --from-json is required",
					"Provide --text \"item text\" or --from-json '{\"text\": \"...\"}'",
				)
			}

			return nil
		},
		RunE: func(cmd *cobra.Command, args []string) error {
			return runCreate(cmd, args[0], textFlag, assigneeFlag, fromJSON)
		},
	}

	cmd.Flags().StringVar(&textFlag, "text", "", "Checklist item text (required)")
	cmd.Flags().StringVar(&assigneeFlag, "assignee", "", "Assignee user ID")
	cmd.Flags().StringVar(
		&fromJSON, "from-json", "",
		`JSON input: inline '{"text":"..."}', @file, or - for stdin`,
	)

	jsonfields.Register("ytr checklist create", ChecklistFields)

	return cmd
}

func runCreate(cmd *cobra.Command, issueKey, textFlag, assigneeFlag, fromJSON string) error {
	opts := output.FromContext(cmd.Context())

	if opts.WantsFieldHint(cmd.Flags().Changed("json")) {
		return output.PrintFieldHint(cmd.ErrOrStderr(), "checklist create", ChecklistFields)
	}

	if opts.JQFilter != "" && !opts.HasFieldSelection() {
		opts.JSONFields = ChecklistFields
	}

	if opts.HasFieldSelection() {
		if err := output.ValidateFields(opts.JSONFields, ChecklistFields); err != nil {
			return err
		}
		opts.JSONFields = output.NormalizeFields(opts.JSONFields, ChecklistFields)
	}

	tokenFlag, _ := cmd.Root().PersistentFlags().GetString("token")
	orgIDFlag, _ := cmd.Root().PersistentFlags().GetString("org-id")
	orgTypeFlag, _ := cmd.Root().PersistentFlags().GetString("org-type")

	auth, err := config.ResolveAuth(tokenFlag, orgIDFlag, orgTypeFlag)
	if err != nil {
		return err
	}

	var req *tracker.ChecklistItemRequest

	if cmd.Flags().Changed("from-json") {
		data, parseErr := validate.ParseJSONInput(fromJSON)
		if parseErr != nil {
			return parseErr
		}
		req = &tracker.ChecklistItemRequest{}
		if unmarshalErr := validate.UnmarshalRequestJSON(data, req); unmarshalErr != nil {
			return unmarshalErr
		}
	} else {
		req = &tracker.ChecklistItemRequest{
			Text: new(textFlag),
		}
		if cmd.Flags().Changed("assignee") {
			req.Assignee = new(assigneeFlag)
		}
	}

	creator := newChecklistCreator(auth)

	issue, _, err := creator.CreateChecklistItem(cmd.Context(), issueKey, req)
	if err != nil {
		return api.MapAPIError(err)
	}

	// The mutation already succeeded. Identify the created item by matching the
	// requested text; if the response doesn't let us identify it, report a
	// best-effort confirmation from the request rather than failing. A non-zero
	// exit here would make agents retry and create duplicates (create isn't
	// idempotent).
	if created := extractCreatedItem(issue, req); created != nil {
		return renderCreateOutput(cmd.OutOrStdout(), opts, toChecklistItem(created), issueKey)
	}
	return renderCreateOutput(cmd.OutOrStdout(), opts, requestedChecklistItem(req), issueKey)
}

func renderCreateOutput(w io.Writer, opts *output.Options, item checklistItem, issueKey string) error {
	if opts.IsJSON() {
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
		output.PrintQuiet(w, item.ID)
		return nil
	}

	_, err := fmt.Fprintf(w, "Checklist item %s created on %s\n", item.ID, issueKey)
	return err
}

// The API echoes the full checklist and item order is not guaranteed, so it
// matches by the requested text (newest match wins on duplicates, since the API
// appends). When the text is unknown (e.g. --from-json without a text field) it
// falls back to the last non-nil item. Returns nil only when no item is present.
func extractCreatedItem(issue *tracker.Issue, req *tracker.ChecklistItemRequest) *tracker.ChecklistItem {
	if issue == nil {
		return nil
	}
	if req != nil && req.Text != nil && *req.Text != "" {
		for i := len(issue.ChecklistItems) - 1; i >= 0; i-- {
			if item := issue.ChecklistItems[i]; item != nil &&
				api.DerefString(item.Text, "") == *req.Text {
				return item
			}
		}
	}
	// API appends new items at the end; fall back to the last non-nil item.
	for i := len(issue.ChecklistItems) - 1; i >= 0; i-- {
		if item := issue.ChecklistItems[i]; item != nil {
			return item
		}
	}
	return nil
}

func requestedChecklistItem(req *tracker.ChecklistItemRequest) checklistItem {
	if req == nil {
		return checklistItem{}
	}
	return checklistItem{
		Text:     api.DerefString(req.Text, ""),
		Checked:  api.DerefBool(req.Checked, false),
		Assignee: api.DerefString(req.Assignee, ""),
	}
}
