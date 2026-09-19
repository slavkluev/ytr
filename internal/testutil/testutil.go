// Package testutil provides shared test helpers for ytr command tests.
package testutil

import (
	"testing"

	"github.com/slavkluev/go-yandex-tracker/tracker"

	"github.com/slavkluev/ytr/internal/output"
)

// ResetOutputFlags resets global output flags and restores them after the test.
// It also clears the three color variables, so a test starts from a known
// color state rather than inheriting whatever the developer's shell exports:
// CLICOLOR_FORCE=1 would otherwise put ANSI codes in output a test compares
// byte for byte. t.Setenv restores the previous values when the test ends.
func ResetOutputFlags(t *testing.T) {
	t.Helper()
	t.Setenv("NO_COLOR", "")
	t.Setenv("CLICOLOR_FORCE", "")
	t.Setenv("CLICOLOR", "")
	output.ResetFlags()
	t.Cleanup(func() {
		output.ResetFlags()
	})
}

// StrPtr returns a pointer to s.
func StrPtr(s string) *string { return new(s) }

// IntPtr returns a pointer to n.
func IntPtr(n int) *int { return new(n) }

// BoolPtr returns a pointer to b.
func BoolPtr(b bool) *bool { return new(b) }

// FlexStringPtr returns a pointer to a tracker.FlexString with value s.
func FlexStringPtr(s string) *tracker.FlexString {
	f := tracker.FlexString(s)
	return &f
}
