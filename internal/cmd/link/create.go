package link

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

// newCreateCmd creates the "link create" command.
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

			// Mutual exclusion: --from-json vs --type/--issue.
			if cmd.Flags().Changed("from-json") &&
				(cmd.Flags().Changed("type") || cmd.Flags().Changed("issue")) {
				return errors.NewUserError(
					"cannot use --type/--issue and --from-json together",
					"Use --type and --issue for individual flags, or --from-json for full JSON input",
				)
			}

			// Require both --type and --issue when not using --from-json.
			if !cmd.Flags().Changed("from-json") {
				if !cmd.Flags().Changed("type") || !cmd.Flags().Changed("issue") {
					return errors.NewUserError(
						"both --type and --issue are required",
						"Use --type \"depends on\" --issue PROJ-456, or --from-json for JSON input",
					)
				}
			}

			// Validate target issue key when --issue is provided.
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

// runCreate executes the link create logic.
func runCreate(cmd *cobra.Command, issueKey, typeFlag, issueFlag, fromJSON string) error {
	// Handle --json field hint (no fields specified, D-10).
	if output.IsJSON() && !output.HasFieldSelection() && output.JQFilter == "" {
		return output.PrintFieldHint(cmd.ErrOrStderr(), "link create", LinkListFields)
	}

	// If --jq without --json: auto-populate all fields (Pitfall 5).
	if output.JQFilter != "" && !output.HasFieldSelection() {
		output.JSONFields = LinkListFields
	}

	// Validate requested fields.
	if output.HasFieldSelection() {
		if err := output.ValidateFields(output.JSONFields, LinkListFields); err != nil {
			return err
		}
		output.JSONFields = output.NormalizeFields(output.JSONFields, LinkListFields)
	}

	// Resolve auth from root persistent flags.
	tokenFlag, _ := cmd.Root().PersistentFlags().GetString("token")
	orgIDFlag, _ := cmd.Root().PersistentFlags().GetString("org-id")
	orgTypeFlag, _ := cmd.Root().PersistentFlags().GetString("org-type")

	auth, err := config.ResolveAuth(tokenFlag, orgIDFlag, orgTypeFlag)
	if err != nil {
		return err
	}

	// Build request from --type/--issue or --from-json.
	var req *tracker.LinkRequest

	if cmd.Flags().Changed("from-json") {
		data, parseErr := validate.ParseJSONInput(fromJSON)
		if parseErr != nil {
			return parseErr
		}
		req = &tracker.LinkRequest{}
		if unmarshalErr := json.Unmarshal(data, req); unmarshalErr != nil {
			return errors.NewUserError(
				fmt.Sprintf("invalid JSON input: %s", unmarshalErr),
				"Provide valid JSON matching the LinkRequest format",
			)
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

	return renderCreateOutput(cmd.OutOrStdout(), link, issueKey)
}

// renderCreateOutput handles JSON/quiet/table output for a link create result.
func renderCreateOutput(w io.Writer, link *tracker.IssueLink, issueKey string) error {
	if output.IsJSON() {
		item := toLinkItem(link)
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
		output.PrintQuiet(w, api.DerefFlexString(link.ID, ""))
		return nil
	}

	// Table output: brief confirmation.
	_, err := fmt.Fprintf(w, "Link %s created on %s\n", api.DerefFlexString(link.ID, ""), issueKey)
	return err
}
