package queue

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"slices"
	"sync"

	"github.com/slavkluev/go-yandex-tracker/tracker"
	"github.com/spf13/cobra"

	"github.com/slavkluev/ytr/internal/api"
	"github.com/slavkluev/ytr/internal/cmd/jsonfields"
	"github.com/slavkluev/ytr/internal/config"
	"github.com/slavkluev/ytr/internal/errors"
	"github.com/slavkluev/ytr/internal/output"
	"github.com/slavkluev/ytr/internal/validate"
)

// QueueContextFields lists the parts of the queue context document, which are
// its top-level JSON keys and the names --json selects.
var QueueContextFields = []string{
	"key",
	"name",
	"defaultType",
	"defaultPriority",
	"issueTypes",
	"statuses",
	"workflows",
	"components",
	"requiredFields",
	"localFields",
	"globalFields",
	"incomplete",
}

// Parts that come from a request of their own, so they can fail on their own.
// The other parts come from the queue itself, which the command cannot do
// without.
const (
	partStatuses       = "statuses"
	partWorkflows      = "workflows"
	partComponents     = "components"
	partRequiredFields = "requiredFields"
	partLocalFields    = "localFields"
	partGlobalFields   = "globalFields"
)

// noQueueFieldsReason is the incomplete reason when /queues/{q}/fields lists
// nothing. That endpoint is empty for many queues, so an empty answer does not
// mean that only summary is required.
const noQueueFieldsReason = "Tracker listed no queue fields, so other fields may be required"

// queueContext is the document queue context prints. A part left nil is
// rendered as null: its request failed and incomplete says why. A part that was
// fetched but is empty is a non-nil empty slice, rendered as [].
type queueContext struct {
	Key             string                 `json:"key"`
	Name            string                 `json:"name"`
	DefaultType     string                 `json:"defaultType"`
	DefaultPriority string                 `json:"defaultPriority"`
	IssueTypes      []contextIssueType     `json:"issueTypes"`
	Statuses        []contextStatus        `json:"statuses"`
	Workflows       []contextWorkflow      `json:"workflows"`
	Components      []contextComponent     `json:"components"`
	RequiredFields  []contextRequiredField `json:"requiredFields"`
	LocalFields     []contextLocalField    `json:"localFields"`
	GlobalFields    []contextGlobalField   `json:"globalFields"`
	Incomplete      []incompletePart       `json:"incomplete"`
}

// contextIssueType is an issue type of the queue and the workflow it follows.
type contextIssueType struct {
	Key      string `json:"key"`
	Name     string `json:"name"`
	Workflow string `json:"workflow"`
}

// contextStatus is a status that a step of one of the queue's workflows has.
type contextStatus struct {
	Key  string `json:"key"`
	Name string `json:"name"`
}

// contextWorkflow is a workflow with the status an issue starts in and the
// statuses each status can move to.
type contextWorkflow struct {
	ID            string        `json:"id"`
	InitialStatus string        `json:"initialStatus"`
	Transitions   transitionMap `json:"transitions"`
}

// contextComponent is a component of the queue.
type contextComponent struct {
	ID   string `json:"id"`
	Name string `json:"name"`
}

// contextRequiredField is a field an issue of the queue needs. Default is the
// queue's default key for type and priority, which fills the field when the
// issue does not set it.
type contextRequiredField struct {
	ID      string `json:"id"`
	Default string `json:"default,omitempty"`
}

// contextLocalField is a field local to the queue. ID is the full
// <queueId>--<key> form that filters need; Options keeps the JSON
// type Tracker sent for each value.
type contextLocalField struct {
	ID       string `json:"id"`
	Key      string `json:"key"`
	Name     string `json:"name"`
	Schema   string `json:"schema"`
	Readonly bool   `json:"readonly"`
	Options  []any  `json:"options,omitempty"`
}

// contextGlobalField is an entry of the editable global field index.
type contextGlobalField struct {
	Key  string `json:"key"`
	Name string `json:"name"`
}

// incompletePart names a part that could not be fetched, or that may be
// missing entries, and why.
type incompletePart struct {
	Part   string `json:"part"`
	Reason string `json:"reason"`
}

// transitionMap maps each workflow step's status key to the status keys its
// actions lead to. It renders as a JSON object that keeps the steps in
// workflow order, which a Go map would sort away.
type transitionMap struct {
	from    []string
	targets map[string][]string
}

// add records the targets of one step's actions under its status key,
// skipping targets it already has. A step without actions gets an empty list.
func (m *transitionMap) add(from string, actions []*tracker.WorkflowAction) {
	if m.targets == nil {
		m.targets = make(map[string][]string)
	}
	if _, ok := m.targets[from]; !ok {
		m.from = append(m.from, from)
		m.targets[from] = []string{}
	}
	for _, action := range actions {
		if action == nil || action.Target == nil {
			continue
		}
		to := api.DerefString(action.Target.Key, "")
		if to == "" || slices.Contains(m.targets[from], to) {
			continue
		}
		m.targets[from] = append(m.targets[from], to)
	}
}

// MarshalJSON renders the map as a JSON object in workflow step order.
func (m transitionMap) MarshalJSON() ([]byte, error) {
	var buf bytes.Buffer
	buf.WriteByte('{')
	for i, from := range m.from {
		if i > 0 {
			buf.WriteByte(',')
		}
		key, err := json.Marshal(from)
		if err != nil {
			return nil, err
		}
		targets, err := json.Marshal(m.targets[from])
		if err != nil {
			return nil, err
		}
		buf.Write(key)
		buf.WriteByte(':')
		buf.Write(targets)
	}
	buf.WriteByte('}')
	return buf.Bytes(), nil
}

// contextResults holds what each part's request returned. Each request runs in
// its own goroutine and writes only its own fields, which are read after all
// of them finish.
type contextResults struct {
	workflows      []*tracker.Workflow
	failedWorkflow string
	workflowsErr   error
	components     []*tracker.Component
	componentsErr  error
	queueFields    []*tracker.Field
	queueFieldsErr error
	localFields    []*tracker.Field
	localFieldsErr error
	globalFields   []*tracker.Field
	globalErr      error
}

const contextLong = `Show what an agent needs to create and move issues in a queue, as one JSON
document: issue types, statuses, workflow transitions, components, defaults,
required fields, local fields, and the editable global fields.

The document is always JSON: with no flags it is the whole document, --json
selects parts and makes only the requests they need, and --jq filters the
document. --quiet is rejected. As for every command, an error is a JSON
document on stderr only under --json or --jq.

key, name, defaultType, defaultPriority, and issueTypes come from the queue
request. statuses and workflows share the workflow requests, and each other
part has a request of its own. A part whose request failed is null, and
incomplete names it with the server's reason; a part that was fetched but is
empty is []. incomplete is always present. Once the queue itself is fetched the
command exits 0, so check incomplete before relying on a part.

PARTS
  key, name        the queue's key and name
  defaultType      default issue type key (what issue create --type takes)
  defaultPriority  default priority key (what issue create --priority takes)
  issueTypes       key, name, and the id of the workflow each type follows
  statuses         every status of the queue's workflows, once
  workflows        id, initialStatus, and transitions: each status key maps to
                   the status keys its actions lead to, [] for a status with
                   no outgoing transitions
  components       id and name
  requiredFields   summary, then every field the queue marks required; default
                   is the queue's default for type and priority. When Tracker
                   lists no queue fields, incomplete says so: other fields may
                   still be required
  localFields      id is the full <queueId>--<key> that --filter needs;
                   options are the allowed values
  globalFields     editable global fields, key and name only; ytr field get KEY
                   shows a field's schema and values
  incomplete       {part, reason} for each part that is null or may be missing
                   entries

JSON FIELDS
  key, name, defaultType, defaultPriority, issueTypes, statuses, workflows, components, requiredFields, localFields, globalFields, incomplete

SEE ALSO
  ytr queue view   - View queue details
  ytr field get    - Show a field's schema and allowed values
  ytr issue create - Create an issue`

// newContextCmd creates the "queue context" command.
func newContextCmd() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "context QUEUE-KEY",
		Short: "Show what is needed to create and move issues in a queue",
		Long:  contextLong,
		Example: `  # Everything needed to work in a queue
  ytr queue context PROJ

  # Only issue types and components (no workflow or field requests)
  ytr queue context PROJ --json issueTypes,components

  # Statuses an issue in "open" can move to
  ytr queue context PROJ --json workflows --jq '.workflows[].transitions.open'

  # Full local field ids for --filter
  ytr queue context PROJ --json localFields --jq '.localFields[] | {id, options}'`,
		Args: cobra.ExactArgs(1),
		PreRunE: func(_ *cobra.Command, args []string) error {
			_, err := validate.ValidateStringID(args[0], "queue key")
			return err
		},
		RunE: func(cmd *cobra.Command, args []string) error {
			queueKey, _ := validate.ValidateStringID(args[0], "queue key")
			return runContext(cmd, queueKey)
		},
	}

	jsonfields.Register("ytr queue context", QueueContextFields)

	return cmd
}

// runContext executes the queue context logic.
func runContext(cmd *cobra.Command, queueKey string) error {
	if output.IsQuiet() {
		return errors.NewUserError(
			"queue context does not support --quiet: its document is always JSON",
			"Run without --quiet: ytr queue context "+queueKey,
		)
	}

	if output.WantsFieldHint(cmd.Flags().Changed("json")) {
		return output.PrintFieldHint(cmd.ErrOrStderr(), "queue context", QueueContextFields)
	}

	if output.HasFieldSelection() {
		if err := output.ValidateFields(output.JSONFields, QueueContextFields); err != nil {
			return err
		}
		output.JSONFields = output.NormalizeFields(output.JSONFields, QueueContextFields)
	}

	// Resolve auth from root persistent flags.
	tokenFlag, _ := cmd.Root().PersistentFlags().GetString("token")
	orgIDFlag, _ := cmd.Root().PersistentFlags().GetString("org-id")
	orgTypeFlag, _ := cmd.Root().PersistentFlags().GetString("org-type")

	auth, err := config.ResolveAuth(tokenFlag, orgIDFlag, orgTypeFlag)
	if err != nil {
		return err
	}

	client := newContextClient(auth)

	// The queue comes first: without it there is no document, and its
	// issueTypesConfig names the workflows to fetch.
	q, _, err := client.GetQueue(cmd.Context(), queueKey, &tracker.QueueGetOptions{Expand: "issueTypesConfig"})
	if err != nil {
		return api.MapAPIError(err)
	}

	wanted := wantedParts()
	results := fetchParts(cmd.Context(), client, queueKey, workflowIDs(q), wanted)

	return renderContext(cmd.OutOrStdout(), buildContext(q, queueKey, results, wanted))
}

// wantedParts returns the parts --json selects, or every part without a
// selection.
func wantedParts() map[string]bool {
	fields := QueueContextFields
	if output.HasFieldSelection() {
		fields = output.JSONFields
	}
	wanted := make(map[string]bool, len(fields))
	for _, f := range fields {
		wanted[f] = true
	}
	return wanted
}

// workflowIDs returns the ids of the workflows the queue's issue types follow,
// once each, in first-seen order.
func workflowIDs(q *tracker.Queue) []string {
	var ids []string
	for _, cfg := range q.IssueTypesConfig {
		if cfg == nil || cfg.Workflow == nil {
			continue
		}
		id := api.DerefFlexString(cfg.Workflow.ID, "")
		if id != "" && !slices.Contains(ids, id) {
			ids = append(ids, id)
		}
	}
	return ids
}

// fetchParts makes the requests the wanted parts need, concurrently.
func fetchParts(
	ctx context.Context,
	client queueContextClient,
	queueKey string,
	workflows []string,
	wanted map[string]bool,
) contextResults {
	var r contextResults
	var wg sync.WaitGroup

	if wanted[partStatuses] || wanted[partWorkflows] {
		wg.Go(func() {
			r.workflows, r.failedWorkflow, r.workflowsErr = fetchWorkflows(ctx, client, workflows)
		})
	}
	if wanted[partComponents] {
		wg.Go(func() {
			opts := &tracker.QueueComponentsListOptions{Fields: "name"}
			r.components, _, r.componentsErr = client.ListComponents(ctx, queueKey, opts)
		})
	}
	if wanted[partRequiredFields] {
		wg.Go(func() {
			r.queueFields, _, r.queueFieldsErr = client.ListQueueFields(ctx, queueKey)
		})
	}
	if wanted[partLocalFields] {
		wg.Go(func() {
			r.localFields, _, r.localFieldsErr = client.ListLocalFields(ctx, queueKey)
		})
	}
	if wanted[partGlobalFields] {
		wg.Go(func() {
			r.globalFields, _, r.globalErr = client.ListGlobalFields(ctx)
		})
	}

	wg.Wait()
	return r
}

// fetchWorkflows fetches each workflow in order and stops at the first
// failure, returning the id that failed: one missing workflow leaves the
// statuses and transitions incomplete, so the rest would not be used.
func fetchWorkflows(
	ctx context.Context,
	client queueContextClient,
	ids []string,
) ([]*tracker.Workflow, string, error) {
	workflows := make([]*tracker.Workflow, 0, len(ids))
	for _, id := range ids {
		wf, _, err := client.GetWorkflow(ctx, id)
		if err != nil {
			return nil, id, err
		}
		workflows = append(workflows, wf)
	}
	return workflows, "", nil
}

// buildContext assembles the document from the queue and the part results.
// incomplete follows the document's part order.
func buildContext(q *tracker.Queue, queueKey string, r contextResults, wanted map[string]bool) *queueContext {
	doc := &queueContext{
		Key:             api.DerefString(q.Key, ""),
		Name:            api.DerefString(q.Name, ""),
		DefaultType:     defaultTypeKey(q),
		DefaultPriority: defaultPriorityKey(q),
		IssueTypes:      toContextIssueTypes(q.IssueTypesConfig),
		Incomplete:      []incompletePart{},
	}

	if wanted[partStatuses] || wanted[partWorkflows] {
		doc.setWorkflows(r, wanted)
	}
	if wanted[partComponents] {
		setPart(doc, &doc.Components, toContextComponents(r.components), partComponents, r.componentsErr)
	}
	if wanted[partRequiredFields] {
		setPart(doc, &doc.RequiredFields, toRequiredFields(r.queueFields, q), partRequiredFields, r.queueFieldsErr)
		if r.queueFieldsErr == nil && len(r.queueFields) == 0 {
			doc.Incomplete = append(
				doc.Incomplete,
				incompletePart{Part: partRequiredFields, Reason: noQueueFieldsReason},
			)
		}
	}
	if wanted[partLocalFields] {
		localFields := toContextLocalFields(r.localFields, api.DerefString(q.Key, queueKey))
		setPart(doc, &doc.LocalFields, localFields, partLocalFields, r.localFieldsErr)
	}
	if wanted[partGlobalFields] {
		setPart(doc, &doc.GlobalFields, toContextGlobalFields(r.globalFields), partGlobalFields, r.globalErr)
	}

	return doc
}

// setWorkflows fills statuses and workflows, which both come from the
// workflow requests. When one of those failed, both stay null, and each one
// the selection holds is named in incomplete with the failed workflow's id.
func (doc *queueContext) setWorkflows(r contextResults, wanted map[string]bool) {
	if r.workflowsErr == nil {
		doc.Statuses, doc.Workflows = toContextWorkflows(r.workflows)
		return
	}

	reason := fmt.Sprintf("workflow %s: %s", r.failedWorkflow, errorReason(r.workflowsErr))
	for _, part := range []string{partStatuses, partWorkflows} {
		if wanted[part] {
			doc.Incomplete = append(doc.Incomplete, incompletePart{Part: part, Reason: reason})
		}
	}
}

// setPart stores a fetched part in dst. When its request failed, dst stays
// null and the part is named in incomplete with the server's reason.
func setPart[T any](doc *queueContext, dst *[]T, value []T, part string, err error) {
	if err != nil {
		doc.Incomplete = append(doc.Incomplete, incompletePart{Part: part, Reason: errorReason(err)})
		return
	}
	*dst = value
}

// errorReason is the text of a failed request as ytr would report it: the
// server's own message when it sent one.
func errorReason(err error) string {
	return api.MapAPIError(err).Error()
}

// defaultTypeKey returns the key of the queue's default issue type, or "".
func defaultTypeKey(q *tracker.Queue) string {
	if q.DefaultType == nil {
		return ""
	}
	return api.DerefString(q.DefaultType.Key, "")
}

// defaultPriorityKey returns the key of the queue's default priority, or "".
func defaultPriorityKey(q *tracker.Queue) string {
	if q.DefaultPriority == nil {
		return ""
	}
	return api.DerefString(q.DefaultPriority.Key, "")
}

// toContextIssueTypes lists the queue's issue types with their workflow ids.
func toContextIssueTypes(configs []*tracker.QueueIssueTypeConfig) []contextIssueType {
	types := make([]contextIssueType, 0, len(configs))
	for _, cfg := range configs {
		if cfg == nil || cfg.IssueType == nil {
			continue
		}
		t := contextIssueType{
			Key:  api.DerefString(cfg.IssueType.Key, ""),
			Name: api.DerefString(cfg.IssueType.Display, ""),
		}
		if cfg.Workflow != nil {
			t.Workflow = api.DerefFlexString(cfg.Workflow.ID, "")
		}
		types = append(types, t)
	}
	return types
}

// toContextWorkflows converts fetched workflows, and collects every step
// status once, in first-seen order.
func toContextWorkflows(workflows []*tracker.Workflow) ([]contextStatus, []contextWorkflow) {
	statuses := []contextStatus{}
	seen := make(map[string]bool)
	converted := make([]contextWorkflow, 0, len(workflows))

	for _, wf := range workflows {
		if wf == nil {
			continue
		}
		cw := contextWorkflow{
			ID:            api.DerefFlexString(wf.ID, ""),
			InitialStatus: initialStatusKey(wf),
			Transitions:   transitionMap{targets: make(map[string][]string)},
		}
		for _, step := range wf.Steps {
			if step == nil || step.Status == nil {
				continue
			}
			key := api.DerefString(step.Status.Key, "")
			if key == "" {
				continue
			}
			if !seen[key] {
				seen[key] = true
				statuses = append(statuses, contextStatus{Key: key, Name: api.DerefString(step.Status.Display, "")})
			}
			cw.Transitions.add(key, step.Actions)
		}
		converted = append(converted, cw)
	}

	return statuses, converted
}

// initialStatusKey returns the status key a new issue starts in, or "".
func initialStatusKey(wf *tracker.Workflow) string {
	if wf.InitialAction == nil || wf.InitialAction.Target == nil {
		return ""
	}
	return api.DerefString(wf.InitialAction.Target.Key, "")
}

// toContextComponents converts the queue's components.
func toContextComponents(components []*tracker.Component) []contextComponent {
	converted := make([]contextComponent, 0, len(components))
	for _, c := range components {
		if c == nil {
			continue
		}
		converted = append(converted, contextComponent{
			ID:   api.DerefFlexString(c.ID, ""),
			Name: api.DerefString(c.Name, ""),
		})
	}
	return converted
}

// toRequiredFields lists summary, which the create API always requires, then
// every queue field whose schema marks it required, each once. type and
// priority carry the queue's default key when it has one.
func toRequiredFields(fields []*tracker.Field, q *tracker.Queue) []contextRequiredField {
	defaults := map[string]string{
		"type":     defaultTypeKey(q),
		"priority": defaultPriorityKey(q),
	}

	required := []contextRequiredField{{ID: "summary"}}
	seen := map[string]bool{"summary": true}
	for _, f := range fields {
		if f == nil || f.Schema == nil || !api.DerefBool(f.Schema.Required, false) {
			continue
		}
		id := api.DerefFlexString(f.ID, api.DerefString(f.Key, ""))
		if id == "" || seen[id] {
			continue
		}
		seen[id] = true
		required = append(required, contextRequiredField{ID: id, Default: defaults[id]})
	}
	return required
}

// toContextLocalFields converts the queue's local fields.
func toContextLocalFields(fields []*tracker.Field, queueKey string) []contextLocalField {
	converted := make([]contextLocalField, 0, len(fields))
	for _, f := range fields {
		if f == nil {
			continue
		}
		lf := contextLocalField{
			ID:       api.DerefFlexString(f.ID, ""),
			Key:      api.DerefString(f.Key, ""),
			Name:     api.DerefString(f.Name, ""),
			Readonly: api.DerefBool(f.Readonly, false),
			Options:  fieldOptions(f.OptionsProvider, queueKey),
		}
		if f.Schema != nil {
			lf.Schema = api.DerefString(f.Schema.Type, "")
		}
		converted = append(converted, lf)
	}
	return converted
}

// fieldOptions returns a field's allowed values: its flat list, else this
// queue's entry in the per-queue lists, else Tracker's defaults list.
func fieldOptions(p *tracker.OptionsProvider, queueKey string) []any {
	if p == nil {
		return nil
	}
	if len(p.Values) > 0 {
		return p.Values
	}
	if values, ok := p.QueueValues[queueKey]; ok {
		return values
	}
	return p.Defaults
}

// toContextGlobalFields lists the editable global fields, key and name only.
func toContextGlobalFields(fields []*tracker.Field) []contextGlobalField {
	converted := make([]contextGlobalField, 0, len(fields))
	for _, f := range fields {
		if f == nil || api.DerefBool(f.Readonly, false) {
			continue
		}
		converted = append(converted, contextGlobalField{
			Key:  api.DerefString(f.Key, ""),
			Name: api.DerefString(f.Name, ""),
		})
	}
	return converted
}

// renderContext prints the document, narrowed to the --json parts when there
// is a selection. incomplete is always included, so a narrowed document still
// says which of its parts are missing.
func renderContext(w io.Writer, doc *queueContext) error {
	var data any = doc
	if output.HasFieldSelection() {
		filtered := output.FilterFields(doc, output.JSONFields)
		filtered["incomplete"] = doc.Incomplete
		data = filtered
	}

	if output.JQFilter != "" {
		return output.ApplyJQ(w, data, output.JQFilter)
	}
	return output.PrintJSON(w, data)
}
