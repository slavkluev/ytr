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

// newEditCmd creates the "component edit" command.
func newEditCmd() *cobra.Command {
	var (
		nameFlag        string
		queueFlag       string
		descriptionFlag string
		leadFlag        string
		assignAutoFlag  bool
		fromJSON        string
	)

	cmd := &cobra.Command{
		Use:   "edit COMPONENT-ID",
		Short: "Edit a component",
		Long: `Edit an existing project component in Yandex Tracker.

Provide one or more flags to update, or --from-json for full JSON input.

JSON FIELDS
  id, name, queue, lead, description, assignAuto

SEE ALSO
  ytr component list    - List all components
  ytr component get     - Show component details
  ytr component create  - Create a component
  ytr component delete  - Delete a component`,
		Example: `  # Update component name
  ytr component edit 42 --name "New Name"

  # Update multiple fields
  ytr component edit 42 --name "Backend" --lead 12345 --assign-auto

  # Update via JSON
  ytr component edit 42 --from-json '{"name":"Backend","description":"Updated"}'`,
		Args: cobra.ExactArgs(1),
		PreRunE: func(cmd *cobra.Command, args []string) error {
			// Validate component ID.
			if _, err := validate.ValidateNumericID(args[0], "component ID"); err != nil {
				return err
			}

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

			// At least one flag or --from-json required.
			if !cmd.Flags().Changed("from-json") &&
				!cmd.Flags().Changed("name") && !cmd.Flags().Changed("queue") &&
				!cmd.Flags().Changed("description") && !cmd.Flags().Changed("lead") &&
				!cmd.Flags().Changed("assign-auto") {
				return errors.NewUserError(
					"at least one of --name, --queue, --description, --lead, --assign-auto, or --from-json is required",
					"Provide at least one flag to update, or --from-json for JSON input",
				)
			}

			return nil
		},
		RunE: func(cmd *cobra.Command, args []string) error {
			return runEdit(cmd, args[0], nameFlag, queueFlag, descriptionFlag, leadFlag, assignAutoFlag, fromJSON)
		},
	}

	cmd.Flags().StringVar(&nameFlag, "name", "", "Component name")
	cmd.Flags().StringVar(&queueFlag, "queue", "", "Queue key")
	cmd.Flags().StringVar(&descriptionFlag, "description", "", "Component description")
	cmd.Flags().StringVar(&leadFlag, "lead", "", "Lead user ID")
	cmd.Flags().BoolVar(&assignAutoFlag, "assign-auto", false, "Auto-assign issues to lead")
	cmd.Flags().StringVar(&fromJSON, "from-json", "", `JSON input: inline '{"name":"..."}', @file, or - for stdin`)

	jsonfields.Register("ytr component edit", ComponentListFields)

	return cmd
}

// runEdit executes the component edit logic.
func runEdit(
	cmd *cobra.Command,
	componentID string,
	nameFlag, queueFlag, descriptionFlag, leadFlag string,
	assignAutoFlag bool,
	fromJSON string,
) error {
	if output.IsJSON() && !output.HasFieldSelection() && output.JQFilter == "" {
		return output.PrintFieldHint(cmd.ErrOrStderr(), "component edit", ComponentListFields)
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
	req, buildErr := buildEditRequest(cmd, nameFlag, queueFlag, descriptionFlag, leadFlag, assignAutoFlag, fromJSON)
	if buildErr != nil {
		return buildErr
	}

	editor := newComponentEditor(auth)

	component, _, err := editor.Edit(cmd.Context(), componentID, req)
	if err != nil {
		return api.MapAPIError(err)
	}

	return renderEditOutput(cmd.OutOrStdout(), component)
}

// buildEditRequest constructs a ComponentRequest from flags or --from-json input.
func buildEditRequest(
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

	req := &tracker.ComponentRequest{}
	if cmd.Flags().Changed("name") {
		req.Name = &nameFlag
	}
	if cmd.Flags().Changed("queue") {
		req.Queue = &queueFlag
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

// renderEditOutput handles JSON/quiet/table output for a component edit result.
func renderEditOutput(w io.Writer, component *tracker.Component) error {
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
	_, err := fmt.Fprintf(w, "Component %s updated\n", api.DerefFlexString(component.ID, ""))
	return err
}
