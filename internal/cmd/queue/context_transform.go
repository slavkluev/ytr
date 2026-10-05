package queue

import (
	"bytes"
	"fmt"
	"slices"

	"github.com/slavkluev/go-yandex-tracker/tracker"

	"github.com/slavkluev/ytr/internal/api"
	"github.com/slavkluev/ytr/internal/jsonenc"
	"github.com/slavkluev/ytr/internal/output"
)

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

// A part left nil is rendered as null: its request failed and incomplete says why.
// A part that was fetched but is empty is a non-nil empty slice, rendered as [].
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

type contextIssueType struct {
	Key      string `json:"key"`
	Name     string `json:"name"`
	Workflow string `json:"workflow"`
}

type contextStatus struct {
	Key  string `json:"key"`
	Name string `json:"name"`
}

type contextWorkflow struct {
	ID            string        `json:"id"`
	InitialStatus string        `json:"initialStatus"`
	Transitions   transitionMap `json:"transitions"`
}

type contextComponent struct {
	ID   string `json:"id"`
	Name string `json:"name"`
}

// A required field's Default is the queue's default key for type and priority,
// which fills the field when the issue does not set it.
type contextRequiredField struct {
	ID      string `json:"id"`
	Default string `json:"default,omitempty"`
}

// A local field's ID is the full <queueId>--<key> form that filters need; its
// Options keep the JSON type Tracker sent for each value.
type contextLocalField struct {
	ID       string `json:"id"`
	Key      string `json:"key"`
	Name     string `json:"name"`
	Schema   string `json:"schema"`
	Readonly bool   `json:"readonly"`
	Options  []any  `json:"options,omitempty"`
}

type contextGlobalField struct {
	Key  string `json:"key"`
	Name string `json:"name"`
}

type incompletePart struct {
	Part   string `json:"part"`
	Reason string `json:"reason"`
}

// transitionMap renders as a JSON object that keeps the steps in workflow order,
// which a Go map would sort away.
type transitionMap struct {
	from    []string
	targets map[string][]string
}

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
		key, err := jsonenc.Marshal(from)
		if err != nil {
			return nil, err
		}
		targets, err := jsonenc.Marshal(m.targets[from])
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

// incomplete is always included, so a narrowed document still says which of its
// parts are missing. With every part selected the document is not cut, which
// keeps its parts in order rather than sorted by name.
func contextDocument(q *tracker.Queue, queueKey string, results contextResults, fields []string) any {
	wanted := wantedParts(fields)
	doc := buildContext(q, queueKey, results, wanted)
	if len(wanted) == len(QueueContextFields) {
		return doc
	}
	selected := output.FilterFields(doc, fields)
	selected["incomplete"] = doc.Incomplete
	return selected
}

func wantedParts(fields []string) map[string]bool {
	if len(fields) == 0 {
		fields = QueueContextFields
	}
	wanted := make(map[string]bool, len(fields))
	for _, f := range fields {
		wanted[f] = true
	}
	return wanted
}

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

func setPart[T any](doc *queueContext, dst *[]T, value []T, part string, err error) {
	if err != nil {
		doc.Incomplete = append(doc.Incomplete, incompletePart{Part: part, Reason: errorReason(err)})
		return
	}
	*dst = value
}

func errorReason(err error) string {
	return api.MapAPIError(err).Error()
}

func defaultTypeKey(q *tracker.Queue) string {
	if q.DefaultType == nil {
		return ""
	}
	return api.DerefString(q.DefaultType.Key, "")
}

func defaultPriorityKey(q *tracker.Queue) string {
	if q.DefaultPriority == nil {
		return ""
	}
	return api.DerefString(q.DefaultPriority.Key, "")
}

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

func initialStatusKey(wf *tracker.Workflow) string {
	if wf.InitialAction == nil || wf.InitialAction.Target == nil {
		return ""
	}
	return api.DerefString(wf.InitialAction.Target.Key, "")
}

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
