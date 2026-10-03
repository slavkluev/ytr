// Package runner builds a command from its declaration and applies the policy
// every such command shares: auth, the --json field prelude, the JSON, jq,
// quiet and table output, and the mapping of Tracker errors.
package runner

import (
	"context"
	"fmt"
	"io"
	"reflect"
	"strings"

	"github.com/slavkluev/go-yandex-tracker/tracker"
	"github.com/spf13/cobra"

	"github.com/slavkluev/ytr/internal/api"
	"github.com/slavkluev/ytr/internal/config"
	"github.com/slavkluev/ytr/internal/output"
)

// A constructor cannot know the path its parent will give the command, so
// completion finds the fields on the command itself.
const fieldsAnnotation = "ytr:json-fields"

// List declares a command that fetches a list of T from Tracker and prints it.
// Item is the flat struct an element becomes under --json; its json tags are
// the fields the command accepts.
type List[T, Item any] struct {
	Use, Short, Long, Example string

	// Empty is printed in place of a table with no rows.
	Empty string

	Call   func(ctx context.Context, c *tracker.Client) ([]T, error)
	Item   func(T) Item
	Header []string
	Row    func(T) []string
	Quiet  func(T) string
}

// Command returns the cobra command l declares.
func (l List[T, Item]) Command() *cobra.Command {
	fields := jsonFields(reflect.TypeFor[Item]())

	return &cobra.Command{
		Use:         l.Use,
		Short:       l.Short,
		Long:        l.Long,
		Example:     l.Example,
		Args:        cobra.NoArgs,
		Annotations: map[string]string{fieldsAnnotation: strings.Join(fields, ",")},
		RunE: func(cmd *cobra.Command, _ []string) error {
			return l.run(cmd, fields)
		},
	}
}

// Fields returns the --json fields of a command this package built.
func Fields(cmd *cobra.Command) ([]string, bool) {
	joined, ok := cmd.Annotations[fieldsAnnotation]
	if !ok {
		return nil, false
	}

	return strings.Split(joined, ","), true
}

func (l List[T, Item]) run(cmd *cobra.Command, fields []string) error {
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

	values, err := l.Call(cmd.Context(), client)
	if err != nil {
		return api.MapAPIError(err)
	}

	return l.render(cmd.OutOrStdout(), opts, values)
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

// The prelude in run leaves every JSON mode with a field selection, so the
// JSON branch always filters.
func (l List[T, Item]) render(w io.Writer, opts *output.Options, values []T) error {
	if opts.IsJSON() {
		items := make([]map[string]any, len(values))
		for i, v := range values {
			items[i] = output.FilterFields(l.Item(v), opts.JSONFields)
		}

		if opts.JQFilter != "" {
			return output.ApplyJQ(w, items, opts.JQFilter)
		}
		return opts.PrintJSON(w, items)
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
		tbl.AddRow(cells(l.Row(v))...)
	}
	tbl.Render()

	return nil
}

func cells(values []string) []any {
	row := make([]any, len(values))
	for i, v := range values {
		row[i] = v
	}

	return row
}

// jsonFields reads the field names off item's json tags by the rule
// output.FilterFields applies, so every name it returns can be selected.
func jsonFields(item reflect.Type) []string {
	var fields []string
	for field := range item.Fields() {
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
