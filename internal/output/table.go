package output

import (
	"fmt"
	"io"
	"strings"

	"github.com/jedib0t/go-pretty/v6/table"
	"github.com/jedib0t/go-pretty/v6/text"
)

// StyleGH is a gh-CLI-inspired borderless table style.
// Produces compact output with space-aligned columns and no borders.
var StyleGH = table.Style{
	Name: "ytr-gh",
	Box: table.BoxStyle{
		PaddingLeft:  "",
		PaddingRight: "  ",
	},
	Options: table.Options{
		DrawBorder:      false,
		SeparateColumns: false,
		SeparateFooter:  false,
		SeparateHeader:  false,
		SeparateRows:    false,
	},
	Format: table.FormatOptions{
		Header: text.FormatUpper,
	},
}

// cellEscaper turns the characters that would break a tab-separated record
// into their two-character escapes, so one record is always one line. It is
// shared by the table and detail paths. The backslash pair comes first and is
// escaped too, so the result can be read back: without it a value holding the
// two characters `\n` and a value holding a real newline would emit the same
// bytes.
var cellEscaper = strings.NewReplacer(
	`\`, `\\`,
	"\t", `\t`,
	"\n", `\n`,
	"\r", `\r`,
)

// TablePrinter renders rows either as an aligned table for a terminal or, when
// stdout is not a TTY, as tab-separated values with one header row: nothing
// padded, nothing truncated, one line per record.
type TablePrinter struct {
	writer   table.Writer
	maxWidth int

	// out, lean and rows serve the non-TTY path, where go-pretty is bypassed.
	out  io.Writer
	lean bool
	rows [][]string
}

// NewTable creates a TablePrinter configured for the current terminal.
// On a TTY it applies the gh-CLI-style borderless format, respects terminal
// width, and disables header formatting when colors are not enabled.
// Off a TTY it emits tab-separated rows instead.
func NewTable(out io.Writer) *TablePrinter {
	if !IsTTY() {
		return &TablePrinter{out: out, lean: true}
	}

	tw := table.NewWriter()
	tw.SetOutputMirror(out)

	style := StyleGH
	if !ColorsEnabled() {
		style.Format.Header = text.FormatDefault
	}
	tw.SetStyle(style)

	maxWidth := TerminalWidth()
	tw.SetAllowedRowLength(maxWidth)

	return &TablePrinter{
		writer:   tw,
		maxWidth: maxWidth,
	}
}

// AddHeader adds a header row to the table.
func (t *TablePrinter) AddHeader(columns ...any) {
	if t.lean {
		t.rows = append(t.rows, leanRow(columns))
		return
	}
	t.writer.AppendHeader(table.Row(columns))
}

// AddRow adds a data row to the table.
func (t *TablePrinter) AddRow(columns ...any) {
	if t.lean {
		t.rows = append(t.rows, leanRow(columns))
		return
	}
	t.writer.AppendRow(table.Row(columns))
}

// Render writes the formatted table to the output writer.
func (t *TablePrinter) Render() {
	if t.lean {
		for _, row := range t.rows {
			fmt.Fprintln(t.out, strings.Join(row, "\t"))
		}
		return
	}
	t.writer.Render()
}

// leanRow stringifies and escapes one row's cells for tab-separated output.
func leanRow(columns []any) []string {
	cells := make([]string, len(columns))
	for i, col := range columns {
		cells[i] = cellEscaper.Replace(fmt.Sprint(col))
	}
	return cells
}
