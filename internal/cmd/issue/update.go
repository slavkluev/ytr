package issue

import (
	"github.com/slavkluev/go-yandex-tracker/tracker"

	"github.com/spf13/cobra"

	"github.com/slavkluev/ytr/internal/api"
	"github.com/slavkluev/ytr/internal/cmd/runner"
	"github.com/slavkluev/ytr/internal/config"
	"github.com/slavkluev/ytr/internal/output"
	"github.com/slavkluev/ytr/internal/validate"
)

var updateBody = validate.Body{Flags: issueFlags, FromJSON: true, Update: true}

func newUpdateCmd() *cobra.Command {
	var (
		summary     string
		description string
		issueType   string
		priority    string
		assignee    string
		parent      string
		fromJSON    string
	)

	cmd := &cobra.Command{
		Use:   "update ISSUE-KEY",
		Short: "Update an issue",
		Long: `Update an existing Yandex Tracker issue. Only changed fields are sent to the API.

JSON FIELDS
  key, summary, status, priority, type, author, authorId, assignee, assigneeId, createdAt, updatedAt, description

SEE ALSO
  ytr issue view        - View issue details
  ytr issue transition  - Transition issue status`,
		Example: `  # Update issue summary
  ytr issue update PROJ-123 --summary "Updated title"

  # Change priority and assignee
  ytr issue update PROJ-123 --priority critical --assignee jane

  # Update from JSON
  ytr issue update PROJ-123 --from-json '{"summary": "New title"}'`,
		Args:    cobra.ExactArgs(1),
		PreRunE: validateUpdateFlags,
		RunE: func(cmd *cobra.Command, args []string) error {
			return runUpdate(cmd, args[0], summary, description, issueType,
				priority, assignee, parent, fromJSON)
		},
	}

	cmd.Flags().StringVar(&summary, "summary", "", "New issue summary")
	cmd.Flags().StringVar(&description, "description", "", "New issue description")
	cmd.Flags().StringVar(&issueType, "type", "", "New issue type key")
	cmd.Flags().StringVar(&priority, "priority", "", "New priority key")
	cmd.Flags().StringVar(&assignee, "assignee", "", "New assignee user ID")
	cmd.Flags().StringVar(&parent, "parent", "", "New parent issue key")
	cmd.Flags().StringVar(&fromJSON, "from-json", "", "JSON input: inline string, @file, or - for stdin")

	runner.SetFields(cmd, IssueDetailFields)

	return cmd
}

func validateUpdateFlags(cmd *cobra.Command, args []string) error {
	if err := validate.ValidateIssueKey(args[0]); err != nil {
		return err
	}

	return updateBody.CheckFlags(cmd.Flags().Changed)
}

func runUpdate(cmd *cobra.Command, issueKey, summary, description, issueType,
	priority, assignee, parent, fromJSON string) error {
	opts := output.FromContext(cmd.Context())

	if opts.WantsFieldHint(cmd.Flags().Changed("json")) {
		return output.PrintFieldHint(cmd.ErrOrStderr(), "issue update", IssueDetailFields)
	}

	if opts.JQFilter != "" && !opts.HasFieldSelection() {
		opts.JSONFields = IssueDetailFields
	}

	if opts.HasFieldSelection() {
		if err := output.ValidateFields(opts.JSONFields, IssueDetailFields); err != nil {
			return err
		}
		opts.JSONFields = output.NormalizeFields(opts.JSONFields, IssueDetailFields)
	}

	tokenFlag, _ := cmd.Root().PersistentFlags().GetString("token")
	orgIDFlag, _ := cmd.Root().PersistentFlags().GetString("org-id")
	orgTypeFlag, _ := cmd.Root().PersistentFlags().GetString("org-type")

	auth, err := config.ResolveAuth(tokenFlag, orgIDFlag, orgTypeFlag)
	if err != nil {
		return err
	}

	req, err := buildUpdateRequest(cmd, summary, description, issueType,
		priority, assignee, parent, fromJSON)
	if err != nil {
		return err
	}

	editor := newEditor(auth)
	issue, _, err := editor.Edit(cmd.Context(), issueKey, req, nil)
	if err != nil {
		return api.MapAPIError(err)
	}

	return outputIssueResult(cmd.OutOrStdout(), opts, issue)
}

func buildUpdateRequest(cmd *cobra.Command, summary, description, issueType,
	priority, assignee, parent, fromJSON string) (*tracker.IssueRequest, error) {
	if cmd.Flags().Changed("from-json") {
		return parseIssueRequestFromJSON(fromJSON, updateBody)
	}

	req := &tracker.IssueRequest{}

	if cmd.Flags().Changed("summary") {
		if valErr := validate.ValidateNoControlChars("summary", summary); valErr != nil {
			return nil, valErr
		}
		req.Summary = new(summary)
	}
	if cmd.Flags().Changed("description") {
		if valErr := validate.ValidateNoControlChars("description", description); valErr != nil {
			return nil, valErr
		}
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
