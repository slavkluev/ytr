package output

import (
	"fmt"
	"io"
)

// PrintQuiet writes each value on its own line with no headers or formatting.
// This implements the --quiet mode: only primary identifiers, one per line.
func PrintQuiet(w io.Writer, values ...string) {
	for _, value := range values {
		fmt.Fprintln(w, value)
	}
}
