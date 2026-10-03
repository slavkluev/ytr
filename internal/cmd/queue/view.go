package queue

import (
	"io"

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
	"leadId",
	"defaultType",
	"defaultPriority",
	"assignAuto",
	"allowExternals",
}

// Uses value types with json tags to avoid null fields from pointer types.
type queueDetail struct {
	Key             string `json:"key"`
	Name            string `json:"name"`
	Description     string `json:"description,omitempty"`
	Lead            string `json:"lead,omitempty"`
	LeadID          string `json:"leadId"`
	DefaultType     string `json:"defaultType,omitempty"`
	DefaultPriority string `json:"defaultPriority,omitempty"`
	AssignAuto      bool   `json:"assignAuto"`
	AllowExternals  bool   `json:"allowExternals"`
}

func newViewCmd() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "view QUEUE-KEY",
		Short: "View queue details",
		Long: `Display detailed information about a Yandex Tracker queue.

JSON FIELDS
  key, name, description, lead, leadId, defaultType, defaultPriority, assignAuto, allowExternals

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

func runView(cmd *cobra.Command, args []string) error {
	if output.WantsFieldHint(cmd.Flags().Changed("json")) {
		return output.PrintFieldHint(cmd.ErrOrStderr(), "queue view", QueueDetailFields)
	}

	if output.JQFilter != "" && !output.HasFieldSelection() {
		output.JSONFields = QueueDetailFields
	}

	if output.HasFieldSelection() {
		if err := output.ValidateFields(output.JSONFields, QueueDetailFields); err != nil {
			return err
		}
		output.JSONFields = output.NormalizeFields(output.JSONFields, QueueDetailFields)
	}

	queueKey := args[0]

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

func renderDetailJSON(w io.Writer, q *tracker.Queue) error {
	detail := queueDetail{
		Key:             api.DerefString(q.Key, ""),
		Name:            api.DerefString(q.Name, ""),
		Lead:            api.DerefUser(q.Lead, ""),
		LeadID:          api.DerefUserID(q.Lead, ""),
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

func renderDetailTable(w io.Writer, q *tracker.Queue) error {
	d := output.NewDetail(w)

	d.Field("Key", api.DerefString(q.Key, "-"))
	d.Field("Name", api.DerefString(q.Name, "-"))
	d.Field("Lead", api.DerefUser(q.Lead, "-"))
	d.Field("Default Type", derefIssueTypeOrFallback(q.DefaultType, "-"))
	d.Field("Default Priority", derefPriorityOrFallback(q.DefaultPriority, "-"))

	if q.Description != nil && *q.Description != "" {
		d.Block("Description", *q.Description)
	}

	return d.Err()
}

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

func derefIssueTypeOrFallback(t *tracker.IssueType, fallback string) string {
	result := derefIssueType(t)
	if result == "" {
		return fallback
	}
	return result
}

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

func derefPriorityOrFallback(p *tracker.Priority, fallback string) string {
	result := derefPriority(p)
	if result == "" {
		return fallback
	}
	return result
}

func derefBool(b *bool) bool {
	if b != nil {
		return *b
	}
	return false
}
