package component

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

// newCreateCmd creates the "component create" command.
func newCreateCmd() *cobra.Command {
	var (
		nameFlag        string
		queueFlag       string
		descriptionFlag string
		leadFlag        string
		assignAutoFlag  bool
		fromJSON        string
	)

	cmd := &cobra.Command{
		Use:   "create",
		Short: "Create a component",
		Long: `Create a new project component in Yandex Tracker.

Provide --name and --queue for required fields, or --from-json for full JSON input.

JSON FIELDS
  id, name, queue, lead, description, assignAuto

SEE ALSO
  ytr component list    - List all components
  ytr component get     - Show component details
  ytr component edit    - Edit a component
  ytr component delete  - Delete a component`,
		Example: `  # Create a simple component
  ytr component create --name "Backend" --queue PROJ

  # Create with all fields
  ytr component create --name "Backend" --queue PROJ --description "Backend services" --lead 12345 --assign-auto

  # Create via JSON
  ytr component create --from-json '{"name":"Backend","queue":"PROJ"}'`,
		Args: cobra.NoArgs,
		PreRunE: func(cmd *cobra.Command, _ []string) error {
			// Mutual exclusion: --from-json vs individual flags.
			if cmd.Flags().Changed("from-json") &&
				(cmd.Flags().Changed("name") || cmd.Flags().Changed("queue") ||
					cmd.Flags().Changed("description") || cmd.Flags().Changed("lead") ||
					cmd.Flags().Changed("assign-auto")) {
				return errors.NewUserError(
					"cannot use individual flags and --from-json together",
					"Use --name, --queue, etc. for individual flags, or --from-json for full JSON input",
				)
			}

			// When --from-json NOT set, --name AND --queue MUST be set.
			if !cmd.Flags().Changed("from-json") {
				if !cmd.Flags().Changed("name") || !cmd.Flags().Changed("queue") {
					return errors.NewUserError(
						"--name and --queue are required",
						"Provide --name and --queue for the component, or --from-json for full JSON input",
					)
				}
			}

			return nil
		},
		RunE: func(cmd *cobra.Command, _ []string) error {
			return runCreate(cmd, nameFlag, queueFlag, descriptionFlag, leadFlag, assignAutoFlag, fromJSON)
		},
	}

	cmd.Flags().StringVar(&nameFlag, "name", "", "Component name (required)")
	cmd.Flags().StringVar(&queueFlag, "queue", "", "Queue key (required)")
	cmd.Flags().StringVar(&descriptionFlag, "description", "", "Component description")
	cmd.Flags().StringVar(&leadFlag, "lead", "", "Lead user ID")
	cmd.Flags().BoolVar(&assignAutoFlag, "assign-auto", false, "Auto-assign issues to lead")
	cmd.Flags().StringVar(
		&fromJSON, "from-json", "",
		`JSON input: inline '{"name":"...","queue":"..."}', @file, or - for stdin`,
	)

	jsonfields.Register("ytr component create", ComponentListFields)

	return cmd
}

// runCreate executes the component create logic.
func runCreate(
	cmd *cobra.Command,
	nameFlag, queueFlag, descriptionFlag, leadFlag string,
	assignAutoFlag bool,
	fromJSON string,
) error {
	if output.IsJSON() && !output.HasFieldSelection() && output.JQFilter == "" {
		return output.PrintFieldHint(cmd.ErrOrStderr(), "component create", ComponentListFields)
	}

	if output.JQFilter != "" && !output.HasFieldSelection() {
		output.JSONFields = ComponentListFields
	}

	// Validate requested fields.
	if output.HasFieldSelection() {
		if err := output.ValidateFields(output.JSONFields, ComponentListFields); err != nil {
			return err
		}
		output.JSONFields = output.NormalizeFields(output.JSONFields, ComponentListFields)
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
	req, buildErr := buildCreateRequest(cmd, nameFlag, queueFlag, descriptionFlag, leadFlag, assignAutoFlag, fromJSON)
	if buildErr != nil {
		return buildErr
	}

	creator := newComponentCreator(auth)

	component, _, err := creator.Create(cmd.Context(), req)
	if err != nil {
		return api.MapAPIError(err)
	}

	return renderCreateOutput(cmd.OutOrStdout(), component)
}

// buildCreateRequest constructs a ComponentRequest from flags or --from-json input.
func buildCreateRequest(
	cmd *cobra.Command,
	nameFlag, queueFlag, descriptionFlag, leadFlag string,
	assignAutoFlag bool,
	fromJSON string,
) (*tracker.ComponentRequest, error) {
	if cmd.Flags().Changed("from-json") {
		data, parseErr := validate.ParseJSONInput(fromJSON)
		if parseErr != nil {
			return nil, parseErr
		}
		req := &tracker.ComponentRequest{}
		if unmarshalErr := json.Unmarshal(data, req); unmarshalErr != nil {
			return nil, errors.NewUserError(
				fmt.Sprintf("invalid JSON input: %s", unmarshalErr),
				"Provide valid JSON matching the ComponentRequest format",
			)
		}
		return req, nil
	}

	req := &tracker.ComponentRequest{
		Name:  &nameFlag,
		Queue: &queueFlag,
	}
	if cmd.Flags().Changed("description") {
		req.Description = &descriptionFlag
	}
	if cmd.Flags().Changed("lead") {
		req.Lead = &leadFlag
	}
	if cmd.Flags().Changed("assign-auto") {
		req.AssignAuto = &assignAutoFlag
	}
	return req, nil
}

// renderCreateOutput handles JSON/quiet/table output for a component create result.
func renderCreateOutput(w io.Writer, component *tracker.Component) error {
	if output.IsJSON() {
		item := toComponentItem(component)
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
		output.PrintQuiet(w, api.DerefFlexString(component.ID, ""))
		return nil
	}

	// Table output: brief confirmation.
	_, err := fmt.Fprintf(w, "Component %s created\n", api.DerefFlexString(component.ID, ""))
	return err
}
