package output_test

import (
	"strings"
	"testing"
	"unicode/utf8"

	"github.com/slavkluev/ytr/internal/output"
)

func TestTruncateDisplay_ASCII(t *testing.T) {
	got := output.TruncateDisplay("Hello world", 8)
	if got != "Hello..." {
		t.Errorf("TruncateDisplay(ASCII) = %q, want %q", got, "Hello...")
	}
}

func TestTruncateDisplay_CyrillicPreservesUTF8(t *testing.T) {
	got := output.TruncateDisplay("Привет мир", 8)
	if got != "Приве..." {
		t.Errorf("TruncateDisplay(cyrillic) = %q, want %q", got, "Приве...")
	}
	if !utf8.ValidString(got) {
		t.Errorf("TruncateDisplay(cyrillic) returned invalid UTF-8: %q", got)
	}
}

func TestTruncateDisplay_TooSmallForEllipsis(t *testing.T) {
	got := output.TruncateDisplay("Привет", 2)
	if got != "Пр" {
		t.Errorf("TruncateDisplay(short width) = %q, want %q", got, "Пр")
	}
	if !utf8.ValidString(got) {
		t.Errorf("TruncateDisplay(short width) returned invalid UTF-8: %q", got)
	}
}

func TestFitColumnOffTTYReturnsValueWhole(t *testing.T) {
	opts := output.Options{}

	summary := strings.Repeat("long summary ", 40)
	if got := opts.FitColumn(summary, 51, 10); got != summary {
		t.Errorf("FitColumn off a TTY = %q, want the value untouched", got)
	}
}

func TestFitColumnOnTTYTruncatesToTheRemainingWidth(t *testing.T) {
	opts := output.Options{TTY: true, Colors: true}

	summary := strings.Repeat("x", 200)
	got := opts.FitColumn(summary, 51, 10)
	if got == summary {
		t.Fatalf("FitColumn on a TTY did not shorten a 200-cell value")
	}
	if want := output.TruncateDisplay(summary, opts.TerminalWidth()-51); got != want {
		t.Errorf("FitColumn = %q, want %q", got, want)
	}
	if !strings.HasSuffix(got, "...") {
		t.Errorf("FitColumn = %q, want an ellipsis", got)
	}
}

func TestFitColumnOnTTYUsesTheTerminalWidth(t *testing.T) {
	opts := output.Options{TTY: true, Width: 120}

	if got := opts.TerminalWidth(); got != 120 {
		t.Fatalf("TerminalWidth() = %d, want the 120 the options carry", got)
	}

	summary := strings.Repeat("x", 200)
	if got, want := opts.FitColumn(summary, 51, 10), output.TruncateDisplay(summary, 120-51); got != want {
		t.Errorf("FitColumn = %q (%d cells), want %q (%d cells)", got, len(got), want, len(want))
	}
}

func TestFitColumnOnTTYKeepsTheMinimumWidth(t *testing.T) {
	opts := output.Options{TTY: true, Colors: true}

	// A reservation wider than the terminal must not collapse the column.
	got := opts.FitColumn(strings.Repeat("x", 50), 500, 10)
	if len(got) != 10 {
		t.Errorf("FitColumn = %q, want 10 cells", got)
	}
}
