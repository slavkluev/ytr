package output_test

import (
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
