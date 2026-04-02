// Package testutil provides shared test helpers for ytr command tests.
package testutil

import (
	"testing"

	"github.com/slavkluev/go-yandex-tracker/tracker"

	"github.com/slavkluev/ytr/internal/output"
)

// ResetOutputFlags resets global output flags and restores them after the test.
func ResetOutputFlags(t *testing.T) {
	t.Helper()
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
