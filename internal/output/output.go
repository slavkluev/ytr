// Package output renders what a command prints: its JSON document, cut to the
// selected fields or filtered through --jq, and the error a failed run ends
// with.
package output

import (
	"context"
	"errors"
	"fmt"
	"io"

	ytrerrors "github.com/slavkluev/ytr/internal/errors"
)

// Options is what one invocation asked of its output. Each invocation carries
// its own value in its context, so nothing one run sets reaches the next.
type Options struct {
	// Bound to the root persistent flags.
	JSONFields []string
	JQFilter   string
	Debug      bool

	DebugOut io.Writer
}

type optionsKey struct{}

// NewContext returns a context that carries opts.
func NewContext(ctx context.Context, opts *Options) context.Context {
	return context.WithValue(ctx, optionsKey{}, opts)
}

// FromContext returns the options ctx carries, or zero Options when it carries
// none: no field selection, no jq filter, no debug.
func FromContext(ctx context.Context) *Options {
	if ctx != nil {
		if opts, ok := ctx.Value(optionsKey{}).(*Options); ok && opts != nil {
			return opts
		}
	}

	return &Options{}
}

// HasFieldSelection returns true when specific JSON fields have been requested.
func (o *Options) HasFieldSelection() bool {
	return len(o.JSONFields) > 0
}

// HasEmptySelection reports whether --json was given but selects no field, as
// `--json=` and `--json ""` do, with or without --jq: pflag parses both to an
// empty slice.
//
// jsonFlagChanged must be cmd.Flags().Changed("json"). It is required because
// an empty selection and no --json at all both leave JSONFields empty, so
// JSONFields alone cannot distinguish them.
func (o *Options) HasEmptySelection(jsonFlagChanged bool) bool {
	return jsonFlagChanged && !o.HasFieldSelection()
}

// HandleInvocationError writes err to w as one JSON document, whatever the
// flags asked for, and returns the exit code it carries. A nil err writes
// nothing and returns ExitSuccess.
func (o *Options) HandleInvocationError(w io.Writer, err error) int {
	return handleError(w, err)
}

// jsonErrorRenderer is implemented by every error that knows its own JSON
// document. Matching the interface once, instead of one branch per concrete
// type, is what lets an error type add fields without touching this file.
type jsonErrorRenderer interface {
	JSONError() ([]byte, error)
}

// w is always the caller's error stream: the single document goes to stderr,
// so stdout carries command output and nothing else.
func handleError(w io.Writer, err error) int {
	if err == nil {
		return ytrerrors.ExitSuccess
	}

	// The exit code lives on ExitError. A richer error type embeds it by value
	// and unwraps to it, so this finds one whether err is an ExitError or
	// carries one.
	exitErr := &ytrerrors.ExitError{
		ExitCode: ytrerrors.ExitUserError,
		Code:     ytrerrors.CodeUserError,
		Message:  err.Error(),
	}
	var wrapped *ytrerrors.ExitError
	if errors.As(err, &wrapped) {
		exitErr = wrapped
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
