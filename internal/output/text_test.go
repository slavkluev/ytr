package output_test

import (
	"strings"
	"testing"
	"unicode/utf8"

	"github.com/slavkluev/ytr/internal/output"
	"github.com/slavkluev/ytr/internal/testutil"
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
	testutil.ResetOutputFlags(t)
	output.SetTTY(false)

	summary := strings.Repeat("long summary ", 40)
	if got := output.FitColumn(summary, 51, 10); got != summary {
		t.Errorf("FitColumn off a TTY = %q, want the value untouched", got)
	}
}

func TestFitColumnOnTTYTruncatesToTheRemainingWidth(t *testing.T) {
	testutil.ResetOutputFlags(t)
	output.SetTTY(true)

	summary := strings.Repeat("x", 200)
	got := output.FitColumn(summary, 51, 10)
	if got == summary {
		t.Fatalf("FitColumn on a TTY did not shorten a 200-cell value")
	}
	if want := output.TruncateDisplay(summary, output.TerminalWidth()-51); got != want {
		t.Errorf("FitColumn = %q, want %q", got, want)
	}
	if !strings.HasSuffix(got, "...") {
		t.Errorf("FitColumn = %q, want an ellipsis", got)
	}
}

func TestFitColumnOnTTYKeepsTheMinimumWidth(t *testing.T) {
	testutil.ResetOutputFlags(t)
	output.SetTTY(true)

	// A reservation wider than the terminal must not collapse the column.
	got := output.FitColumn(strings.Repeat("x", 50), 500, 10)
	if len(got) != 10 {
		t.Errorf("FitColumn = %q, want 10 cells", got)
	}
}
