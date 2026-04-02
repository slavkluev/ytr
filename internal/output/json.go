package output

import (
	"encoding/json"
	"fmt"
	"io"
)

// PrintJSON writes data as indented JSON to the writer, followed by a newline.
// No ANSI codes or color are ever included in the output per OUT-01.
func PrintJSON(w io.Writer, data any) error {
	bytes, err := json.MarshalIndent(data, "", "  ")
	if err != nil {
		return fmt.Errorf("failed to marshal JSON: %w", err)
	}

	_, writeErr := fmt.Fprintln(w, string(bytes))
	return writeErr
}
