package issue

import (
	"fmt"
	"strconv"
	"strings"
	"time"

	"github.com/slavkluev/go-yandex-tracker/tracker"

	"github.com/slavkluev/ytr/internal/api"
	"github.com/slavkluev/ytr/internal/output"
)

type changelogEntry struct {
	Date               string             `json:"date"`
	Author             string             `json:"author"`
	AuthorID           string             `json:"authorId"`
	Type               string             `json:"type"`
	Transport          string             `json:"transport,omitempty"`
	Fields             []fieldChange      `json:"fields,omitempty"`
	Comments           []commentChange    `json:"comments,omitempty"`
	Links              []linkChange       `json:"links,omitempty"`
	Attachments        []attachmentChange `json:"attachments,omitempty"`
	Worklog            []worklogChange    `json:"worklog,omitempty"`
	RelatedResolutions []resolutionChange `json:"relatedResolutions,omitempty"`
}

type fieldChange struct {
	Field        string `json:"field"`
	FieldDisplay string `json:"fieldDisplay,omitempty"`
	From         any    `json:"from,omitempty"`
	To           any    `json:"to,omitempty"`
}

type commentChange struct {
	Action   string `json:"action"`
	ID       string `json:"id"`
	From     string `json:"from,omitempty"`
	To       string `json:"to,omitempty"`
	Reaction string `json:"reaction,omitempty"`
}

type linkChange struct {
	From *linkValue `json:"from,omitempty"`
	To   *linkValue `json:"to,omitempty"`
}

type linkValue struct {
	Direction    string `json:"direction"`
	Issue        string `json:"issue"`
	IssueDisplay string `json:"issueDisplay,omitempty"`
	LinkType     string `json:"linkType"`
	LinkTypeName string `json:"linkTypeName,omitempty"`
}

type attachmentChange struct {
	Action string `json:"action"`
	ID     string `json:"id"`
	Name   string `json:"name"`
}

type worklogChange struct {
	Record        string        `json:"record"`
	RecordDisplay string        `json:"recordDisplay,omitempty"`
	From          *worklogValue `json:"from,omitempty"`
	To            *worklogValue `json:"to,omitempty"`
}

type worklogValue struct {
	Duration string `json:"duration"`
	Start    string `json:"start,omitempty"`
}

type resolutionChange struct {
	Direction         string `json:"direction"`
	Issue             string `json:"issue"`
	IssueDisplay      string `json:"issueDisplay,omitempty"`
	LinkType          string `json:"linkType"`
	LinkTypeName      string `json:"linkTypeName,omitempty"`
	Resolution        string `json:"resolution"`
	ResolutionDisplay string `json:"resolutionDisplay,omitempty"`
}

const (
	dirOutward = "outward"
	dirInward  = "inward"
)

// changelogPagination leaves Total nil, printed as null: Tracker sends no count
// for the changelog, neither in the body nor in a header.
func changelogPagination(entries []*tracker.Changelog, limit int) output.PaginationMeta {
	if len(entries) != limit {
		return output.PaginationMeta{}
	}
	return output.PaginationMeta{Cursor: lastChangelogCursorID(entries), HasMore: true}
}

// A null element in the API's JSON array decodes to a nil *tracker.Changelog;
// reading .ID off it would panic, so trailing nils are skipped — they carry no
// data and are dropped when rendered.
func lastChangelogCursorID(entries []*tracker.Changelog) string {
	for i := len(entries) - 1; i >= 0; i-- {
		if e := entries[i]; e != nil {
			return api.DerefFlexString(e.ID, "")
		}
	}
	return ""
}

func normalizeChangelog(entries []*tracker.Changelog) []changelogEntry {
	result := make([]changelogEntry, 0, len(entries))
	for _, e := range entries {
		if e == nil {
			continue
		}
		entry := changelogEntry{
			Author:    e.UpdatedBy.DisplayOr(""),
			AuthorID:  e.UpdatedBy.IDOr(""),
			Type:      api.DerefString(e.Type, ""),
			Transport: api.DerefString(e.Transport, ""),
		}
		if e.UpdatedAt != nil {
			entry.Date = e.UpdatedAt.Format(time.RFC3339)
		}
		entry.Fields = normalizeFields(e.Fields)
		entry.Comments = normalizeComments(e.Comments)
		entry.Links = normalizeLinks(e.Links)
		entry.Attachments = normalizeAttachments(e.Attachments)
		entry.Worklog = normalizeWorklog(e.Worklog)
		entry.RelatedResolutions = normalizeRelatedResolutions(e.RelatedResolutions)
		result = append(result, entry)
	}
	return result
}

func normalizeFields(events []*tracker.ChangelogEvent) []fieldChange {
	if len(events) == 0 {
		return nil
	}
	result := make([]fieldChange, 0, len(events))
	for _, ev := range events {
		if ev == nil {
			continue
		}
		fc := fieldChange{
			Field: changelogFieldName(ev.Field),
			From:  stripSelfURLs(ev.From),
			To:    stripSelfURLs(ev.To),
		}
		if ev.Field != nil && ev.Field.Display != nil {
			fc.FieldDisplay = *ev.Field.Display
		}
		result = append(result, fc)
	}
	return result
}

func normalizeComments(c *tracker.ChangelogComments) []commentChange {
	if c == nil {
		return nil
	}
	var result []commentChange
	for _, ref := range c.Added {
		if ref == nil {
			continue
		}
		result = append(result, commentChange{
			Action: "added",
			ID:     api.DerefFlexString(ref.ID, ""),
			To:     api.DerefString(ref.Display, ""),
		})
	}
	for _, ref := range c.Removed {
		if ref == nil {
			continue
		}
		result = append(result, commentChange{
			Action: "removed",
			ID:     api.DerefFlexString(ref.ID, ""),
			From:   api.DerefString(ref.Display, ""),
		})
	}
	for _, cu := range c.Updated {
		if cu == nil {
			continue
		}
		id := ""
		if cu.Comment != nil {
			id = api.DerefFlexString(cu.Comment.ID, "")
		}
		switch {
		case cu.AddedReaction != nil:
			result = append(result, commentChange{
				Action:   "reactionAdded",
				ID:       id,
				Reaction: *cu.AddedReaction,
			})
		case cu.RemovedReaction != nil:
			result = append(result, commentChange{
				Action:   "reactionRemoved",
				ID:       id,
				Reaction: *cu.RemovedReaction,
			})
		default:
			result = append(result, commentChange{
				Action: "updated",
				ID:     id,
				From:   normalizeChangeValue(cu.From),
				To:     normalizeChangeValue(cu.To),
			})
		}
	}
	return result
}

func normalizeLinks(links []*tracker.ChangelogLink) []linkChange {
	if len(links) == 0 {
		return nil
	}
	result := make([]linkChange, 0, len(links))
	for _, l := range links {
		if l == nil {
			continue
		}
		result = append(result, linkChange{
			From: normalizeLinkValue(l.From),
			To:   normalizeLinkValue(l.To),
		})
	}
	return result
}

func normalizeLinkValue(v *tracker.ChangelogLinkValue) *linkValue {
	if v == nil {
		return nil
	}
	lv := &linkValue{
		Direction: api.DerefString(v.Direction, ""),
	}
	if v.Object != nil {
		lv.Issue = api.DerefString(v.Object.Key, "")
		lv.IssueDisplay = api.DerefString(v.Object.Display, "")
	}
	if v.Type != nil {
		lv.LinkType = api.DerefFlexString(v.Type.ID, "")
		lv.LinkTypeName = linkTypeNameByDirection(v.Type, lv.Direction)
	}
	return lv
}

// For asymmetric link types (e.g., depends), outward and inward have different names.
func linkTypeNameByDirection(lt *tracker.IssueLinkType, direction string) string {
	if lt == nil {
		return ""
	}
	switch direction {
	case dirOutward:
		return api.DerefString(lt.Outward, "")
	case dirInward:
		return api.DerefString(lt.Inward, "")
	default:
		return api.DerefFlexString(lt.ID, "")
	}
}

func normalizeAttachments(a *tracker.ChangelogAttachments) []attachmentChange {
	if a == nil {
		return nil
	}
	var result []attachmentChange
	for _, ref := range a.Added {
		if ref == nil {
			continue
		}
		result = append(result, attachmentChange{
			Action: "added",
			ID:     api.DerefFlexString(ref.ID, ""),
			Name:   api.DerefString(ref.Display, ""),
		})
	}
	for _, ref := range a.Removed {
		if ref == nil {
			continue
		}
		result = append(result, attachmentChange{
			Action: "removed",
			ID:     api.DerefFlexString(ref.ID, ""),
			Name:   api.DerefString(ref.Display, ""),
		})
	}
	return result
}

func normalizeWorklog(wl []*tracker.ChangelogWorklog) []worklogChange {
	if len(wl) == 0 {
		return nil
	}
	result := make([]worklogChange, 0, len(wl))
	for _, w := range wl {
		if w == nil {
			continue
		}
		wc := worklogChange{
			From: normalizeWorklogValue(w.From),
			To:   normalizeWorklogValue(w.To),
		}
		if w.Record != nil {
			wc.Record = api.DerefFlexString(w.Record.ID, "")
			wc.RecordDisplay = api.DerefString(w.Record.Display, "")
		}
		result = append(result, wc)
	}
	return result
}

func normalizeWorklogValue(v *tracker.ChangelogWorklogValue) *worklogValue {
	if v == nil {
		return nil
	}
	wv := &worklogValue{
		Duration: formatDurationISO(v.Duration),
	}
	if v.Start != nil {
		wv.Start = v.Start.Format(time.RFC3339)
	}
	return wv
}

func normalizeRelatedResolutions(rr []*tracker.RelatedResolution) []resolutionChange {
	if len(rr) == 0 {
		return nil
	}
	result := make([]resolutionChange, 0, len(rr))
	for _, r := range rr {
		if r == nil {
			continue
		}
		rc := resolutionChange{
			Direction: api.DerefString(r.Direction, ""),
		}
		if r.Issue != nil {
			rc.Issue = api.DerefString(r.Issue.Key, "")
			rc.IssueDisplay = api.DerefString(r.Issue.Display, "")
		}
		if r.LinkType != nil {
			rc.LinkType = api.DerefFlexString(r.LinkType.ID, "")
			rc.LinkTypeName = linkTypeNameByDirection(r.LinkType, rc.Direction)
		}
		if r.NewResolution != nil {
			rc.Resolution = api.DerefString(r.NewResolution.Key, "")
			rc.ResolutionDisplay = api.DerefString(r.NewResolution.Display, "")
		}
		result = append(result, rc)
	}
	return result
}

func stripSelfURLs(v any) any {
	if v == nil {
		return nil
	}
	switch val := v.(type) {
	case map[string]any:
		result := make(map[string]any, len(val))
		for k, v2 := range val {
			if k == "self" {
				continue
			}
			result[k] = stripSelfURLs(v2)
		}
		return result
	case []any:
		result := make([]any, len(val))
		for i, v2 := range val {
			result[i] = stripSelfURLs(v2)
		}
		return result
	default:
		return v
	}
}

func formatDurationISO(d *tracker.Duration) string {
	if d == nil {
		return ""
	}
	return d.String()
}

// Prefers ID (machine-readable, matches --field filter values) over Display.
func changelogFieldName(f *tracker.FieldRef) string {
	if f == nil {
		return ""
	}
	if f.ID != nil {
		return string(*f.ID)
	}
	if f.Display != nil {
		return *f.Display
	}
	return ""
}

func normalizeChangeValue(v any) string {
	if v == nil {
		return ""
	}
	switch val := v.(type) {
	case string:
		return val
	case float64:
		if val == float64(int64(val)) {
			return strconv.FormatInt(int64(val), 10)
		}
		return fmt.Sprintf("%g", val)
	case map[string]any:
		if display, ok := val["display"]; ok {
			return normalizeChangeValue(display)
		}
		if key, ok := val["key"]; ok {
			return normalizeChangeValue(key)
		}
		if id, ok := val["id"]; ok {
			return normalizeChangeValue(id)
		}
		return fmt.Sprintf("%v", val)
	case []any:
		if len(val) == 0 {
			return ""
		}
		parts := make([]string, 0, len(val))
		for _, elem := range val {
			parts = append(parts, normalizeChangeValue(elem))
		}
		return strings.Join(parts, ", ")
	default:
		return fmt.Sprintf("%v", v)
	}
}
