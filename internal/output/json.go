package output

import (
	"fmt"
	"io"

	"github.com/slavkluev/ytr/internal/jsonenc"
)

// PrintJSON writes data to w as one line of compact JSON, whatever w is: the
// reader is a program paying for every byte.
func PrintJSON(w io.Writer, data any) error {
	bytes, err := jsonenc.Marshal(data)
	if err != nil {
		return fmt.Errorf("failed to marshal JSON: %w", err)
	}

	_, writeErr := fmt.Fprintln(w, string(bytes))
	return writeErr
}
