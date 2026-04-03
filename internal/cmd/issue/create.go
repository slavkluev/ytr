package issue

import (
	"encoding/json"
	"fmt"
	"slices"

	"github.com/jedib0t/go-pretty/v6/text"
	"github.com/slavkluev/go-yandex-tracker/tracker"

	"github.com/spf13/cobra"

	"github.com/slavkluev/ytr/internal/api"
	"github.com/slavkluev/ytr/internal/cmd/jsonfields"
	"github.com/slavkluev/ytr/internal/config"
	"github.com/slavkluev/ytr/internal/errors"
	"github.com/slavkluev/ytr/internal/output"
	"github.com/slavkluev/ytr/internal/validate"
)

// newCreateCmd creates the "issue create" command for creating new issues.
func newCreateCmd() *cobra.Command {
	var (
		queue       string
		summary     string
		description string
		issueType   string
		priority    string
		assignee    string
		parent      string
		fromJSON    string
	)

	cmd := &cobra.Command{
		Use:   "create",
		Short: "Create an issue",
		Long: `Create a new Yandex Tracker issue with flags or raw JSON input.

JSON FIELDS
  key, summary, status, priority, type, author, assignee, createdAt, updatedAt, description

SEE ALSO
  ytr issue list    - List issues
  ytr issue view    - View issue details
  ytr issue update  - Update an issue`,
		Example: `  # Create a simple issue
  ytr issue create --queue PROJ --summary "Fix login bug"

  # Create with all fields
  ytr issue create --queue PROJ --summary "Add feature" --type task --priority normal --assignee john

  # Create from JSON file
  ytr issue create --from-json @issue.json

  # Create and get the new key
  ytr issue create --queue PROJ --summary "Bug" --json key --jq '.key'`,
		PreRunE: func(cmd *cobra.Command, args []string) error {
			return validateCreateFlags(cmd)
		},
		RunE: func(cmd *cobra.Command, args []string) error {
			return runCreate(cmd, queue, summary, description, issueType,
				priority, assignee, parent, fromJSON)
		},
	}

	cmd.Flags().StringVar(&queue, "queue", "", "Queue key (required unless --from-json)")
	cmd.Flags().StringVar(&summary, "summary", "", "Issue summary (required unless --from-json)")
	cmd.Flags().StringVar(&description, "description", "", "Issue description")
	cmd.Flags().StringVar(&issueType, "type", "", "Issue type key")
	cmd.Flags().StringVar(&priority, "priority", "", "Priority key")
	cmd.Flags().StringVar(&assignee, "assignee", "", "Assignee user ID")
	cmd.Flags().StringVar(&parent, "parent", "", "Parent issue key")
	cmd.Flags().StringVar(&fromJSON, "from-json", "", "JSON input: inline string, @file, or - for stdin")

	jsonfields.Register("ytr issue create", IssueDetailFields)

	return cmd
}

// validateCreateFlags checks flag constraints for create:
// - If --from-json is set, no individual field flags may be set.
// - If --from-json is not set, --queue and --summary are required.
func validateCreateFlags(cmd *cobra.Command) error {
	if cmd.Flags().Changed("from-json") {
		fieldFlags := []string{"summary", "description", "type", "priority", "assignee", "parent"}
		if slices.ContainsFunc(fieldFlags, func(flag string) bool {
			return cmd.Flags().Changed(flag)
		}) {
			return errors.NewUserError(
				"Use --from-json OR individual flags, not both",
				"Remove --from-json to use individual flags, or remove individual flags to use --from-json",
			)
		}
		return nil
	}

	// Without --from-json, queue and summary are required.
	if !cmd.Flags().Changed("queue") {
		return errors.NewUserError(
			"required flag \"queue\" not set",
			"Provide --queue with the target queue key",
		)
	}
	if !cmd.Flags().Changed("summary") {
		return errors.NewUserError(
			"required flag \"summary\" not set",
			"Provide --summary with the issue title",
		)
	}

	return nil
}

// runCreate executes the issue create logic.
func runCreate(cmd *cobra.Command, queue, summary, description, issueType,
	priority, assignee, parent, fromJSON string) error {
	if output.IsJSON() && !output.HasFieldSelection() && output.JQFilter == "" {
		return output.PrintFieldHint(cmd.ErrOrStderr(), "issue create", IssueDetailFields)
	}

	if output.JQFilter != "" && !output.HasFieldSelection() {
		output.JSONFields = IssueDetailFields
	}

	// Validate requested fields.
	if output.HasFieldSelection() {
		if err := output.ValidateFields(output.JSONFields, IssueDetailFields); err != nil {
			return err
		}
		output.JSONFields = output.NormalizeFields(output.JSONFields, IssueDetailFields)
	}

	tokenFlag, _ := cmd.Root().PersistentFlags().GetString("token")
	orgIDFlag, _ := cmd.Root().PersistentFlags().GetString("org-id")
	orgTypeFlag, _ := cmd.Root().PersistentFlags().GetString("org-type")

	auth, err := config.ResolveAuth(tokenFlag, orgIDFlag, orgTypeFlag)
	if err != nil {
		return err
	}

	req, err := buildCreateRequest(cmd, queue, summary, description, issueType,
		priority, assignee, parent, fromJSON)
	if err != nil {
		return err
	}

	creator := newCreator(auth)
	issue, _, err := creator.Create(cmd.Context(), req)
	if err != nil {
		return api.MapAPIError(err)
	}

	return outputIssueResult(cmd, issue)
}

// buildCreateRequest constructs the IssueRequest from flags or --from-json input.
func buildCreateRequest(cmd *cobra.Command, queue, summary, description, issueType,
	priority, assignee, parent, fromJSON string) (*tracker.IssueRequest, error) {
	if cmd.Flags().Changed("from-json") {
		return parseIssueRequestFromJSON(fromJSON)
	}

	// Validate string inputs for control characters.
	if valErr := validate.ValidateNoControlChars("summary", summary); valErr != nil {
		return nil, valErr
	}
	if cmd.Flags().Changed("description") {
		if valErr := validate.ValidateNoControlChars("description", description); valErr != nil {
			return nil, valErr
		}
	}

	req := &tracker.IssueRequest{}
	req.Queue = new(queue)
	req.Summary = new(summary)

	if cmd.Flags().Changed("description") {
		req.Description = new(description)
	}
	if cmd.Flags().Changed("type") {
		req.Type = new(issueType)
	}
	if cmd.Flags().Changed("priority") {
		req.Priority = new(priority)
	}
	if cmd.Flags().Changed("assignee") {
		req.Assignee = new(assignee)
	}
	if cmd.Flags().Changed("parent") {
		req.Parent = new(parent)
	}

	return req, nil
}

// parseIssueRequestFromJSON parses a --from-json input into an IssueRequest.
func parseIssueRequestFromJSON(fromJSON string) (*tracker.IssueRequest, error) {
	data, parseErr := validate.ParseJSONInput(fromJSON)
	if parseErr != nil {
		return nil, parseErr
	}
	req := &tracker.IssueRequest{}
	if unmarshalErr := json.Unmarshal(data, req); unmarshalErr != nil {
		return nil, errors.NewUserError(
			fmt.Sprintf("invalid JSON input: %s", unmarshalErr),
			"Provide valid JSON matching the IssueRequest format",
		)
	}
	return req, nil
}

// outputIssueResult formats the issue in the current output mode.
// Shared between create and update commands.
func outputIssueResult(cmd *cobra.Command, issue *tracker.Issue) error {
	w := cmd.OutOrStdout()

	if output.IsJSON() {
		detail := toIssueDetail(issue)

		if output.HasFieldSelection() {
			filtered := output.FilterFields(detail, output.JSONFields)
			if output.JQFilter != "" {
				return output.ApplyJQ(w, filtered, output.JQFilter)
			}
			return output.PrintJSON(w, filtered)
		}
		if output.JQFilter != "" {
			return output.ApplyJQ(w, detail, output.JQFilter)
		}
		return output.PrintJSON(w, detail)
	}

	if output.IsQuiet() {
		output.PrintQuiet(w, api.DerefString(issue.Key, ""))
		return nil
	}

	// Table-style key-value output.
	bold := func(label string) string {
		if output.ColorsEnabled() {
			return text.Colors{text.Bold}.Sprint(label)
		}
		return label
	}

	var writeErr error
	printField := func(label, value string) {
		if writeErr != nil {
			return
		}
		_, writeErr = fmt.Fprintf(w, "%s  %s\n", bold(label+":"), value)
	}

	printField("Key", api.DerefString(issue.Key, "-"))
	printField("Summary", api.DerefString(issue.Summary, "-"))
	printField("Status", issueStatusDisplay(issue))

	return writeErr
}
