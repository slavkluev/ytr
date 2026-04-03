package component

import (
	"fmt"
	"io"

	"github.com/jedib0t/go-pretty/v6/text"
	"github.com/slavkluev/go-yandex-tracker/tracker"
	"github.com/spf13/cobra"

	"github.com/slavkluev/ytr/internal/api"
	"github.com/slavkluev/ytr/internal/cmd/jsonfields"
	"github.com/slavkluev/ytr/internal/config"
	"github.com/slavkluev/ytr/internal/output"
	"github.com/slavkluev/ytr/internal/validate"
)

// ComponentGetFields lists the available JSON field names for component get output.
var ComponentGetFields = []string{"id", "name", "queue", "lead", "description", "assignAuto"}

// componentDetail is a clean struct for JSON serialization of a single component.
type componentDetail struct {
	ID          string `json:"id"`
	Name        string `json:"name"`
	Queue       string `json:"queue,omitempty"`
	Lead        string `json:"lead,omitempty"`
	Description string `json:"description,omitempty"`
	AssignAuto  bool   `json:"assignAuto"`
}

// toComponentDetail converts a tracker.Component into a componentDetail struct for JSON output.
func toComponentDetail(c *tracker.Component) componentDetail {
	queue := ""
	if c.Queue != nil {
		queue = api.DerefString(c.Queue.Key, "")
	}

	return componentDetail{
		ID:          api.DerefFlexString(c.ID, ""),
		Name:        api.DerefString(c.Name, ""),
		Queue:       queue,
		Lead:        api.DerefUser(c.Lead, ""),
		Description: api.DerefString(c.Description, ""),
		AssignAuto:  api.DerefBool(c.AssignAuto, false),
	}
}

// newGetCmd creates the "component get" command for displaying component details.
func newGetCmd() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "get COMPONENT-ID",
		Short: "Show component details",
		Long: `Display detailed information about a Yandex Tracker component.

JSON FIELDS
  id, name, queue, lead, description, assignAuto

SEE ALSO
  ytr component list    - List all components
  ytr component edit    - Edit a component
  ytr component delete  - Delete a component`,
		Example: `  # View component details
  ytr component get 42

  # Get specific fields as JSON
  ytr component get 42 --json name,queue,lead`,
		Args: cobra.ExactArgs(1),
		PreRunE: func(_ *cobra.Command, args []string) error {
			_, err := validate.ValidateNumericID(args[0], "component ID")
			return err
		},
		RunE: func(cmd *cobra.Command, args []string) error {
			return runGet(cmd, args[0])
		},
	}

	jsonfields.Register("ytr component get", ComponentGetFields)

	return cmd
}

// runGet executes the component get logic.
func runGet(cmd *cobra.Command, componentID string) error {
	if output.IsJSON() && !output.HasFieldSelection() && output.JQFilter == "" {
		return output.PrintFieldHint(cmd.ErrOrStderr(), "component get", ComponentGetFields)
	}

	if output.JQFilter != "" && !output.HasFieldSelection() {
		output.JSONFields = ComponentGetFields
	}

	// Validate requested fields.
	if output.HasFieldSelection() {
		if err := output.ValidateFields(output.JSONFields, ComponentGetFields); err != nil {
			return err
		}
		output.JSONFields = output.NormalizeFields(output.JSONFields, ComponentGetFields)
	}

	// Resolve auth from root persistent flags.
	tokenFlag, _ := cmd.Root().PersistentFlags().GetString("token")
	orgIDFlag, _ := cmd.Root().PersistentFlags().GetString("org-id")
	orgTypeFlag, _ := cmd.Root().PersistentFlags().GetString("org-type")

	auth, err := config.ResolveAuth(tokenFlag, orgIDFlag, orgTypeFlag)
	if err != nil {
		return err
	}

	getter := newComponentGetter(auth)

	component, _, err := getter.Get(cmd.Context(), componentID)
	if err != nil {
		return api.MapAPIError(err)
	}

	w := cmd.OutOrStdout()

	if output.IsJSON() {
		detail := toComponentDetail(component)

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
		output.PrintQuiet(w, api.DerefFlexString(component.ID, ""))
		return nil
	}

	return renderComponentCard(w, component)
}

// renderComponentCard renders a bold-label detail card for a component.
func renderComponentCard(w io.Writer, c *tracker.Component) error {
	bold := func(label string) string {
		if output.ColorsEnabled() {
			return text.Colors{text.Bold}.Sprint(label)
		}
		return label
	}

	// writeErr captures the first write error encountered.
	var writeErr error
	printField := func(label, value string) {
		if writeErr != nil {
			return
		}
		_, writeErr = fmt.Fprintf(w, "%s  %s\n", bold(label+":"), value)
	}

	printField("ID", api.DerefFlexString(c.ID, ""))
	printField("Name", api.DerefString(c.Name, "-"))

	queue := "-"
	if c.Queue != nil {
		queue = api.DerefString(c.Queue.Key, "-")
	}
	printField("Queue", queue)

	printField("Lead", api.DerefUser(c.Lead, "-"))

	// Only show Description if non-empty.
	desc := api.DerefString(c.Description, "")
	if desc != "" {
		printField("Description", desc)
	}

	assignAuto := "no"
	if api.DerefBool(c.AssignAuto, false) {
		assignAuto = "yes"
	}
	printField("AssignAuto", assignAuto)

	return writeErr
}
