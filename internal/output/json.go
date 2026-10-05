package output

import (
	"fmt"
	"io"

	"github.com/slavkluev/ytr/internal/jsonenc"
)

// PrintJSON writes data as JSON to the writer, followed by a newline.
// It is indented for a terminal and minified to a single line off a TTY,
// where the reader is a program paying for every byte.
// No ANSI codes or color are ever included, so the output stays machine-parseable.
func (o *Options) PrintJSON(w io.Writer, data any) error {
	var (
		bytes []byte
		err   error
	)
	if o.TTY {
		bytes, err = jsonenc.MarshalIndent(data, "", "  ")
	} else {
		bytes, err = jsonenc.Marshal(data)
	}
	if err != nil {
		return fmt.Errorf("failed to marshal JSON: %w", err)
	}

	_, writeErr := fmt.Fprintln(w, string(bytes))
	return writeErr
}
