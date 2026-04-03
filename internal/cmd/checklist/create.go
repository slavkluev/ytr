package checklist

import (
	"encoding/json"
	stdErrors "errors"
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

// newCreateCmd creates the "checklist create" command.
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
  id, text, checked, assignee

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

			// Mutual exclusion: --from-json vs individual flags.
			if cmd.Flags().Changed("from-json") &&
				(cmd.Flags().Changed("text") || cmd.Flags().Changed("assignee")) {
				return errors.NewUserError(
					"cannot use individual flags and --from-json together",
					"Use --text and --assignee for individual flags, or --from-json for full JSON input",
				)
			}

			// At least --text or --from-json required.
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

// runCreate executes the checklist create logic.
func runCreate(cmd *cobra.Command, issueKey, textFlag, assigneeFlag, fromJSON string) error {
	if output.IsJSON() && !output.HasFieldSelection() && output.JQFilter == "" {
		return output.PrintFieldHint(cmd.ErrOrStderr(), "checklist create", ChecklistFields)
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

	// Extract created item from Issue.ChecklistItems.
	created := extractCreatedItem(issue)
	if created == nil {
		return stdErrors.New("unexpected: created item not found in API response")
	}

	return renderCreateOutput(cmd.OutOrStdout(), toChecklistItem(created), issueKey)
}

// renderCreateOutput handles JSON/quiet/table output for a checklist create result.
func renderCreateOutput(w io.Writer, item checklistItem, issueKey string) error {
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
	_, err := fmt.Fprintf(w, "Checklist item %s created on %s\n", item.ID, issueKey)
	return err
}

// extractCreatedItem returns the last element from Issue.ChecklistItems.
// The API appends new items at the end, so the last element is the created item.
// Returns nil if ChecklistItems is nil or empty.
func extractCreatedItem(issue *tracker.Issue) *tracker.ChecklistItem {
	if issue == nil || len(issue.ChecklistItems) == 0 {
		return nil
	}
	return issue.ChecklistItems[len(issue.ChecklistItems)-1]
}
