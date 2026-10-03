package output_test

import (
	"bytes"
	"errors"
	"strings"
	"testing"

	"github.com/jedib0t/go-pretty/v6/text"

	"github.com/slavkluev/ytr/internal/output"
)

// go-pretty decides once, at init, whether to emit ANSI codes, from NO_COLOR
// and TERM; without this a developer's NO_COLOR=1 would strip the codes a test
// looks for.
func enableANSI(t *testing.T) {
	t.Helper()
	if text.Bold.Sprint("x") == "x" {
		text.EnableColors()
		t.Cleanup(text.DisableColors)
	}
}

func TestDetailOffTTYWritesTabSeparatedRows(t *testing.T) {
	opts := output.Options{}

	var buf bytes.Buffer
	d := opts.NewDetail(&buf)
	d.Field("Key", "APP-1")
	d.Field("Created", "2026-09-19T14:22:31+03:00")
	if err := d.Err(); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	want := "Key\tAPP-1\nCreated\t2026-09-19T14:22:31+03:00\n"
	if got := buf.String(); got != want {
		t.Errorf("detail = %q, want %q", got, want)
	}
}

func TestDetailOnTTYWritesLabeledRows(t *testing.T) {
	opts := output.Options{TTY: true}

	var buf bytes.Buffer
	d := opts.NewDetail(&buf)
	d.Field("Key", "APP-1")
	if err := d.Err(); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if got, want := buf.String(), "Key:  APP-1\n"; got != want {
		t.Errorf("detail = %q, want %q", got, want)
	}
}

func TestDetailOnTTYWithColorsBoldsTheLabel(t *testing.T) {
	enableANSI(t)
	opts := output.Options{TTY: true, Colors: true}

	var buf bytes.Buffer
	d := opts.NewDetail(&buf)
	d.Field("Key", "APP-1")
	if err := d.Err(); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if got, want := buf.String(), "\x1b[1mKey:\x1b[0m  APP-1\n"; got != want {
		t.Errorf("detail = %q, want %q", got, want)
	}
}

func TestDetailBlockKeepsItsShapeOffTTY(t *testing.T) {
	opts := output.Options{}

	var buf bytes.Buffer
	d := opts.NewDetail(&buf)
	d.Field("Key", "APP-1")
	d.Block("Description", "The login page returns 500.")
	if err := d.Err(); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	want := "Key\tAPP-1\n\nDescription:\n  The login page returns 500.\n"
	if got := buf.String(); got != want {
		t.Errorf("detail = %q, want %q", got, want)
	}
}

// failingWriter reports an error on every write.
type failingWriter struct{}

func (failingWriter) Write([]byte) (int, error) { return 0, errors.New("write failed") }

func TestDetailKeepsFirstWriteError(t *testing.T) {
	opts := output.Options{}

	d := opts.NewDetail(failingWriter{})
	d.Field("Key", "APP-1")
	d.Field("Name", "second row")
	d.Block("Description", "third write")

	if d.Err() == nil {
		t.Error("a failed write must reach the caller through Err")
	}
}

func TestDetailOffTTYEscapesControlCharacters(t *testing.T) {
	opts := output.Options{}

	var buf bytes.Buffer
	d := opts.NewDetail(&buf)
	d.Field("Summary", "first\nsecond\tthird\rfourth")
	d.Field("Key", "APP-1")
	if err := d.Err(); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	got := buf.String()
	want := "Summary\t" + `first\nsecond\tthird\rfourth` + "\nKey\tAPP-1\n"
	if got != want {
		t.Errorf("detail = %q, want %q", got, want)
	}
	if lines := strings.Count(got, "\n"); lines != 2 {
		t.Errorf("two fields must be two lines, got %d in %q", lines, got)
	}
}
