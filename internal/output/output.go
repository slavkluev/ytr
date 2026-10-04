package output

import (
	"context"
	"errors"
	"fmt"
	"io"
	"strings"

	ytrerrors "github.com/slavkluev/ytr/internal/errors"
)

// Options is what one invocation asked of its output and what the terminal it
// writes to can show. Each invocation carries its own value in its context, so
// nothing one run sets reaches the next.
type Options struct {
	// Bound to the root persistent flags.
	JSONFields []string
	JQFilter   string
	Quiet      bool
	Debug      bool

	TTY    bool
	Colors bool
	// Width is the terminal width in columns; 0 means the 80-column default.
	Width    int
	DebugOut io.Writer
}

type optionsKey struct{}

// NewContext returns a context that carries opts. It holds the pointer, so a
// command that normalizes opts.JSONFields changes them for its own run only.
func NewContext(ctx context.Context, opts *Options) context.Context {
	return context.WithValue(ctx, optionsKey{}, opts)
}

// FromContext returns the options ctx carries, or zero Options when it carries
// none: off a TTY, no JSON, no debug, no colors.
func FromContext(ctx context.Context) *Options {
	if ctx != nil {
		if opts, ok := ctx.Value(optionsKey{}).(*Options); ok && opts != nil {
			return opts
		}
	}

	return &Options{}
}

// IsJSON returns true when JSON output mode is active.
// JSON mode is active when field selection is specified or a jq filter is set.
func (o *Options) IsJSON() bool {
	return len(o.JSONFields) > 0 || o.JQFilter != ""
}

// HasFieldSelection returns true when specific JSON fields have been requested.
func (o *Options) HasFieldSelection() bool {
	return len(o.JSONFields) > 0
}

// WantsFieldHint reports whether a field-selecting command should print its
// available-fields hint instead of producing output. This is the case when the
// user asked for JSON via --json (including the empty form `--json=`, which
// pflag parses to an empty slice) but named no concrete fields and gave no --jq
// filter.
//
// jsonFlagChanged must be cmd.Flags().Changed("json"). It is required because
// `--json=` and "no --json at all" both leave JSONFields empty, so JSONFields
// alone cannot distinguish them.
func (o *Options) WantsFieldHint(jsonFlagChanged bool) bool {
	return jsonFlagChanged && !o.HasFieldSelection() && o.JQFilter == ""
}

// HandleError formats and writes an error to the given writer,
// returning the appropriate exit code.
// If err is nil, returns ExitSuccess (0).
// If err is an *ExitError, formats it as JSON or human-readable based on IsJSON().
// For unknown errors, writes a generic message and returns ExitUserError.
func (o *Options) HandleError(w io.Writer, err error) int {
	return handleError(w, err, o.IsJSON(), o.Colors)
}

// HandleInvocationError renders err the way HandleError does, but reads the
// output mode from rawArgs as well when the parsed flags do not show JSON.
//
// A failed invocation can hide the mode it asked for: pflag stops at the first
// flag it does not know, so `--nosuchflag --json key` never fills JSONFields.
// rawArgs is consulted only when err is non-nil, so a value that happens to
// read like --json (a comment body, a query) can never turn a successful run
// into JSON.
func (o *Options) HandleInvocationError(w io.Writer, err error, rawArgs []string) int {
	return handleError(w, err, o.IsJSON() || (err != nil && jsonRequestedIn(rawArgs)), o.Colors)
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

// jsonErrorRenderer is implemented by every error that knows its own JSON
// document. Matching the interface once, instead of one branch per concrete
// type, is what lets an error type add fields without touching this file.
type jsonErrorRenderer interface {
	JSONError() ([]byte, error)
}

// w is always the caller's error stream: the single document goes to stderr in
// every mode, so stdout carries command output and nothing else.
func handleError(w io.Writer, err error, asJSON, colors bool) int {
	if err == nil {
		return ytrerrors.ExitSuccess
	}

	// The exit code and the human text live on ExitError. A richer error type
	// embeds it by value and unwraps to it, so this finds one whether err is an
	// ExitError or carries one.
	exitErr := &ytrerrors.ExitError{
		ExitCode: ytrerrors.ExitUserError,
		Code:     ytrerrors.CodeUserError,
		Message:  err.Error(),
	}
	var wrapped *ytrerrors.ExitError
	if errors.As(err, &wrapped) {
		exitErr = wrapped
	}

	if !asJSON {
		ytrerrors.PrintHuman(w, exitErr, colors)
		return exitErr.ExitCode
	}

	// errors.As walks outward in, so the outermost error that renders itself
	// wins: an InvalidFieldError keeps its invalidField/validFields payload
	// rather than being flattened to the ExitError it unwraps to.
	renderer := jsonErrorRenderer(exitErr)
	var self jsonErrorRenderer
	if errors.As(err, &self) {
		renderer = self
	}

	if data, jsonErr := renderer.JSONError(); jsonErr == nil {
		fmt.Fprintln(w, string(data))
	}

	return exitErr.ExitCode
}
