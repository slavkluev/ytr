package link

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
		typeFlag  string
		issueFlag string
		fromJSON  string
	)

	cmd := &cobra.Command{
		Use:   "create ISSUE-KEY",
		Short: "Create a link to another issue",
		Long: `Create a typed link between two Yandex Tracker issues.

Provide --type and --issue for individual flags, or --from-json for full JSON input.

JSON FIELDS
  id, type, issue, summary

SEE ALSO
  ytr link list    - List links on issue
  ytr link delete  - Delete a link`,
		Example: `  # Create a dependency link
  ytr link create PROJ-123 --type "depends on" --issue PROJ-456

  # Create link via JSON
  ytr link create PROJ-123 --from-json '{"relationship":"relates","issue":"PROJ-456"}'

  # Create link and get result as JSON
  ytr link create PROJ-123 --type "relates" --issue PROJ-456 --json id,type`,
		Args: cobra.ExactArgs(1),
		PreRunE: func(cmd *cobra.Command, args []string) error {
			if err := validate.ValidateIssueKey(args[0]); err != nil {
				return err
			}

			if cmd.Flags().Changed("from-json") &&
				(cmd.Flags().Changed("type") || cmd.Flags().Changed("issue")) {
				return errors.NewUserError(
					"cannot use --type/--issue and --from-json together",
					"Use --type and --issue for individual flags, or --from-json for full JSON input",
				)
			}

			if !cmd.Flags().Changed("from-json") {
				if !cmd.Flags().Changed("type") || !cmd.Flags().Changed("issue") {
					return errors.NewUserError(
						"both --type and --issue are required",
						"Use --type \"depends on\" --issue PROJ-456, or --from-json for JSON input",
					)
				}
			}

			if cmd.Flags().Changed("issue") {
				if err := validate.ValidateIssueKey(issueFlag); err != nil {
					return err
				}
			}

			return nil
		},
		RunE: func(cmd *cobra.Command, args []string) error {
			return runCreate(cmd, args[0], typeFlag, issueFlag, fromJSON)
		},
	}

	cmd.Flags().StringVar(&typeFlag, "type", "", "Link type (e.g., \"depends on\", \"relates\")")
	cmd.Flags().StringVar(&issueFlag, "issue", "", "Target issue key (e.g., PROJ-456)")
	cmd.Flags().StringVar(
		&fromJSON, "from-json", "",
		`JSON input: inline '{"relationship":"...","issue":"..."}', @file, or - for stdin`,
	)

	jsonfields.Register("ytr link create", LinkListFields)

	return cmd
}

func runCreate(cmd *cobra.Command, issueKey, typeFlag, issueFlag, fromJSON string) error {
	opts := output.FromContext(cmd.Context())

	if opts.WantsFieldHint(cmd.Flags().Changed("json")) {
		return output.PrintFieldHint(cmd.ErrOrStderr(), "link create", LinkListFields)
	}

	if opts.JQFilter != "" && !opts.HasFieldSelection() {
		opts.JSONFields = LinkListFields
	}

	if opts.HasFieldSelection() {
		if err := output.ValidateFields(opts.JSONFields, LinkListFields); err != nil {
			return err
		}
		opts.JSONFields = output.NormalizeFields(opts.JSONFields, LinkListFields)
	}

	tokenFlag, _ := cmd.Root().PersistentFlags().GetString("token")
	orgIDFlag, _ := cmd.Root().PersistentFlags().GetString("org-id")
	orgTypeFlag, _ := cmd.Root().PersistentFlags().GetString("org-type")

	auth, err := config.ResolveAuth(tokenFlag, orgIDFlag, orgTypeFlag)
	if err != nil {
		return err
	}

	var req *tracker.LinkRequest

	if cmd.Flags().Changed("from-json") {
		data, parseErr := validate.ParseJSONInput(fromJSON)
		if parseErr != nil {
			return parseErr
		}
		req = &tracker.LinkRequest{}
		if unmarshalErr := validate.UnmarshalRequestJSON(data, req); unmarshalErr != nil {
			return unmarshalErr
		}
	} else {
		req = &tracker.LinkRequest{
			Relationship: &typeFlag,
			Issue:        &issueFlag,
		}
	}

	creator := newLinkCreator(auth)

	link, _, err := creator.CreateLink(cmd.Context(), issueKey, req)
	if err != nil {
		return api.MapAPIError(err)
	}

	return renderCreateOutput(cmd.OutOrStdout(), opts, link, issueKey)
}

func renderCreateOutput(w io.Writer, opts *output.Options, link *tracker.IssueLink, issueKey string) error {
	if opts.IsJSON() {
		item := toLinkItem(link)
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
		output.PrintQuiet(w, api.DerefFlexString(link.ID, ""))
		return nil
	}

	_, err := fmt.Fprintf(w, "Link %s created on %s\n", api.DerefFlexString(link.ID, ""), issueKey)
	return err
}
