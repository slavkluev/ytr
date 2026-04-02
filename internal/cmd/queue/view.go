package queue

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
)

// QueueDetailFields lists the available JSON field names for queue detail output.
var QueueDetailFields = []string{
	"key",
	"name",
	"description",
	"lead",
	"defaultType",
	"defaultPriority",
	"assignAuto",
	"allowExternals",
}

// queueDetail is a clean struct for JSON serialization of a single queue.
// Uses value types with json tags to avoid null fields from pointer types.
type queueDetail struct {
	Key             string `json:"key"`
	Name            string `json:"name"`
	Description     string `json:"description,omitempty"`
	Lead            string `json:"lead,omitempty"`
	DefaultType     string `json:"defaultType,omitempty"`
	DefaultPriority string `json:"defaultPriority,omitempty"`
	AssignAuto      bool   `json:"assignAuto"`
	AllowExternals  bool   `json:"allowExternals"`
}

// newViewCmd creates the "queue view" command for displaying queue details.
func newViewCmd() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "view QUEUE-KEY",
		Short: "View queue details",
		Long: `Display detailed information about a Yandex Tracker queue.

JSON FIELDS
  key, name, description, lead, defaultType, defaultPriority, assignAuto, allowExternals

SEE ALSO
  ytr queue list    - List queues
  ytr issue list    - List issues in a queue`,
		Example: `  # View queue details
  ytr queue view PROJ

  # Get queue config as JSON
  ytr queue view PROJ --json key,name,lead,defaultType`,
		Args: cobra.ExactArgs(1),
		RunE: runView,
	}

	jsonfields.Register("ytr queue view", QueueDetailFields)

	return cmd
}

// runView executes the queue view logic.
func runView(cmd *cobra.Command, args []string) error {
	// Handle --json field hint (no fields specified, D-10).
	if output.IsJSON() && !output.HasFieldSelection() && output.JQFilter == "" {
		return output.PrintFieldHint(cmd.ErrOrStderr(), "queue view", QueueDetailFields)
	}

	// If --jq without --json: auto-populate all fields (Pitfall 5).
	if output.JQFilter != "" && !output.HasFieldSelection() {
		output.JSONFields = QueueDetailFields
	}

	// Validate requested fields.
	if output.HasFieldSelection() {
		if err := output.ValidateFields(output.JSONFields, QueueDetailFields); err != nil {
			return err
		}
		output.JSONFields = output.NormalizeFields(output.JSONFields, QueueDetailFields)
	}

	queueKey := args[0]

	// Resolve auth from root persistent flags.
	tokenFlag, _ := cmd.Root().PersistentFlags().GetString("token")
	orgIDFlag, _ := cmd.Root().PersistentFlags().GetString("org-id")
	orgTypeFlag, _ := cmd.Root().PersistentFlags().GetString("org-type")

	auth, err := config.ResolveAuth(tokenFlag, orgIDFlag, orgTypeFlag)
	if err != nil {
		return err
	}

	getter := newGetter(auth)

	q, _, err := getter.Get(cmd.Context(), queueKey, nil)
	if err != nil {
		return api.MapAPIError(err)
	}

	return renderDetailOutput(cmd.OutOrStdout(), q)
}

// renderDetailOutput renders a queue in JSON, quiet, or table mode.
func renderDetailOutput(w io.Writer, q *tracker.Queue) error {
	if output.IsJSON() {
		return renderDetailJSON(w, q)
	}

	if output.IsQuiet() {
		output.PrintQuiet(w, api.DerefString(q.Key, ""))
		return nil
	}

	return renderDetailTable(w, q)
}

// renderDetailJSON renders queue detail as JSON with field selection and JQ support.
func renderDetailJSON(w io.Writer, q *tracker.Queue) error {
	detail := queueDetail{
		Key:             api.DerefString(q.Key, ""),
		Name:            api.DerefString(q.Name, ""),
		Lead:            api.DerefUser(q.Lead, ""),
		DefaultType:     derefIssueType(q.DefaultType),
		DefaultPriority: derefPriority(q.DefaultPriority),
		AssignAuto:      derefBool(q.AssignAuto),
		AllowExternals:  derefBool(q.AllowExternals),
	}
	if q.Description != nil {
		detail.Description = *q.Description
	}

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

// renderDetailTable renders the queue as labeled key-value rows.
func renderDetailTable(w io.Writer, q *tracker.Queue) error {
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

	printField("Key", api.DerefString(q.Key, "-"))
	printField("Name", api.DerefString(q.Name, "-"))
	printField("Lead", api.DerefUser(q.Lead, "-"))
	printField("Default Type", derefIssueTypeOrFallback(q.DefaultType, "-"))
	printField("Default Priority", derefPriorityOrFallback(q.DefaultPriority, "-"))

	if writeErr != nil {
		return writeErr
	}

	// Description with separator.
	if q.Description != nil && *q.Description != "" {
		if _, err := fmt.Fprintln(w); err != nil {
			return err
		}
		if _, err := fmt.Fprintf(w, "%s\n", bold("Description:")); err != nil {
			return err
		}
		if _, err := fmt.Fprintf(w, "  %s\n", *q.Description); err != nil {
			return err
		}
	}

	return nil
}

// derefIssueType extracts the display name from a *tracker.IssueType.
// Returns empty string if nil.
func derefIssueType(t *tracker.IssueType) string {
	if t == nil {
		return ""
	}
	if t.Display != nil {
		return *t.Display
	}
	if t.Name != nil {
		return *t.Name
	}
	if t.Key != nil {
		return *t.Key
	}
	return ""
}

// derefIssueTypeOrFallback extracts the display name from a *tracker.IssueType.
// Returns fallback if nil or no displayable fields.
func derefIssueTypeOrFallback(t *tracker.IssueType, fallback string) string {
	result := derefIssueType(t)
	if result == "" {
		return fallback
	}
	return result
}

// derefPriority extracts the display name from a *tracker.Priority.
// Returns empty string if nil.
func derefPriority(p *tracker.Priority) string {
	if p == nil {
		return ""
	}
	if p.Display != nil {
		return *p.Display
	}
	if p.Name != nil {
		return *p.Name
	}
	if p.Key != nil {
		return *p.Key
	}
	return ""
}

// derefPriorityOrFallback extracts the display name from a *tracker.Priority.
// Returns fallback if nil or no displayable fields.
func derefPriorityOrFallback(p *tracker.Priority, fallback string) string {
	result := derefPriority(p)
	if result == "" {
		return fallback
	}
	return result
}

// derefBool safely dereferences a *bool pointer.
// Returns false if nil.
func derefBool(b *bool) bool {
	if b != nil {
		return *b
	}
	return false
}
