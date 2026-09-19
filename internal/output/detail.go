package output

import (
	"fmt"
	"io"

	"github.com/jedib0t/go-pretty/v6/text"
)

// DetailPrinter renders a resource as labeled key/value rows. On a TTY a row
// is a bold "Label:" followed by two spaces; off a TTY it is "Label\tvalue",
// so a reader can split the line on the single tab.
//
// The first write error is kept and every later write is skipped, so callers
// report it once from Err instead of checking each row.
type DetailPrinter struct {
	w   io.Writer
	err error
}

// NewDetail creates a DetailPrinter writing to w.
func NewDetail(w io.Writer) *DetailPrinter {
	return &DetailPrinter{w: w}
}

// Field writes one labeled row. Off a TTY the value is escaped like a table
// cell, so a tab or line break inside it cannot split the record in two and
// have the remainder read as another Label/value row.
func (d *DetailPrinter) Field(label, value string) {
	if d.err != nil {
		return
	}
	if IsTTY() {
		_, d.err = fmt.Fprintf(d.w, "%s  %s\n", d.bold(label+":"), value)
		return
	}
	_, d.err = fmt.Fprintf(d.w, "%s\t%s\n", label, cellEscaper.Replace(value))
}

// Block writes a free-form section: a blank separator line, the label on its
// own line, then the value indented by two spaces. Long prose such as a
// description keeps this shape in both modes, since it is already one block
// per line rather than a column.
func (d *DetailPrinter) Block(label, value string) {
	if d.err != nil {
		return
	}
	if _, err := fmt.Fprintln(d.w); err != nil {
		d.err = err
		return
	}
	if _, err := fmt.Fprintf(d.w, "%s\n", d.bold(label+":")); err != nil {
		d.err = err
		return
	}
	_, d.err = fmt.Fprintf(d.w, "  %s\n", value)
}

// Err returns the first write error, or nil when every write succeeded.
func (d *DetailPrinter) Err() error {
	return d.err
}

// bold emphasizes a label when colors are enabled, which off a TTY happens
// only if the caller forced them.
func (d *DetailPrinter) bold(label string) string {
	if ColorsEnabled() {
		return text.Colors{text.Bold}.Sprint(label)
	}
	return label
}
