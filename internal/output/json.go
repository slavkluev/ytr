package output

import (
	"encoding/json"
	"fmt"
	"io"
)

// PrintJSON writes data as JSON to the writer, followed by a newline.
// It is indented for a terminal and minified to a single line off a TTY,
// where the reader is a program paying for every byte.
// No ANSI codes or color are ever included, so the output stays machine-parseable.
func PrintJSON(w io.Writer, data any) error {
	var (
		bytes []byte
		err   error
	)
	if IsTTY() {
		bytes, err = json.MarshalIndent(data, "", "  ")
	} else {
		bytes, err = json.Marshal(data)
	}
	if err != nil {
		return fmt.Errorf("failed to marshal JSON: %w", err)
	}

	_, writeErr := fmt.Fprintln(w, string(bytes))
	return writeErr
}
