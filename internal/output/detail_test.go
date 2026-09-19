package output_test

import (
	"bytes"
	"errors"
	"strings"
	"testing"

	"github.com/slavkluev/ytr/internal/output"
	"github.com/slavkluev/ytr/internal/testutil"
)

func TestDetailOffTTYWritesTabSeparatedRows(t *testing.T) {
	testutil.ResetOutputFlags(t)
	output.SetTTY(false)

	var buf bytes.Buffer
	d := output.NewDetail(&buf)
	d.Field("Key", "MTP-1")
	d.Field("Created", "2026-09-19T14:22:31+03:00")
	if err := d.Err(); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	want := "Key\tMTP-1\nCreated\t2026-09-19T14:22:31+03:00\n"
	if got := buf.String(); got != want {
		t.Errorf("detail = %q, want %q", got, want)
	}
}

func TestDetailOnTTYWritesLabeledRows(t *testing.T) {
	testutil.ResetOutputFlags(t)
	t.Setenv("NO_COLOR", "1")
	output.SetTTY(true)

	var buf bytes.Buffer
	d := output.NewDetail(&buf)
	d.Field("Key", "MTP-1")
	if err := d.Err(); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if got, want := buf.String(), "Key:  MTP-1\n"; got != want {
		t.Errorf("detail = %q, want %q", got, want)
	}
}

func TestDetailBlockKeepsItsShapeOffTTY(t *testing.T) {
	testutil.ResetOutputFlags(t)
	output.SetTTY(false)

	var buf bytes.Buffer
	d := output.NewDetail(&buf)
	d.Field("Key", "MTP-1")
	d.Block("Description", "The login page returns 500.")
	if err := d.Err(); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	want := "Key\tMTP-1\n\nDescription:\n  The login page returns 500.\n"
	if got := buf.String(); got != want {
		t.Errorf("detail = %q, want %q", got, want)
	}
}

// failingWriter reports an error on every write.
type failingWriter struct{}

func (failingWriter) Write([]byte) (int, error) { return 0, errors.New("write failed") }

func TestDetailKeepsFirstWriteError(t *testing.T) {
	testutil.ResetOutputFlags(t)
	output.SetTTY(false)

	d := output.NewDetail(failingWriter{})
	d.Field("Key", "MTP-1")
	d.Field("Name", "second row")
	d.Block("Description", "third write")

	if d.Err() == nil {
		t.Error("a failed write must reach the caller through Err")
	}
}

func TestDetailOffTTYEscapesControlCharacters(t *testing.T) {
	testutil.ResetOutputFlags(t)
	output.SetTTY(false)

	var buf bytes.Buffer
	d := output.NewDetail(&buf)
	d.Field("Summary", "first\nsecond\tthird\rfourth")
	d.Field("Key", "MTP-1")
	if err := d.Err(); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	got := buf.String()
	want := "Summary\t" + `first\nsecond\tthird\rfourth` + "\nKey\tMTP-1\n"
	if got != want {
		t.Errorf("detail = %q, want %q", got, want)
	}
	if lines := strings.Count(got, "\n"); lines != 2 {
		t.Errorf("two fields must be two lines, got %d in %q", lines, got)
	}
}
