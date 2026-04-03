package output

import (
	"io"

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

// TablePrinter wraps go-pretty's table.Writer for TTY-aware table rendering.
type TablePrinter struct {
	writer   table.Writer
	maxWidth int
}

// NewTable creates a TablePrinter configured for the current terminal.
// It applies the gh-CLI-style borderless format, respects terminal width,
// and disables header formatting when colors are not enabled.
func NewTable(out io.Writer) *TablePrinter {
	tw := table.NewWriter()
	tw.SetOutputMirror(out)

	style := StyleGH
	if !ColorsEnabled() {
		style.Format.Header = text.FormatDefault
	}
	tw.SetStyle(style)

	maxWidth := 80
	if IsTTY() {
		maxWidth = TerminalWidth()
	}
	tw.SetAllowedRowLength(maxWidth)

	return &TablePrinter{
		writer:   tw,
		maxWidth: maxWidth,
	}
}

// AddHeader adds a header row to the table.
func (t *TablePrinter) AddHeader(columns ...any) {
	t.writer.AppendHeader(table.Row(columns))
}

// AddRow adds a data row to the table.
func (t *TablePrinter) AddRow(columns ...any) {
	t.writer.AppendRow(table.Row(columns))
}

// Render writes the formatted table to the output writer.
func (t *TablePrinter) Render() {
	t.writer.Render()
}
