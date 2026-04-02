package testutil

import "testing"

func TestStrPtr(t *testing.T) {
	p := StrPtr("hello")
	if p == nil || *p != "hello" {
		t.Errorf("StrPtr(\"hello\") = %v, want pointer to \"hello\"", p)
	}
}

func TestIntPtr(t *testing.T) {
	p := IntPtr(42)
	if p == nil || *p != 42 {
		t.Errorf("IntPtr(42) = %v, want pointer to 42", p)
	}
}

func TestBoolPtr(t *testing.T) {
	p := BoolPtr(true)
	if p == nil || *p != true {
		t.Errorf("BoolPtr(true) = %v, want pointer to true", p)
	}
}

func TestResetOutputFlags(t *testing.T) {
	// Should not panic when called.
	ResetOutputFlags(t)
}
