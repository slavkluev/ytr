// Package runner builds a command from its declaration and applies the policy
// every such command shares: positional arguments, the request flags and
// --from-json of a write, the paging flags of a paged list, auth, the --json
// field prelude, the JSON, jq, quiet and table, card or confirm-line output,
// and the mapping of Tracker errors.
package runner

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"iter"
	"reflect"
	"strconv"
	"strings"
	"time"

	"github.com/slavkluev/go-yandex-tracker/tracker"
	"github.com/spf13/cobra"
	"github.com/spf13/pflag"

	"github.com/slavkluev/ytr/internal/api"
	"github.com/slavkluev/ytr/internal/config"
	"github.com/slavkluev/ytr/internal/errors"
	"github.com/slavkluev/ytr/internal/output"
	"github.com/slavkluev/ytr/internal/validate"
)

// A constructor cannot know the path its parent will give the command, so
// completion finds the fields on the command itself.
const fieldsAnnotation = "ytr:json-fields"

// List declares a command that fetches a list of T from Tracker and prints it.
// Item is the flat struct an element becomes under --json; its json tags are
// the fields the command accepts, in order.
type List[T, Item any] struct {
	// Long is the description; Command adds the JSON FIELDS section after it.
	Use, Short, Long, Example string

	Args []Arg

	// Empty is printed in place of a table with no rows.
	Empty string

	Call   func(ctx context.Context, c *tracker.Client, args []string) ([]T, error)
	Item   func(T) Item
	Header []string
	Row    func(*output.Options, T) []string
	Quiet  func(T) string
}

// Command returns the cobra command l declares.
func (l List[T, Item]) Command() *cobra.Command {
	fields := ItemFields[Item]()

	return newCommand(help{l.Use, l.Short, l.Long, l.Example}, l.Args, fields,
		func(cmd *cobra.Command, args []string) error {
			return run(cmd, args, steps[[]T]{args: l.Args, fields: fields, call: l.Call, render: l.render})
		})
}

func (l List[T, Item]) render(w io.Writer, opts *output.Options, _ []string, values []T) error {
	if opts.IsJSON() {
		return printJSON(w, opts, cut(opts, l.items(values)))
	}

	// Before the empty check, so --quiet on an empty list prints nothing.
	if opts.Quiet {
		keys := make([]string, len(values))
		for i, v := range values {
			keys[i] = l.Quiet(v)
		}
		output.PrintQuiet(w, keys...)
		return nil
	}

	if len(values) == 0 {
		_, err := fmt.Fprintln(w, l.Empty)
		return err
	}

	tbl := opts.NewTable(w)
	tbl.AddHeader(cells(l.Header)...)
	for _, v := range values {
		tbl.AddRow(cells(l.Row(opts, v))...)
	}
	tbl.Render()

	return nil
}

func (l List[T, Item]) items(values []T) []Item {
	items := make([]Item, len(values))
	for i, v := range values {
		items[i] = l.Item(v)
	}

	return items
}

const defaultPageLimit = 50

// Pages declares a command that prints one page of a page-numbered Tracker
// list, chosen with --limit and --cursor, or every page with --all. Under
// --json the items come in the {items, pagination} envelope. Item is the flat
// struct an element becomes under --json; its json tags are the fields the
// command accepts, in order.
type Pages[T, Item any] struct {
	// Long is the description; Command adds the JSON FIELDS section after it.
	Use, Short, Long, Example string

	// Check, when set, refuses flags that cannot go together or values that
	// do not parse, ahead of the field hint and auth. It may keep what it
	// parses for Page and All, which run only after it succeeds.
	Check func(*pflag.FlagSet) error

	// Empty is printed in place of a table with no rows.
	Empty string

	// Page fetches the page o names, and its Response gives the total. All is
	// the library's iterator from the page o names on; it drops each Response,
	// so --all can only count the items it yields.
	Page func(ctx context.Context, c *tracker.Client, o tracker.ListOptions) ([]T, *tracker.Response, error)
	All  func(ctx context.Context, c *tracker.Client, o tracker.ListOptions) iter.Seq2[T, error]

	Item   func(T) Item
	Header []string
	Row    func(*output.Options, T) []string
	Quiet  func(T) string
}

// listPage is what a run of a Pages command fetched: the items and the
// envelope's pagination.
type listPage[T any] struct {
	values []T
	meta   output.PaginationMeta
}

// Command returns the cobra command p declares.
func (p Pages[T, Item]) Command() *cobra.Command {
	fields := ItemFields[Item]()

	cmd := newCommand(help{p.Use, p.Short, p.Long, p.Example}, nil, fields,
		func(cmd *cobra.Command, args []string) error {
			set := cmd.Flags()
			var o tracker.ListOptions

			return run(cmd, args, steps[listPage[T]]{
				fields: fields,
				check: func() error {
					if p.Check != nil {
						if err := p.Check(set); err != nil {
							return err
						}
					}
					if err := validate.ConflictingAllAndCursor(set.Changed("all"), set.Changed("cursor")); err != nil {
						return err
					}

					o.PerPage, _ = set.GetInt("limit")
					if err := validate.ValidatePageLimit(o.PerPage); err != nil {
						return err
					}

					cursor, _ := set.GetString("cursor")
					var err error
					o.Page, err = validate.ParsePageCursor(cursor)
					return err
				},
				call: func(ctx context.Context, c *tracker.Client, _ []string) (listPage[T], error) {
					if all, _ := set.GetBool("all"); all {
						return p.fetchAll(ctx, c, o)
					}
					return p.fetchPage(ctx, c, o)
				},
				render: p.render,
			})
		})

	cmd.Flags().Int("limit", defaultPageLimit, "Maximum number of results per page (max 1000)")
	cmd.Flags().String("cursor", "", "Page number for pagination")
	cmd.Flags().Bool("all", false, "Fetch all pages automatically")

	return cmd
}

func (p Pages[T, Item]) fetchPage(ctx context.Context, c *tracker.Client, o tracker.ListOptions) (listPage[T], error) {
	values, resp, err := p.Page(ctx, c, o)
	if err != nil {
		return listPage[T]{}, err
	}

	meta := output.PaginationMeta{HasMore: len(values) == o.PerPage}
	if meta.HasMore {
		meta.Cursor = strconv.Itoa(o.Page + 1)
	}
	if resp != nil {
		meta.Total = resp.TotalCount
	}

	return listPage[T]{values: values, meta: meta}, nil
}

// fetchAll starts at o, which --cursor cannot move under --all, so at page 1.
func (p Pages[T, Item]) fetchAll(ctx context.Context, c *tracker.Client, o tracker.ListOptions) (listPage[T], error) {
	values, err := Collect(p.All(ctx, c, o))
	if err != nil {
		return listPage[T]{}, err
	}

	return listPage[T]{values: values, meta: output.PaginationMeta{Total: len(values)}}, nil
}

func (p Pages[T, Item]) render(w io.Writer, opts *output.Options, args []string, page listPage[T]) error {
	list := List[T, Item]{Empty: p.Empty, Item: p.Item, Header: p.Header, Row: p.Row, Quiet: p.Quiet}
	if !opts.IsJSON() {
		return list.render(w, opts, args, page.values)
	}

	return PrintPage(w, opts, list.items(page.values), page.meta)
}

// PrintPage prints the JSON of a page: items in the {items, pagination}
// envelope, each cut to the --json fields SelectFields left in opts.
func PrintPage[Item any](w io.Writer, opts *output.Options, items []Item, meta output.PaginationMeta) error {
	return printJSON(w, opts, output.PaginatedResult{Items: cut(opts, items), Pagination: meta})
}

func cut[Item any](opts *output.Options, items []Item) []map[string]any {
	filtered := make([]map[string]any, len(items))
	for i, item := range items {
		filtered[i] = output.FilterFields(item, opts.JSONFields)
	}

	return filtered
}

// Get declares a command that fetches one T from Tracker and prints it as a
// card of labeled rows. Item is the flat struct T becomes under --json; its
// json tags are the fields the command accepts, in order.
type Get[T, Item any] struct {
	// Long is the description; Command adds the JSON FIELDS section after it.
	Use, Short, Long, Example string

	Args []Arg

	Call   func(ctx context.Context, c *tracker.Client, args []string) (T, error)
	Item   func(T) Item
	Detail func(*output.DetailPrinter, *output.Options, T)
	Quiet  func(T) string
}

// Command returns the cobra command g declares.
func (g Get[T, Item]) Command() *cobra.Command {
	fields := ItemFields[Item]()

	return newCommand(help{g.Use, g.Short, g.Long, g.Example}, g.Args, fields,
		func(cmd *cobra.Command, args []string) error {
			return run(cmd, args, steps[T]{args: g.Args, fields: fields, call: g.Call, render: g.render})
		})
}

func (g Get[T, Item]) render(w io.Writer, opts *output.Options, _ []string, value T) error {
	if opts.IsJSON() {
		return printJSON(w, opts, output.FilterFields(g.Item(value), opts.JSONFields))
	}

	if opts.Quiet {
		output.PrintQuiet(w, g.Quiet(value))
		return nil
	}

	card := opts.NewDetail(w)
	g.Detail(card, opts, value)

	return card.Err()
}

// Write declares a command that sends Tracker a Req, built from its request
// flags or given whole by --from-json, and prints the T Tracker answers with.
// Item is the flat struct T becomes under --json; its json tags are the fields
// the command accepts, in order.
type Write[Req, T, Item any] struct {
	// Long is the description; Command adds the JSON FIELDS section after it.
	Use, Short, Long, Example string

	Args []Arg

	// Flags are in the order errors name them.
	Flags []Flag

	// FromJSON is the help of --from-json; without it the command takes no
	// --from-json.
	FromJSON string

	// Required are the body keys a create cannot go without, whichever way the
	// body comes. Update instead makes a body that sets no key an error.
	Required []string
	Update   bool

	Call    func(ctx context.Context, c *tracker.Client, args []string, req *Req) (T, error)
	Item    func(T) Item
	Quiet   func(T) string
	Confirm func(args []string, value T) string
}

// Command returns the cobra command w declares.
func (w Write[Req, T, Item]) Command() *cobra.Command {
	fields := ItemFields[Item]()
	body := validate.Body{Required: w.Required, FromJSON: w.FromJSON != "", Update: w.Update}
	for _, f := range w.Flags {
		body.Flags = append(body.Flags, validate.BodyFlag{Name: f.name, Key: f.key})
	}

	cmd := newCommand(help{w.Use, w.Short, w.Long, w.Example}, w.Args, fields,
		func(cmd *cobra.Command, args []string) error {
			var (
				patch map[string]any
				req   Req
			)

			return run(cmd, args, steps[T]{
				args: w.Args, fields: fields,
				check: func() error {
					var err error
					patch, err = flagBody(cmd.Flags(), body, w.Flags)
					return err
				},
				prepare: func() error {
					data, err := requestBody(cmd, patch)
					if err != nil {
						return err
					}
					return body.Decode(data, &req)
				},
				call: func(ctx context.Context, c *tracker.Client, args []string) (T, error) {
					return w.Call(ctx, c, args, &req)
				},
				render: w.render,
			})
		})

	for _, f := range w.Flags {
		f.define(cmd.Flags())
	}
	if w.FromJSON != "" {
		cmd.Flags().String(validate.FromJSONFlag, "", w.FromJSON)
	}

	return cmd
}

func (w Write[Req, T, Item]) render(out io.Writer, opts *output.Options, args []string, value T) error {
	if opts.IsJSON() {
		return printJSON(out, opts, output.FilterFields(w.Item(value), opts.JSONFields))
	}

	if opts.Quiet {
		output.PrintQuiet(out, w.Quiet(value))
		return nil
	}

	_, err := fmt.Fprintln(out, w.Confirm(args, value))
	return err
}

// Delete declares a command that deletes what its last argument names and
// prints that ID: as a deleted item under --json, alone under --quiet, and
// otherwise in the Confirm line.
type Delete struct {
	// Long is the description; Command adds the JSON FIELDS section after it.
	Use, Short, Long, Example string

	Args []Arg

	Call    func(ctx context.Context, c *tracker.Client, args []string) error
	Confirm func(id string) string
}

// Command returns the cobra command d declares.
func (d Delete) Command() *cobra.Command {
	fields := ItemFields[deleted]()

	return newCommand(help{d.Use, d.Short, d.Long, d.Example}, d.Args, fields,
		func(cmd *cobra.Command, args []string) error {
			return run(cmd, args, steps[struct{}]{
				args: d.Args, fields: fields,
				call: func(ctx context.Context, c *tracker.Client, args []string) (struct{}, error) {
					return struct{}{}, d.Call(ctx, c, args)
				},
				render: d.render,
			})
		})
}

func (d Delete) render(w io.Writer, opts *output.Options, args []string, _ struct{}) error {
	id := args[len(args)-1]

	if opts.IsJSON() {
		return printJSON(w, opts, output.FilterFields(deleted{ID: id, Deleted: true}, opts.JSONFields))
	}

	if opts.Quiet {
		output.PrintQuiet(w, id)
		return nil
	}

	_, err := fmt.Fprintln(w, d.Confirm(id))
	return err
}

type deleted struct {
	ID      string `json:"id"`
	Deleted bool   `json:"deleted"`
}

// Flag is one request flag of a Write: the body key it sets and how its text
// becomes that key's value.
type Flag struct {
	name, key, usage string
	boolean          bool
	parse            func(string) (any, error)
}

// Text is a flag whose value is its text as given.
func Text(name, usage string) Flag {
	return Flag{name: name, key: name, usage: usage, parse: func(s string) (any, error) { return s, nil }}
}

// Bool is a flag set to true by its name alone, or to false as --name=false.
func Bool(name, usage string) Flag {
	return Flag{name: name, key: name, usage: usage, boolean: true, parse: func(s string) (any, error) {
		return strconv.ParseBool(s)
	}}
}

// Duration is an ISO 8601 duration such as PT1H30M.
func Duration(name, usage string) Flag {
	return Flag{name: name, key: name, usage: usage, parse: func(s string) (any, error) {
		var d tracker.Duration
		if err := json.Unmarshal([]byte(`"`+s+`"`), &d); err != nil {
			return nil, errors.NewUserError(
				fmt.Sprintf("invalid ISO 8601 duration %q", s),
				"Use ISO 8601 format: PT1H30M (1h30m), PT45M (45min), P1D (1 day), P1DT2H (1 day 2 hours)",
			)
		}

		return d, nil
	}}
}

// Time is an RFC 3339 time such as 2026-03-30T10:00:00Z.
func Time(name, usage string) Flag {
	return Flag{name: name, key: name, usage: usage, parse: func(s string) (any, error) {
		t, err := time.Parse(time.RFC3339, s)
		if err != nil {
			return nil, errors.NewUserError(
				fmt.Sprintf("invalid timestamp %q", s),
				"Use RFC 3339 format: 2026-03-30T10:00:00Z",
			)
		}

		return tracker.Timestamp{Time: t}, nil
	}}
}

// Key makes f set key, where by default it sets the key named as the flag.
func (f Flag) Key(key string) Flag {
	f.key = key
	return f
}

// Check makes f refuse a value check rejects, with check's error.
func (f Flag) Check(check func(string) error) Flag {
	parse := f.parse
	f.parse = func(s string) (any, error) {
		if err := check(s); err != nil {
			return nil, err
		}
		return parse(s)
	}

	return f
}

func (f Flag) define(flags *pflag.FlagSet) {
	if f.boolean {
		flags.Bool(f.name, false, f.usage)
		return
	}

	flags.String(f.name, "", f.usage)
}

// flagBody checks the request flags a run set and returns the body they give,
// one key per flag, before the field hint and auth. It is empty when
// --from-json gives the body.
func flagBody(set *pflag.FlagSet, body validate.Body, flags []Flag) (map[string]any, error) {
	if err := body.CheckFlags(set.Changed); err != nil {
		return nil, err
	}

	patch := make(map[string]any)
	for _, f := range flags {
		if !set.Changed(f.name) {
			continue
		}

		value, err := f.parse(set.Lookup(f.name).Value.String())
		if err != nil {
			return nil, err
		}
		patch[f.key] = value
	}

	return patch, nil
}

// requestBody returns the body --from-json gives, read only once auth has
// resolved, or else the one the flags gave, so both reach the same decoder.
func requestBody(cmd *cobra.Command, patch map[string]any) ([]byte, error) {
	if set := cmd.Flags(); set.Changed(validate.FromJSONFlag) {
		value, _ := set.GetString(validate.FromJSONFlag)
		return validate.ParseJSONInputFrom(value, cmd.InOrStdin())
	}

	return json.Marshal(patch)
}

// Arg is one positional argument: the check it must pass before anything else
// runs, and the value Call receives in its place.
type Arg struct {
	parse func(string) (string, error)
}

// IssueKey is an issue key such as PROJ-123.
var IssueKey = Arg{parse: func(arg string) (string, error) {
	return arg, validate.ValidateIssueKey(arg)
}}

// StringID is a non-empty ID, which Call receives with its surrounding spaces
// trimmed. label names it in the error.
func StringID(label string) Arg {
	return Arg{parse: func(arg string) (string, error) {
		return validate.ValidateStringID(arg, label)
	}}
}

// NumericID is a positive integer ID, which Call receives as given. label
// names it in the error.
func NumericID(label string) Arg {
	return Arg{parse: func(arg string) (string, error) {
		_, err := validate.ValidateNumericID(arg, label)
		return arg, err
	}}
}

// Fields returns the --json fields of a command, as its constructor recorded
// them with Command or SetFields.
func Fields(cmd *cobra.Command) ([]string, bool) {
	joined, ok := cmd.Annotations[fieldsAnnotation]
	if !ok {
		return nil, false
	}

	return strings.Split(joined, ","), true
}

// SetFields records fields as the --json fields of cmd, a command Command did
// not build, so completion and Fields find them as they find any other's.
func SetFields(cmd *cobra.Command, fields []string) {
	if cmd.Annotations == nil {
		cmd.Annotations = make(map[string]string, 1)
	}

	cmd.Annotations[fieldsAnnotation] = strings.Join(fields, ",")
}

// ItemFields returns the --json fields of commands that print Item: the names
// of its json tags, by the rule output.FilterFields applies, in order.
func ItemFields[Item any]() []string {
	var fields []string
	for field := range reflect.TypeFor[Item]().Fields() {
		tag := field.Tag.Get("json")
		if tag == "" || tag == "-" {
			continue
		}

		name, _, _ := strings.Cut(tag, ",")
		if name == "" {
			name = field.Name
		}
		fields = append(fields, name)
	}

	return fields
}

// Collect drains seq, the iterator a library List method pages through, so a
// Call returns every page instead of the first. It returns nothing but the
// error once seq yields one.
func Collect[T any](seq iter.Seq2[T, error]) ([]T, error) {
	var values []T
	for v, err := range seq {
		if err != nil {
			return nil, err
		}
		values = append(values, v)
	}

	return values, nil
}

type help struct {
	use, short, long, example string
}

func newCommand(h help, args []Arg, fields []string, runE func(*cobra.Command, []string) error) *cobra.Command {
	long := h.long + "\n\nJSON FIELDS\n  " + strings.Join(fields, ", ")

	// A leaf without arguments reports a stray one as an unknown command, which
	// cobra.ExactArgs(0) would turn into an argument count.
	accepts := cobra.NoArgs
	if len(args) > 0 {
		accepts = cobra.ExactArgs(len(args))
	}

	cmd := &cobra.Command{
		Use:     h.use,
		Short:   h.short,
		Long:    long,
		Example: h.example,
		Args:    accepts,
		RunE:    runE,
	}
	SetFields(cmd, fields)

	return cmd
}

// steps are what one command runs inside the policy run applies.
type steps[V any] struct {
	args   []Arg
	fields []string

	// check runs once the arguments pass, ahead of the field hint and auth.
	check func() error

	// prepare runs once auth resolves, ahead of call. Its error is the
	// command's own, not Tracker's, so it is not mapped.
	prepare func() error

	call   func(context.Context, *tracker.Client, []string) (V, error)
	render func(w io.Writer, opts *output.Options, args []string, value V) error
}

func run[V any](cmd *cobra.Command, raw []string, s steps[V]) error {
	args := make([]string, len(raw))
	for i, arg := range s.args {
		parsed, err := arg.parse(raw[i])
		if err != nil {
			return err
		}
		args[i] = parsed
	}

	if s.check != nil {
		if err := s.check(); err != nil {
			return err
		}
	}

	opts, err := SelectFields(cmd, s.fields)
	if err != nil {
		return err
	}

	client, err := Client(cmd)
	if err != nil {
		return err
	}

	if s.prepare != nil {
		if err = s.prepare(); err != nil {
			return err
		}
	}

	value, err := s.call(cmd.Context(), client, args)
	if err != nil {
		return api.MapAPIError(err)
	}

	return s.render(cmd.OutOrStdout(), opts, args, value)
}

// SelectFields returns the output options of a run of cmd, a command whose
// --json fields are fields, left with the fields the run selected: every one
// under a bare --jq. It answers a bare --json= with the field hint instead.
func SelectFields(cmd *cobra.Command, fields []string) (*output.Options, error) {
	opts := output.FromContext(cmd.Context())

	if opts.WantsFieldHint(cmd.Flags().Changed("json")) {
		name := strings.TrimPrefix(cmd.CommandPath(), cmd.Root().Name()+" ")
		return nil, output.PrintFieldHint(cmd.ErrOrStderr(), name, fields)
	}

	if opts.JQFilter != "" && !opts.HasFieldSelection() {
		opts.JSONFields = fields
	}

	if opts.HasFieldSelection() {
		if err := output.ValidateFields(opts.JSONFields, fields); err != nil {
			return nil, err
		}
		opts.JSONFields = output.NormalizeFields(opts.JSONFields, fields)
	}

	return opts, nil
}

// Client returns the Tracker client a run of cmd sends its requests with,
// signed in by what config.ResolveAuth makes of the root's auth flags.
func Client(cmd *cobra.Command) (*tracker.Client, error) {
	flags := cmd.Root().PersistentFlags()
	token, _ := flags.GetString("token")
	orgID, _ := flags.GetString("org-id")
	orgType, _ := flags.GetString("org-type")

	auth, err := config.ResolveAuth(cmd.Context(), token, orgID, orgType)
	if err != nil {
		return nil, err
	}

	return api.NewClient(auth), nil
}

// SelectFields leaves every JSON mode with a field selection, so doc is always
// the filtered view.
func printJSON(w io.Writer, opts *output.Options, doc any) error {
	if opts.JQFilter != "" {
		return output.ApplyJQ(w, doc, opts.JQFilter)
	}

	return opts.PrintJSON(w, doc)
}

func cells(values []string) []any {
	row := make([]any, len(values))
	for i, v := range values {
		row[i] = v
	}

	return row
}
