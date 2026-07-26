package api_test

import (
	"testing"

	"github.com/slavkluev/go-yandex-tracker/tracker"

	"github.com/slavkluev/ytr/internal/api"
)

func ptr(s string) *string {
	return &s
}

func TestDerefString_NonNil(t *testing.T) {
	got := api.DerefString(ptr("hello"), "x")
	if got != "hello" {
		t.Errorf("DerefString(ptr(hello), x) = %q, want %q", got, "hello")
	}
}

func TestDerefString_Nil(t *testing.T) {
	got := api.DerefString(nil, "x")
	if got != "x" {
		t.Errorf("DerefString(nil, x) = %q, want %q", got, "x")
	}
}

func TestDerefUser_Nil(t *testing.T) {
	got := api.DerefUser(nil, "-")
	if got != "-" {
		t.Errorf("DerefUser(nil, -) = %q, want %q", got, "-")
	}
}

func TestDerefUser_Display(t *testing.T) {
	user := &tracker.User{
		Display: ptr("John Doe"),
		Login:   ptr("johndoe"),
	}
	got := api.DerefUser(user, "-")
	if got != "John Doe" {
		t.Errorf("DerefUser(display+login, -) = %q, want %q", got, "John Doe")
	}
}

func TestDerefUser_LoginOnly(t *testing.T) {
	user := &tracker.User{
		Login: ptr("johndoe"),
	}
	got := api.DerefUser(user, "-")
	if got != "johndoe" {
		t.Errorf("DerefUser(login-only, -) = %q, want %q", got, "johndoe")
	}
}

func TestDerefUser_EmptyDisplayFallsThroughToLogin(t *testing.T) {
	// An empty Display string must not win over a real Login.
	user := &tracker.User{
		Display: ptr(""),
		Login:   ptr("johndoe"),
	}
	if got := api.DerefUser(user, "-"); got != "johndoe" {
		t.Errorf("DerefUser(emptyDisplay+login, -) = %q, want %q", got, "johndoe")
	}
}

func TestDerefUser_IDFallback(t *testing.T) {
	// Embedded refs (Component.Lead, Queue.Lead) often carry only an ID.
	id := tracker.FlexString("user-123")
	user := &tracker.User{ID: &id}
	if got := api.DerefUser(user, "-"); got != "user-123" {
		t.Errorf("DerefUser(id-only, -) = %q, want %q", got, "user-123")
	}

	// Empty display + empty login should still fall through to the ID.
	user2 := &tracker.User{Display: ptr(""), Login: ptr(""), ID: &id}
	if got := api.DerefUser(user2, "-"); got != "user-123" {
		t.Errorf("DerefUser(emptyDisplay+emptyLogin+id, -) = %q, want %q", got, "user-123")
	}
}

func TestDerefUserID(t *testing.T) {
	numeric := tracker.FlexString("1120000000123456")
	login := tracker.FlexString("johndoe")

	tests := []struct {
		name string
		user *tracker.User
		want string
	}{
		{name: "nil user", user: nil, want: "-"},
		{name: "nil ID", user: &tracker.User{Display: ptr("John Doe")}, want: "-"},
		{name: "numeric id (cloud org)", user: &tracker.User{ID: &numeric}, want: "1120000000123456"},
		{name: "login-shaped id (360 org)", user: &tracker.User{ID: &login}, want: "johndoe"},
		{
			// Display must not shadow the ID the way DerefUser lets it.
			name: "display present alongside id",
			user: &tracker.User{Display: ptr("John Doe"), ID: &login},
			want: "johndoe",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := api.DerefUserID(tt.user, "-"); got != tt.want {
				t.Errorf("DerefUserID() = %q, want %q", got, tt.want)
			}
		})
	}
}

func TestDerefUserIDSeparatesNamesakes(t *testing.T) {
	// The defect this field exists to fix: two users share a display name,
	// so DerefUser collapses them to the same string.
	idA := tracker.FlexString("uid-a")
	idB := tracker.FlexString("uid-b")
	alice := &tracker.User{Display: ptr("Иван Петров"), ID: &idA}
	bob := &tracker.User{Display: ptr("Иван Петров"), ID: &idB}

	if api.DerefUser(alice, "") != api.DerefUser(bob, "") {
		t.Fatal("test premise broken: display names should collide")
	}
	if api.DerefUserID(alice, "") == api.DerefUserID(bob, "") {
		t.Error("DerefUserID returned the same ID for two distinct namesakes")
	}
}

func boolPtr(b bool) *bool { return &b }

func TestDerefBool(t *testing.T) {
	tests := []struct {
		name     string
		b        *bool
		fallback bool
		want     bool
	}{
		{name: "true pointer with false fallback", b: boolPtr(true), fallback: false, want: true},
		{name: "false pointer with true fallback", b: boolPtr(false), fallback: true, want: false},
		{name: "nil with true fallback", b: nil, fallback: true, want: true},
		{name: "nil with false fallback", b: nil, fallback: false, want: false},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := api.DerefBool(tt.b, tt.fallback)
			if got != tt.want {
				t.Errorf("DerefBool(%v, %v) = %v, want %v", tt.b, tt.fallback, got, tt.want)
			}
		})
	}
}
