package issue

import (
	"io"

	"github.com/slavkluev/go-yandex-tracker/tracker"

	"github.com/spf13/cobra"

	"github.com/slavkluev/ytr/internal/api"
	"github.com/slavkluev/ytr/internal/cmd/runner"
	"github.com/slavkluev/ytr/internal/config"
	"github.com/slavkluev/ytr/internal/output"
	"github.com/slavkluev/ytr/internal/validate"
)

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
  key, summary, status, priority, type, author, authorId, assignee, assigneeId, createdAt, updatedAt, description`,
		Example: `  # Create a simple issue
  ytr issue create --queue PROJ --summary "Fix login bug"

  # Create with all fields
  ytr issue create --queue PROJ --summary "Add feature" --type task --priority normal --assignee john

  # Create from JSON file
  ytr issue create --from-json @issue.json

  # Create and get the new key
  ytr issue create --queue PROJ --summary "Bug" --json key --jq '.key'`,
		Args: cobra.NoArgs,
		PreRunE: func(cmd *cobra.Command, args []string) error {
			return createBody.CheckFlags(cmd.Flags().Changed)
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

	runner.SetFields(cmd, IssueDetailFields)

	return cmd
}

var createBody = validate.Body{
	Flags:    append([]validate.BodyFlag{{Name: "queue", Key: "queue"}}, issueFlags...),
	Required: []string{"queue", "summary"},
	FromJSON: true,
}

var issueFlags = []validate.BodyFlag{
	{Name: "summary", Key: "summary"}, {Name: "description", Key: "description"}, {Name: "type", Key: "type"},
	{Name: "priority", Key: "priority"}, {Name: "assignee", Key: "assignee"}, {Name: "parent", Key: "parent"},
}

func runCreate(cmd *cobra.Command, queue, summary, description, issueType,
	priority, assignee, parent, fromJSON string) error {
	opts := output.FromContext(cmd.Context())

	if opts.WantsFieldHint(cmd.Flags().Changed("json")) {
		return output.PrintFieldHint(cmd.ErrOrStderr(), "issue create", IssueDetailFields)
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

	return outputIssueResult(cmd.OutOrStdout(), opts, issue)
}

func buildCreateRequest(cmd *cobra.Command, queue, summary, description, issueType,
	priority, assignee, parent, fromJSON string) (*tracker.IssueRequest, error) {
	if cmd.Flags().Changed("from-json") {
		return parseIssueRequestFromJSON(fromJSON, createBody)
	}

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

func parseIssueRequestFromJSON(fromJSON string, body validate.Body) (*tracker.IssueRequest, error) {
	data, parseErr := validate.ParseJSONInput(fromJSON)
	if parseErr != nil {
		return nil, parseErr
	}
	req := &tracker.IssueRequest{}
	if decodeErr := body.Decode(data, req); decodeErr != nil {
		return nil, decodeErr
	}
	return req, nil
}

func outputIssueResult(w io.Writer, opts *output.Options, issue *tracker.Issue) error {
	if opts.IsJSON() {
		detail := toIssueDetail(issue)

		if opts.HasFieldSelection() {
			filtered := output.FilterFields(detail, opts.JSONFields)
			if opts.JQFilter != "" {
				return output.ApplyJQ(w, filtered, opts.JQFilter)
			}
			return opts.PrintJSON(w, filtered)
		}
		if opts.JQFilter != "" {
			return output.ApplyJQ(w, detail, opts.JQFilter)
		}
		return opts.PrintJSON(w, detail)
	}

	if opts.Quiet {
		output.PrintQuiet(w, api.DerefString(issue.Key, ""))
		return nil
	}

	d := opts.NewDetail(w)

	d.Field("Key", api.DerefString(issue.Key, "-"))
	d.Field("Summary", api.DerefString(issue.Summary, "-"))
	d.Field("Status", issueStatusDisplay(issue))

	return d.Err()
}
