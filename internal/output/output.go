package output

import (
	"errors"
	"fmt"
	"io"
	"os"

	ytrerrors "github.com/slavkluev/ytr/internal/errors"
)

// OutputMode represents the current output format.
type OutputMode int

const (
	// ModeTable is the default human-readable table output for TTY.
	ModeTable OutputMode = iota

	// ModeJSON outputs clean JSON with no ANSI codes.
	ModeJSON

	// ModeQuiet outputs only primary identifiers, one per line.
	ModeQuiet
)

// JSONFields holds the list of requested JSON field names.
// Set by the --json global persistent flag (StringSlice).
// When non-empty, output is rendered as JSON with only these fields.
var JSONFields []string

// JQFilter holds an optional jq expression to apply to JSON output.
// Set by the --jq global persistent flag.
// When set, implies JSON output mode.
var JQFilter string

// QuietFlag controls whether output is rendered in quiet mode.
// Set by the --quiet global persistent flag.
var QuietFlag bool

// IsJSON returns true when JSON output mode is active.
// JSON mode is active when field selection is specified or a jq filter is set.
func IsJSON() bool {
	return len(JSONFields) > 0 || JQFilter != ""
}

// HasFieldSelection returns true when specific JSON fields have been requested.
func HasFieldSelection() bool {
	return len(JSONFields) > 0
}

// WantsFieldHint reports whether a field-selecting command should print its
// available-fields hint instead of producing output. This is the case when the
// user asked for JSON via --json (including the empty form `--json=`, which
// pflag parses to an empty slice) but named no concrete fields and gave no --jq
// filter.
//
// jsonFlagChanged must be cmd.Flags().Changed("json"). It is required because
// `--json=` and "no --json at all" both leave JSONFields empty, so JSONFields
// alone cannot distinguish them — the same reason the auth commands key off
// Changed("json"). (The previous predicate, IsJSON() && !HasFieldSelection(),
// was unsatisfiable and the hint was dead code.)
func WantsFieldHint(jsonFlagChanged bool) bool {
	return jsonFlagChanged && !HasFieldSelection() && JQFilter == ""
}

// IsQuiet returns true when quiet output mode is active.
func IsQuiet() bool {
	return QuietFlag
}

// ResetFlags resets all output flags to their zero values.
// Used in tests to ensure clean state between test cases.
func ResetFlags() {
	JSONFields = nil
	JQFilter = ""
	QuietFlag = false
	DebugFlag = false
	SetDebugWriter(os.Stderr)
	SetJSONErrorWriter(os.Stdout)
}

// Mode returns the current output mode based on flag state.
// JSON takes precedence over Quiet.
func Mode() OutputMode {
	if IsJSON() {
		return ModeJSON
	}
	if QuietFlag {
		return ModeQuiet
	}
	return ModeTable
}

// HandleError formats and writes an error to the given writer,
// returning the appropriate exit code.
// If err is nil, returns ExitSuccess (0).
// If err is an *ExitError, formats it as JSON or human-readable based on IsJSON().
// For unknown errors, writes a generic message and returns ExitUserError.
func HandleError(w io.Writer, err error) int {
	if err == nil {
		return ytrerrors.ExitSuccess
	}

	genericExitErr := &ytrerrors.ExitError{
		ExitCode: ytrerrors.ExitUserError,
		Code:     ytrerrors.CodeUserError,
		Message:  err.Error(),
	}

	// Match *InvalidFieldError before the generic *ExitError: it embeds
	// ExitError by value, so without this branch errors.As would route it to
	// the generic handler and drop the structured invalidField/validFields
	// payload (and the "Valid fields: …" hint in human mode).
	var invalidFieldErr *ytrerrors.InvalidFieldError
	if errors.As(err, &invalidFieldErr) {
		if IsJSON() {
			if data, jsonErr := invalidFieldErr.JSONError(); jsonErr == nil {
				fmt.Fprintln(JSONErrorWriter(w), string(data))
			}
		} else {
			ytrerrors.PrintHuman(w, &invalidFieldErr.ExitError, ColorsEnabled())
		}
		return invalidFieldErr.ExitCode
	}

	var exitErr *ytrerrors.ExitError
	if errors.As(err, &exitErr) {
		if IsJSON() {
			data, jsonErr := exitErr.JSONError()
			if jsonErr == nil {
				fmt.Fprintln(JSONErrorWriter(w), string(data))
			}
		} else {
			ytrerrors.PrintHuman(w, exitErr, ColorsEnabled())
		}
		return exitErr.ExitCode
	}

	if IsJSON() {
		data, jsonErr := genericExitErr.JSONError()
		if jsonErr == nil {
			fmt.Fprintln(JSONErrorWriter(w), string(data))
		}
		return genericExitErr.ExitCode
	}

	ytrerrors.PrintHuman(w, genericExitErr, ColorsEnabled())
	return genericExitErr.ExitCode
}
