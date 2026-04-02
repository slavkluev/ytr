package api

import (
	"github.com/slavkluev/go-yandex-tracker/tracker"
)

// DerefString safely dereferences a *string pointer.
// Returns the pointed-to value if non-nil, or the fallback otherwise.
func DerefString(s *string, fallback string) string {
	if s != nil {
		return *s
	}
	return fallback
}

// DerefInt safely dereferences an *int pointer.
// Returns the pointed-to value if non-nil, or the fallback otherwise.
func DerefInt(n *int, fallback int) int {
	if n != nil {
		return *n
	}
	return fallback
}

// DerefFlexString safely dereferences a *tracker.FlexString pointer.
// Returns the underlying string value if non-nil, or the fallback otherwise.
func DerefFlexString(f *tracker.FlexString, fallback string) string {
	if f != nil {
		return string(*f)
	}
	return fallback
}

// DerefBool safely dereferences a *bool pointer.
// Returns the pointed-to value if non-nil, or the fallback otherwise.
func DerefBool(b *bool, fallback bool) bool {
	if b != nil {
		return *b
	}
	return fallback
}

// DerefUser safely extracts a display name from a *tracker.User pointer.
// Prefers Display over Login. Returns fallback if user is nil or has no
// displayable fields.
func DerefUser(u *tracker.User, fallback string) string {
	if u == nil {
		return fallback
	}
	if u.Display != nil {
		return *u.Display
	}
	if u.Login != nil {
		return *u.Login
	}
	return fallback
}
