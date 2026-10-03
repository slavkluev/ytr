package checklist

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
		{Name: "text", Key: "text"}, {Name: "checked", Key: "checked"}, {Name: "assignee", Key: "assignee"},
	},
	FromJSON: true,
	Update:   true,
}

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
  id, text, checked, assignee, assigneeId

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

			return editBody.CheckFlags(cmd.Flags().Changed)
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

	runner.SetFields(cmd, ChecklistFields)

	return cmd
}

func runEdit(
	cmd *cobra.Command,
	issueKey, itemID, textFlag string,
	checkedFlag bool,
	assigneeFlag, fromJSON string,
) error {
	opts := output.FromContext(cmd.Context())

	if opts.WantsFieldHint(cmd.Flags().Changed("json")) {
		return output.PrintFieldHint(cmd.ErrOrStderr(), "checklist edit", ChecklistFields)
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
		if decodeErr := editBody.Decode(data, req); decodeErr != nil {
			return decodeErr
		}
	} else {
		req = buildEditRequest(cmd, textFlag, checkedFlag, assigneeFlag)
	}

	editor := newChecklistEditor(auth)

	issue, _, err := editor.EditChecklistItem(cmd.Context(), issueKey, itemID, req)
	if err != nil {
		return api.MapAPIError(err)
	}

	// The edit already succeeded. If the response doesn't echo the item back by
	// ID, report a best-effort confirmation from the request rather than failing
	// with a misleading error (the item ID is known, so output still carries it).
	if edited := extractEditedItem(issue, itemID); edited != nil {
		return renderEditOutput(cmd.OutOrStdout(), opts, toChecklistItem(edited), issueKey)
	}
	return renderEditOutput(cmd.OutOrStdout(), opts, editedChecklistItem(itemID, req), issueKey)
}

func renderEditOutput(w io.Writer, opts *output.Options, item checklistItem, issueKey string) error {
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

	_, err := fmt.Fprintf(w, "Checklist item %s updated on %s\n", item.ID, issueKey)
	return err
}

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

func extractEditedItem(issue *tracker.Issue, itemID string) *tracker.ChecklistItem {
	if issue == nil {
		return nil
	}
	for _, item := range issue.ChecklistItems {
		if item == nil {
			continue
		}
		if api.DerefFlexString(item.ID, "") == itemID {
			return item
		}
	}
	return nil
}

func editedChecklistItem(itemID string, req *tracker.ChecklistItemRequest) checklistItem {
	item := checklistItem{ID: itemID}
	if req != nil {
		item.Text = api.DerefString(req.Text, "")
		item.Checked = api.DerefBool(req.Checked, false)
		item.Assignee = api.DerefString(req.Assignee, "")
	}
	return item
}
