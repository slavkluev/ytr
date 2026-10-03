// Package runner builds a command from its declaration and applies the policy
// every such command shares: positional arguments, auth, the --json field
// prelude, the JSON, jq, quiet and table or card output, and the mapping of
// Tracker errors.
package runner

import (
	"context"
	"fmt"
	"io"
	"iter"
	"reflect"
	"strings"

	"github.com/slavkluev/go-yandex-tracker/tracker"
	"github.com/spf13/cobra"

	"github.com/slavkluev/ytr/internal/api"
	"github.com/slavkluev/ytr/internal/config"
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
	// Long is the description; Command adds the JSON FIELDS section after it,
	// then SeeAlso as the SEE ALSO section.
	Use, Short, Long, SeeAlso, Example string

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

	return newCommand(help{l.Use, l.Short, l.Long, l.SeeAlso, l.Example}, l.Args, fields,
		func(cmd *cobra.Command, args []string) error {
			return run(cmd, args, l.Args, fields, l.Call, l.render)
		})
}

func (l List[T, Item]) render(w io.Writer, opts *output.Options, values []T) error {
	if opts.IsJSON() {
		items := make([]map[string]any, len(values))
		for i, v := range values {
			items[i] = output.FilterFields(l.Item(v), opts.JSONFields)
		}

		return printJSON(w, opts, items)
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

// Arg is one positional argument: the check it must pass before anything else
// runs, and the value Call receives in its place.
type Arg struct {
	parse func(string) (string, error)
}

// IssueKey is an issue key such as PROJ-123.
var IssueKey = Arg{parse: func(arg string) (string, error) {
	return arg, validate.ValidateIssueKey(arg)
}}

// AnyArg takes the argument as given.
var AnyArg = Arg{parse: func(arg string) (string, error) { return arg, nil }}

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
	use, short, long, seeAlso, example string
}

func newCommand(h help, args []Arg, fields []string, runE func(*cobra.Command, []string) error) *cobra.Command {
	long := h.long + "\n\nJSON FIELDS\n  " + strings.Join(fields, ", ")
	if h.seeAlso != "" {
		long += "\n\nSEE ALSO\n" + h.seeAlso
	}

	// A leaf without arguments reports a stray one as an unknown command, which
	// cobra.ExactArgs(0) would turn into an argument count.
	accepts := cobra.NoArgs
	if len(args) > 0 {
		accepts = cobra.ExactArgs(len(args))
	}

	return &cobra.Command{
		Use:         h.use,
		Short:       h.short,
		Long:        long,
		Example:     h.example,
		Args:        accepts,
		Annotations: map[string]string{fieldsAnnotation: strings.Join(fields, ",")},
		RunE:        runE,
	}
}

func run[V any](
	cmd *cobra.Command,
	raw []string,
	declared []Arg,
	fields []string,
	call func(context.Context, *tracker.Client, []string) (V, error),
	render func(io.Writer, *output.Options, V) error,
) error {
	args := make([]string, len(raw))
	for i, arg := range declared {
		parsed, err := arg.parse(raw[i])
		if err != nil {
			return err
		}
		args[i] = parsed
	}

	opts := output.FromContext(cmd.Context())

	if opts.WantsFieldHint(cmd.Flags().Changed("json")) {
		name := strings.TrimPrefix(cmd.CommandPath(), cmd.Root().Name()+" ")
		return output.PrintFieldHint(cmd.ErrOrStderr(), name, fields)
	}

	if opts.JQFilter != "" && !opts.HasFieldSelection() {
		opts.JSONFields = fields
	}

	if opts.HasFieldSelection() {
		if err := output.ValidateFields(opts.JSONFields, fields); err != nil {
			return err
		}
		opts.JSONFields = output.NormalizeFields(opts.JSONFields, fields)
	}

	client, err := newClient(cmd)
	if err != nil {
		return err
	}

	value, err := call(cmd.Context(), client, args)
	if err != nil {
		return api.MapAPIError(err)
	}

	return render(cmd.OutOrStdout(), opts, value)
}

func newClient(cmd *cobra.Command) (*tracker.Client, error) {
	flags := cmd.Root().PersistentFlags()
	token, _ := flags.GetString("token")
	orgID, _ := flags.GetString("org-id")
	orgType, _ := flags.GetString("org-type")

	auth, err := config.ResolveAuth(token, orgID, orgType)
	if err != nil {
		return nil, err
	}

	return api.NewClient(auth), nil
}

// The prelude in run leaves every JSON mode with a field selection, so doc is
// always the filtered view.
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
