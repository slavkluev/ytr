package issue

import (
	"context"
	"fmt"
	"io"
	"strconv"
	"strings"
	"time"

	"github.com/slavkluev/go-yandex-tracker/tracker"
	"github.com/spf13/cobra"

	"github.com/slavkluev/ytr/internal/api"
	"github.com/slavkluev/ytr/internal/cmd/runner"
	"github.com/slavkluev/ytr/internal/config"
	"github.com/slavkluev/ytr/internal/output"
	"github.com/slavkluev/ytr/internal/validate"
)

// IssueChangelogFields lists the available JSON field names for changelog output.
var IssueChangelogFields = []string{
	"date", "author", "authorId", "type", "transport",
	"fields", "comments", "links", "attachments", "worklog", "relatedResolutions",
}

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

type changelogItem struct {
	Date   string `json:"date"`
	Author string `json:"author"`
	Field  string `json:"field"`
	From   string `json:"from"`
	To     string `json:"to"`
}

func newChangelogCmd() *cobra.Command {
	var (
		fieldFilter string
		typeFilter  string
		limit       int
		cursor      string
		all         bool
	)

	cmd := &cobra.Command{
		Use:   "changelog ISSUE-KEY",
		Short: "Show issue change history",
		Long: `Display the change history of a Yandex Tracker issue.

Each changelog entry represents an atomic event (field change, comment, link, etc.)
with the date, author, type, and structured details.

JSON FIELDS
  date, author, authorId, type, transport, fields, comments, links, attachments, worklog, relatedResolutions

SEE ALSO
  ytr issue view        - View issue details
  ytr issue list        - List issues
  ytr issue transition  - Transition issue status`,
		Example: `  # Show all changes for an issue
  ytr issue changelog PROJ-123

  # Filter to only status transitions
  ytr issue changelog PROJ-123 --field status

  # Get as JSON with all fields
  ytr issue changelog PROJ-123 --json date,author,type,fields,comments,links

  # Extract status transitions using jq
  ytr issue changelog PROJ-123 --json date,type,fields --jq '.items[] | select(.type=="IssueWorkflow")'

  # Fetch all pages automatically
  ytr issue changelog PROJ-123 --all`,
		Args: cobra.ExactArgs(1),
		PreRunE: func(cmd *cobra.Command, args []string) error {
			return validate.ValidateIssueKey(args[0])
		},
		RunE: func(cmd *cobra.Command, args []string) error {
			return runChangelog(cmd, args, fieldFilter, typeFilter, limit, cursor, all)
		},
	}

	cmd.Flags().StringVar(&fieldFilter, "field", "", "Filter changes by field ID (case-sensitive, e.g. storyPoints)")
	cmd.Flags().StringVar(&typeFilter, "type", "", "Filter by change type (e.g., IssueWorkflow, IssueCommentAdded)")
	cmd.Flags().IntVar(&limit, "limit", defaultLimit, "Maximum number of changelog entries per page")
	cmd.Flags().StringVar(&cursor, "cursor", "", "Cursor ID for pagination (from previous response)")
	cmd.Flags().BoolVar(&all, "all", false, "Fetch all pages automatically")

	runner.SetFields(cmd, IssueChangelogFields)

	return cmd
}

func runChangelog(
	cmd *cobra.Command,
	args []string,
	fieldFilter, typeFilter string,
	limit int,
	cursor string,
	all bool,
) error {
	opts := output.FromContext(cmd.Context())

	if opts.WantsFieldHint(cmd.Flags().Changed("json")) {
		return output.PrintFieldHint(cmd.ErrOrStderr(), "issue changelog", IssueChangelogFields)
	}

	if err := validate.ConflictingAllAndCursor(
		cmd.Flags().Changed("all"), cmd.Flags().Changed("cursor"),
	); err != nil {
		return err
	}

	if opts.JQFilter != "" && !opts.HasFieldSelection() {
		opts.JSONFields = IssueChangelogFields
	}

	if opts.HasFieldSelection() {
		if err := output.ValidateFields(opts.JSONFields, IssueChangelogFields); err != nil {
			return err
		}
		opts.JSONFields = output.NormalizeFields(opts.JSONFields, IssueChangelogFields)
	}

	issueKey := args[0]

	tokenFlag, _ := cmd.Root().PersistentFlags().GetString("token")
	orgIDFlag, _ := cmd.Root().PersistentFlags().GetString("org-id")
	orgTypeFlag, _ := cmd.Root().PersistentFlags().GetString("org-type")

	auth, err := config.ResolveAuth(tokenFlag, orgIDFlag, orgTypeFlag)
	if err != nil {
		return err
	}

	getter := newChangelogGetter(auth)

	if limit < 1 {
		limit = defaultLimit
	}
	if limit > maxLimit {
		limit = maxLimit
	}

	entries, hasMore, nextCursor, err := fetchChangelogPage(
		cmd.Context(), getter, issueKey, limit, cursor, all, fieldFilter, typeFilter,
	)
	if err != nil {
		return err
	}

	if opts.IsJSON() {
		normalized := normalizeChangelog(entries)
		return renderChangelogJSON(cmd.OutOrStdout(), opts, normalized, hasMore, nextCursor)
	}

	items := flattenChangelog(entries)
	return renderChangelogNonJSON(cmd.OutOrStdout(), opts, items)
}

func fetchChangelogPage(
	ctx context.Context,
	getter changelogGetter,
	issueKey string,
	limit int,
	cursor string,
	all bool,
	fieldFilter string,
	typeFilter string,
) ([]*tracker.Changelog, bool, string, error) {
	if all {
		entries, err := fetchAllChangelog(ctx, getter, issueKey, limit, fieldFilter, typeFilter)
		return entries, false, "", err
	}

	opts := &tracker.ChangelogOptions{
		ID:      cursor,
		PerPage: limit,
		Field:   fieldFilter,
		Type:    typeFilter,
	}
	entries, _, err := getter.GetChangelog(ctx, issueKey, opts)
	if err != nil {
		return nil, false, "", api.MapAPIError(err)
	}

	hasMore := len(entries) == limit
	var next string
	if hasMore {
		next = lastChangelogCursorID(entries)
	}
	return entries, hasMore, next, nil
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

func renderChangelogJSON(
	w io.Writer,
	opts *output.Options,
	entries []changelogEntry,
	hasMore bool,
	nextCursor string,
) error {
	var data any
	if opts.HasFieldSelection() {
		filtered := make([]map[string]any, 0, len(entries))
		for _, entry := range entries {
			filtered = append(filtered, output.FilterFields(entry, opts.JSONFields))
		}
		data = output.PaginatedResult{
			Items: filtered,
			Pagination: output.PaginationMeta{
				Cursor:  nextCursor,
				HasMore: hasMore,
			},
		}
	} else {
		data = output.PaginatedResult{
			Items: entries,
			Pagination: output.PaginationMeta{
				Cursor:  nextCursor,
				HasMore: hasMore,
			},
		}
	}

	if opts.JQFilter != "" {
		return output.ApplyJQ(w, data, opts.JQFilter)
	}
	return opts.PrintJSON(w, data)
}

func flattenChangelog(entries []*tracker.Changelog) []changelogItem {
	var items []changelogItem
	for _, entry := range entries {
		if entry == nil {
			continue
		}
		var date string
		if entry.UpdatedAt != nil {
			date = entry.UpdatedAt.Format(time.RFC3339)
		}
		author := entry.UpdatedBy.DisplayOr("")

		for _, event := range entry.Fields {
			if event == nil {
				continue
			}
			items = append(items, changelogItem{
				Date: date, Author: author,
				Field: changelogFieldName(event.Field),
				From:  normalizeChangeValue(event.From),
				To:    normalizeChangeValue(event.To),
			})
		}
		items = flattenCommentsToItems(items, date, author, entry.Comments)
		items = flattenLinksToItems(items, date, author, entry.Links)
		items = flattenAttachmentsToItems(items, date, author, entry.Attachments)
		items = flattenWorklogToItems(items, date, author, entry.Worklog)
		items = flattenResolutionsToItems(items, date, author, entry.RelatedResolutions)
	}
	return items
}

func flattenCommentsToItems(
	items []changelogItem, date, author string, c *tracker.ChangelogComments,
) []changelogItem {
	if c == nil {
		return items
	}
	for _, ref := range c.Added {
		if ref == nil {
			continue
		}
		items = append(items, changelogItem{
			Date: date, Author: author,
			Field: "comment",
			To:    api.DerefString(ref.Display, ""),
		})
	}
	for _, ref := range c.Removed {
		if ref == nil {
			continue
		}
		items = append(items, changelogItem{
			Date: date, Author: author,
			Field: "comment",
			From:  api.DerefString(ref.Display, ""),
		})
	}
	for _, cu := range c.Updated {
		if cu == nil {
			continue
		}
		switch {
		case cu.AddedReaction != nil:
			items = append(items, changelogItem{
				Date: date, Author: author,
				Field: "reaction",
				To:    *cu.AddedReaction,
			})
		case cu.RemovedReaction != nil:
			items = append(items, changelogItem{
				Date: date, Author: author,
				Field: "reaction",
				From:  *cu.RemovedReaction,
			})
		default:
			items = append(items, changelogItem{
				Date: date, Author: author,
				Field: "comment",
				From:  normalizeChangeValue(cu.From),
				To:    normalizeChangeValue(cu.To),
			})
		}
	}
	return items
}

func flattenLinksToItems(
	items []changelogItem, date, author string, links []*tracker.ChangelogLink,
) []changelogItem {
	for _, l := range links {
		if l == nil {
			continue
		}
		items = append(items, changelogItem{
			Date: date, Author: author,
			Field: "link",
			From:  formatLinkValueString(l.From),
			To:    formatLinkValueString(l.To),
		})
	}
	return items
}

func flattenAttachmentsToItems(
	items []changelogItem, date, author string, a *tracker.ChangelogAttachments,
) []changelogItem {
	if a == nil {
		return items
	}
	for _, ref := range a.Added {
		if ref == nil {
			continue
		}
		items = append(items, changelogItem{
			Date: date, Author: author,
			Field: "attachment",
			To:    api.DerefString(ref.Display, ""),
		})
	}
	for _, ref := range a.Removed {
		if ref == nil {
			continue
		}
		items = append(items, changelogItem{
			Date: date, Author: author,
			Field: "attachment",
			From:  api.DerefString(ref.Display, ""),
		})
	}
	return items
}

func flattenWorklogToItems(
	items []changelogItem, date, author string, wl []*tracker.ChangelogWorklog,
) []changelogItem {
	for _, w := range wl {
		if w == nil {
			continue
		}
		items = append(items, changelogItem{
			Date: date, Author: author,
			Field: "worklog",
			From:  formatWorklogValueString(w.From),
			To:    formatWorklogValueString(w.To),
		})
	}
	return items
}

func flattenResolutionsToItems(
	items []changelogItem, date, author string, rr []*tracker.RelatedResolution,
) []changelogItem {
	for _, r := range rr {
		if r == nil {
			continue
		}
		items = append(items, changelogItem{
			Date: date, Author: author,
			Field: "relatedResolution",
			To:    formatRelatedResolutionString(r),
		})
	}
	return items
}

func formatLinkValueString(v *tracker.ChangelogLinkValue) string {
	if v == nil {
		return ""
	}
	var linkName string
	if v.Type != nil {
		linkName = linkTypeNameByDirection(v.Type, api.DerefString(v.Direction, ""))
		if linkName == "" {
			linkName = api.DerefFlexString(v.Type.ID, "")
		}
	}
	issueKey := ""
	if v.Object != nil {
		issueKey = api.DerefString(v.Object.Key, "")
	}
	if linkName != "" && issueKey != "" {
		return linkName + " → " + issueKey
	}
	if issueKey != "" {
		return issueKey
	}
	return linkName
}

func formatWorklogValueString(v *tracker.ChangelogWorklogValue) string {
	if v == nil {
		return ""
	}
	return formatDurationISO(v.Duration)
}

func formatRelatedResolutionString(rr *tracker.RelatedResolution) string {
	if rr == nil {
		return ""
	}
	issueKey := ""
	if rr.Issue != nil {
		issueKey = api.DerefString(rr.Issue.Key, "")
	}
	resolution := ""
	if rr.NewResolution != nil {
		resolution = api.DerefString(rr.NewResolution.Display, api.DerefString(rr.NewResolution.Key, ""))
	}
	if issueKey != "" && resolution != "" {
		return issueKey + ": " + resolution
	}
	if issueKey != "" {
		return issueKey
	}
	return resolution
}

func renderChangelogNonJSON(w io.Writer, opts *output.Options, items []changelogItem) error {
	if opts.Quiet {
		for _, item := range items {
			output.PrintQuiet(w, fmt.Sprintf("%s: %s -> %s", item.Field, item.From, item.To))
		}
		return nil
	}

	return renderChangelogTable(w, opts, items)
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

func fetchAllChangelog(
	ctx context.Context,
	getter changelogGetter,
	issueKey string,
	limit int,
	fieldFilter string,
	typeFilter string,
) ([]*tracker.Changelog, error) {
	var all []*tracker.Changelog
	currentCursor := ""

	for {
		opts := &tracker.ChangelogOptions{
			ID:      currentCursor,
			PerPage: limit,
			Field:   fieldFilter,
			Type:    typeFilter,
		}
		entries, _, err := getter.GetChangelog(ctx, issueKey, opts)
		if err != nil {
			return nil, api.MapAPIError(err)
		}

		if len(entries) == 0 {
			break
		}

		all = append(all, entries...)

		if len(entries) < limit {
			break
		}

		lastID := lastChangelogCursorID(entries)
		if lastID == "" {
			break
		}
		currentCursor = lastID
	}

	return all, nil
}

func renderChangelogTable(w io.Writer, opts *output.Options, items []changelogItem) error {
	if len(items) == 0 {
		_, err := fmt.Fprintln(w, "No changes found")
		return err
	}

	tbl := opts.NewTable(w)
	tbl.AddHeader("DATE", "AUTHOR", "FIELD", "FROM", "TO")

	for _, item := range items {
		tbl.AddRow(item.Date, item.Author, item.Field, item.From, item.To)
	}

	tbl.Render()
	return nil
}
