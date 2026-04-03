package checklist

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

// newEditCmd creates the "checklist edit" command.
func newEditCmd() *cobra.Command {
	var (
		textFlag     string
		checkedFlag  bool
		assigneeFlag string
		fromJSON     string
	)

	cmd := &cobra.Command{
		Use:   "edit ISSUE-KEY ITEM-ID",
		Short: "Edit a checklist item",
		Long: `Edit an existing checklist item on a Yandex Tracker issue.

Deadline is supported only via --from-json (not as a separate flag).

Use --checked to mark an item as done, --checked=false to unmark it.

JSON FIELDS
  id, text, checked, assignee

SEE ALSO
  ytr checklist list    - List checklist items on issue
  ytr checklist create  - Add checklist item to issue
  ytr checklist delete  - Delete a checklist item`,
		Example: `  # Update checklist item text
  ytr checklist edit PROJ-123 item-1 --text "Updated text"

  # Mark item as checked
  ytr checklist edit PROJ-123 item-1 --checked

  # Unmark item
  ytr checklist edit PROJ-123 item-1 --checked=false`,
		Args: cobra.ExactArgs(2),
		PreRunE: func(cmd *cobra.Command, args []string) error {
			if err := validate.ValidateIssueKey(args[0]); err != nil {
				return err
			}
			if _, err := validate.ValidateStringID(args[1], "checklist item ID"); err != nil {
				return err
			}

			// Mutual exclusion: --from-json vs individual flags.
			if cmd.Flags().Changed("from-json") &&
				(cmd.Flags().Changed("text") || cmd.Flags().Changed("checked") ||
					cmd.Flags().Changed("assignee")) {
				return errors.NewUserError(
					"cannot use individual flags and --from-json together",
					"Use --text, --checked, etc. for individual flags, or --from-json for full JSON input",
				)
			}

			// At least one flag or --from-json required.
			if !cmd.Flags().Changed("from-json") &&
				!cmd.Flags().Changed("text") && !cmd.Flags().Changed("checked") &&
				!cmd.Flags().Changed("assignee") {
				return errors.NewUserError(
					"at least one of --text, --checked, --assignee, or --from-json is required",
					"Provide at least one flag to update, or --from-json for JSON input",
				)
			}

			return nil
		},
		RunE: func(cmd *cobra.Command, args []string) error {
			itemID, _ := validate.ValidateStringID(args[1], "checklist item ID")
			return runEdit(cmd, args[0], itemID, textFlag, checkedFlag, assigneeFlag, fromJSON)
		},
	}

	cmd.Flags().StringVar(&textFlag, "text", "", "Checklist item text")
	cmd.Flags().BoolVar(&checkedFlag, "checked", false, "Mark item as checked (--checked=false to unmark)")
	cmd.Flags().StringVar(&assigneeFlag, "assignee", "", "Assignee user ID")
	cmd.Flags().StringVar(
		&fromJSON, "from-json", "",
		`JSON input: inline '{"text":"..."}', @file, or - for stdin`,
	)

	jsonfields.Register("ytr checklist edit", ChecklistFields)

	return cmd
}

// runEdit executes the checklist edit logic.
func runEdit(
	cmd *cobra.Command,
	issueKey, itemID, textFlag string,
	checkedFlag bool,
	assigneeFlag, fromJSON string,
) error {
	if output.IsJSON() && !output.HasFieldSelection() && output.JQFilter == "" {
		return output.PrintFieldHint(cmd.ErrOrStderr(), "checklist edit", ChecklistFields)
	}

	if output.JQFilter != "" && !output.HasFieldSelection() {
		output.JSONFields = ChecklistFields
	}

	// Validate requested fields.
	if output.HasFieldSelection() {
		if err := output.ValidateFields(output.JSONFields, ChecklistFields); err != nil {
			return err
		}
		output.JSONFields = output.NormalizeFields(output.JSONFields, ChecklistFields)
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
	var req *tracker.ChecklistItemRequest

	if cmd.Flags().Changed("from-json") {
		data, parseErr := validate.ParseJSONInput(fromJSON)
		if parseErr != nil {
			return parseErr
		}
		req = &tracker.ChecklistItemRequest{}
		if unmarshalErr := json.Unmarshal(data, req); unmarshalErr != nil {
			return errors.NewUserError(
				fmt.Sprintf("invalid JSON input: %s", unmarshalErr),
				"Provide valid JSON matching the ChecklistItemRequest format",
			)
		}
	} else {
		req = buildEditRequest(cmd, textFlag, checkedFlag, assigneeFlag)
	}

	editor := newChecklistEditor(auth)

	issue, _, err := editor.EditChecklistItem(cmd.Context(), issueKey, itemID, req)
	if err != nil {
		return api.MapAPIError(err)
	}

	// Extract edited item from Issue.ChecklistItems.
	edited := extractEditedItem(issue, itemID)
	if edited == nil {
		return fmt.Errorf("unexpected: edited item %s not found in API response", itemID)
	}

	return renderEditOutput(cmd.OutOrStdout(), toChecklistItem(edited), issueKey)
}

// renderEditOutput handles JSON/quiet/table output for a checklist edit result.
func renderEditOutput(w io.Writer, item checklistItem, issueKey string) error {
	if output.IsJSON() {
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
		output.PrintQuiet(w, item.ID)
		return nil
	}

	// Table output: brief confirmation.
	_, err := fmt.Fprintf(w, "Checklist item %s updated on %s\n", item.ID, issueKey)
	return err
}

// buildEditRequest constructs a ChecklistItemRequest from changed flags.
func buildEditRequest(
	cmd *cobra.Command,
	textFlag string,
	checkedFlag bool,
	assigneeFlag string,
) *tracker.ChecklistItemRequest {
	req := &tracker.ChecklistItemRequest{}
	if cmd.Flags().Changed("text") {
		req.Text = new(textFlag)
	}
	if cmd.Flags().Changed("checked") {
		req.Checked = new(checkedFlag)
	}
	if cmd.Flags().Changed("assignee") {
		req.Assignee = new(assigneeFlag)
	}
	return req
}

// extractEditedItem finds the checklist item matching itemID in the Issue response.
// Returns nil if no matching item is found.
func extractEditedItem(issue *tracker.Issue, itemID string) *tracker.ChecklistItem {
	if issue == nil {
		return nil
	}
	for _, item := range issue.ChecklistItems {
		if api.DerefFlexString(item.ID, "") == itemID {
			return item
		}
	}
	return nil
}
