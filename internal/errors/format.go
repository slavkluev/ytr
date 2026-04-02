package errors

import (
	"fmt"
	"io"
)

const errorPrefixANSI = "\x1b[1;31mError\x1b[0m"

// PrintHuman formats an error for TTY display in gh CLI style per D-06.
// Format: "Error: <message>" on first line, suggestion on next line if present.
// When useColors is true, the "Error" prefix is rendered in red bold.
func PrintHuman(w io.Writer, err *ExitError, useColors bool) {
	prefix := "Error"
	if useColors {
		prefix = errorPrefixANSI
	}

	fmt.Fprintf(w, "%s: %s\n", prefix, err.Message)

	if err.Suggestion != "" {
		fmt.Fprintf(w, "%s\n", err.Suggestion)
	}
}
