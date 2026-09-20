package output

import (
	"errors"
	"fmt"
	"io"
	"os"
	"strings"

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
	ttyOverride = nil
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
	return handleError(w, err, IsJSON())
}

// HandleInvocationError renders err the way HandleError does, but reads the
// output mode from rawArgs as well when the parsed flags do not show JSON.
//
// A failed invocation can hide the mode it asked for: pflag stops at the first
// flag it does not know, so `--nosuchflag --json key` never fills JSONFields.
// rawArgs is consulted only when err is non-nil, so a value that happens to
// read like --json (a comment body, a query) can never turn a successful run
// into JSON.
func HandleInvocationError(w io.Writer, err error, rawArgs []string) int {
	return handleError(w, err, IsJSON() || (err != nil && jsonRequestedIn(rawArgs)))
}

// jsonRequestedIn reports whether args ask for JSON output, by the rule IsJSON
// applies to the parsed flags: a non-empty --json field list or a non-empty
// --jq filter. It reads the arguments as given, so it survives a parse failure.
func jsonRequestedIn(args []string) bool {
	for i, arg := range args {
		// Everything after a bare -- is a value, not a flag.
		if arg == "--" {
			return false
		}

		if flagValueAt(args, i, "--json") != "" || flagValueAt(args, i, "--jq") != "" {
			return true
		}
	}

	return false
}

// flagValueAt returns the value args[i] gives to the flag called name, or ""
// when args[i] is not that flag or carries no value. `--json=` with nothing
// after it carries no value, which keeps the empty form on its plain-text
// field-hint path.
func flagValueAt(args []string, i int, name string) string {
	if value, ok := strings.CutPrefix(args[i], name+"="); ok {
		return value
	}

	if args[i] != name || i+1 >= len(args) {
		return ""
	}

	// pflag would take a following flag as the value; refusing it here keeps a
	// flag that was really some other flag's value from inventing a JSON
	// request out of a run that failed for an unrelated reason.
	if next := args[i+1]; !strings.HasPrefix(next, "-") {
		return next
	}

	return ""
}

// handleError renders err, as JSON when asJSON is set and as human-readable
// text otherwise, and returns the exit code err carries.
func handleError(w io.Writer, err error, asJSON bool) int {
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
		if asJSON {
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
		if asJSON {
			data, jsonErr := exitErr.JSONError()
			if jsonErr == nil {
				fmt.Fprintln(JSONErrorWriter(w), string(data))
			}
		} else {
			ytrerrors.PrintHuman(w, exitErr, ColorsEnabled())
		}
		return exitErr.ExitCode
	}

	if asJSON {
		data, jsonErr := genericExitErr.JSONError()
		if jsonErr == nil {
			fmt.Fprintln(JSONErrorWriter(w), string(data))
		}
		return genericExitErr.ExitCode
	}

	ytrerrors.PrintHuman(w, genericExitErr, ColorsEnabled())
	return genericExitErr.ExitCode
}
