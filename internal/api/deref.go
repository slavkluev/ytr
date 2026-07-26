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
// Prefers a non-empty Display, then a non-empty Login, then the ID, before
// giving up and returning the fallback. Embedded references (Component.Lead,
// Queue.Lead) often carry only an ID, so falling through to it preserves a
// meaningful value instead of rendering blank.
func DerefUser(u *tracker.User, fallback string) string {
	if u == nil {
		return fallback
	}
	if u.Display != nil && *u.Display != "" {
		return *u.Display
	}
	if u.Login != nil && *u.Login != "" {
		return *u.Login
	}
	if u.ID != nil {
		return string(*u.ID)
	}
	return fallback
}

// DerefUserID safely extracts the identifier from a *tracker.User pointer.
// Embedded references carry only Self, ID, and Display, so the ID is the one
// stable handle for telling apart users who share a display name — DerefUser
// collapses those to the same string.
func DerefUserID(u *tracker.User, fallback string) string {
	if u == nil {
		return fallback
	}
	return DerefFlexString(u.ID, fallback)
}
