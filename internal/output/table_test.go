package output_test

import (
	"bytes"
	"strings"
	"testing"

	"github.com/slavkluev/ytr/internal/output"
	"github.com/slavkluev/ytr/internal/testutil"
)

func TestTableOffTTYJoinsColumnsWithTabs(t *testing.T) {
	testutil.ResetOutputFlags(t)
	output.SetTTY(false)

	var buf bytes.Buffer
	tbl := output.NewTable(&buf)
	tbl.AddHeader("KEY", "STATUS", "ASSIGNEE", "SUMMARY")
	tbl.AddRow("MTP-1", "Open", "john.doe", "Fix login")
	tbl.AddRow("MTP-2", "Closed", "jane.roe", "Ship it")
	tbl.Render()

	want := "KEY\tSTATUS\tASSIGNEE\tSUMMARY\n" +
		"MTP-1\tOpen\tjohn.doe\tFix login\n" +
		"MTP-2\tClosed\tjane.roe\tShip it\n"
	if got := buf.String(); got != want {
		t.Errorf("table = %q, want %q", got, want)
	}
}

func TestTableOffTTYHasNoANSI(t *testing.T) {
	testutil.ResetOutputFlags(t)
	t.Setenv("NO_COLOR", "")
	t.Setenv("CLICOLOR_FORCE", "")
	t.Setenv("CLICOLOR", "")
	output.SetTTY(false)

	var buf bytes.Buffer
	tbl := output.NewTable(&buf)
	tbl.AddHeader("KEY", "SUMMARY")
	tbl.AddRow("MTP-1", "Fix login")
	tbl.Render()

	if strings.Contains(buf.String(), "\x1b") {
		t.Errorf("off-TTY table carries ANSI escapes: %q", buf.String())
	}
}

func TestTableOnTTYPadsColumns(t *testing.T) {
	testutil.ResetOutputFlags(t)
	output.SetTTY(true)

	var buf bytes.Buffer
	tbl := output.NewTable(&buf)
	tbl.AddHeader("KEY", "SUMMARY")
	tbl.AddRow("MTP-1", "Fix login")
	tbl.AddRow("MTP-1234", "Ship it")
	tbl.Render()

	got := buf.String()
	if strings.Contains(got, "\t") {
		t.Errorf("TTY table uses tabs instead of padding: %q", got)
	}

	// Every line starts its second column at the same offset.
	lines := strings.Split(strings.TrimRight(got, "\n"), "\n")
	if len(lines) != 3 {
		t.Fatalf("expected a header and two rows, got %q", got)
	}
	want := strings.Index(lines[0], "SUMMARY")
	for _, line := range lines[1:] {
		if strings.Index(line, "Fix login") != want && strings.Index(line, "Ship it") != want {
			t.Errorf("second column is not aligned at %d in %q", want, line)
		}
	}
}

func TestTableOffTTYEscapesControlCharacters(t *testing.T) {
	testutil.ResetOutputFlags(t)
	output.SetTTY(false)

	var buf bytes.Buffer
	tbl := output.NewTable(&buf)
	tbl.AddHeader("ID", "BODY")
	tbl.AddRow("1", "first\nsecond\tthird\rfourth")
	tbl.Render()

	got := buf.String()
	want := "ID\tBODY\n" + `1` + "\t" + `first\nsecond\tthird\rfourth` + "\n"
	if got != want {
		t.Errorf("table = %q, want %q", got, want)
	}
	if lines := strings.Count(got, "\n"); lines != 2 {
		t.Errorf("one record must be one line, got %d lines in %q", lines, got)
	}
}

func TestTableOffTTYKeepsLongCellWhole(t *testing.T) {
	testutil.ResetOutputFlags(t)
	output.SetTTY(false)

	summary := strings.Repeat("long summary ", 40)

	var buf bytes.Buffer
	tbl := output.NewTable(&buf)
	tbl.AddHeader("KEY", "SUMMARY")
	tbl.AddRow("MTP-1", summary)
	tbl.Render()

	got := buf.String()
	if !strings.Contains(got, summary) {
		t.Errorf("off-TTY table shortened the summary: %q", got)
	}
	if strings.Contains(got, "...") {
		t.Errorf("off-TTY table added an ellipsis: %q", got)
	}
}

func TestTableOffTTYKeepsForcedColor(t *testing.T) {
	testutil.ResetOutputFlags(t)
	t.Setenv("NO_COLOR", "")
	t.Setenv("CLICOLOR_FORCE", "1")
	output.SetTTY(false)

	if !output.ColorsEnabled() {
		t.Fatal("CLICOLOR_FORCE=1 must keep colors on off a TTY")
	}

	var buf bytes.Buffer
	tbl := output.NewTable(&buf)
	tbl.AddHeader("KEY", "STATUS")
	tbl.AddRow("MTP-1", "\x1b[32mOpen\x1b[0m")
	tbl.Render()

	got := buf.String()
	if !strings.Contains(got, "\x1b[32mOpen\x1b[0m") {
		t.Errorf("forced color was dropped: %q", got)
	}
	if !strings.Contains(got, "MTP-1\t") {
		t.Errorf("forced color must not cost the tab separators: %q", got)
	}
}

// unescapeCell reverses the escaping the off-TTY table applies to a cell.
func unescapeCell(t *testing.T, cell string) string {
	t.Helper()
	var out strings.Builder
	for i := 0; i < len(cell); i++ {
		if cell[i] != '\\' {
			out.WriteByte(cell[i])
			continue
		}
		i++
		if i == len(cell) {
			t.Fatalf("cell ends in a lone backslash: %q", cell)
		}
		switch cell[i] {
		case '\\':
			out.WriteByte('\\')
		case 't':
			out.WriteByte('\t')
		case 'n':
			out.WriteByte('\n')
		case 'r':
			out.WriteByte('\r')
		default:
			t.Fatalf("unknown escape %q in %q", cell[i:i+1], cell)
		}
	}
	return out.String()
}

func TestTableOffTTYEscapingRoundTrips(t *testing.T) {
	testutil.ResetOutputFlags(t)
	output.SetTTY(false)

	values := []string{
		"plain",
		"a real\nnewline",
		`the two characters \n`,
		"tab\there",
		"carriage\rreturn",
		`a backslash \ alone`,
		`\\t`,
	}

	for _, value := range values {
		var buf bytes.Buffer
		tbl := output.NewTable(&buf)
		tbl.AddRow(value)
		tbl.Render()

		cell := strings.TrimSuffix(buf.String(), "\n")
		if got := unescapeCell(t, cell); got != value {
			t.Errorf("round trip of %q gave %q (encoded as %q)", value, got, cell)
		}
	}
}

func TestTableOffTTYTellsRealNewlineFromItsEscape(t *testing.T) {
	testutil.ResetOutputFlags(t)
	output.SetTTY(false)

	render := func(value string) string {
		var buf bytes.Buffer
		tbl := output.NewTable(&buf)
		tbl.AddRow(value)
		tbl.Render()
		return buf.String()
	}

	fromNewline := render("a\nb")
	fromEscape := render(`a\nb`)
	if fromNewline == fromEscape {
		t.Errorf("a real newline and the characters %q encode alike: %q", `\n`, fromNewline)
	}
}
