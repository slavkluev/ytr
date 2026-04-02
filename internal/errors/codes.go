// Package errors provides typed errors with semantic exit codes for ytr CLI.
package errors

// Exit code constants define semantic meanings for process exit codes.
// These follow Unix conventions and ytr-specific semantics per OUT-05.
const (
	// ExitSuccess indicates the command completed successfully.
	ExitSuccess = 0

	// ExitUserError indicates invalid input, bad flags, or general user mistakes.
	ExitUserError = 1

	// ExitAuthError indicates authentication or authorization failure.
	ExitAuthError = 3

	// ExitNotFound indicates the requested resource does not exist.
	ExitNotFound = 4

	// ExitRateLimited indicates the API rate limit was exceeded.
	ExitRateLimited = 5

	// ExitInterrupted indicates the process was interrupted by a signal (e.g., Ctrl+C).
	ExitInterrupted = 130
)
