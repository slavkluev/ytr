package errors

import (
	"fmt"
	"io"
)

// PrintHuman writes err as plain text in gh CLI style: "Error: <message>" on
// the first line and the suggestion, if any, on the next.
func PrintHuman(w io.Writer, err *ExitError) {
	fmt.Fprintf(w, "Error: %s\n", err.Message)

	if err.Suggestion != "" {
		fmt.Fprintf(w, "%s\n", err.Suggestion)
	}
}
