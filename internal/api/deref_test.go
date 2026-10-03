package api_test

import (
	"testing"

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
