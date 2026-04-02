package jsonfields

import "testing"

func TestRegisterAndGet(t *testing.T) {
	// Clear registry for test isolation.
	mu.Lock()
	registry = make(map[string][]string)
	mu.Unlock()

	Register("ytr issue list", []string{"key", "summary"})
	fields, ok := Get("ytr issue list")
	if !ok {
		t.Fatal("Get returned ok=false for registered path")
	}
	if len(fields) != 2 || fields[0] != "key" || fields[1] != "summary" {
		t.Errorf("Get returned %v, want [key summary]", fields)
	}
}

func TestGetUnregistered(t *testing.T) {
	mu.Lock()
	registry = make(map[string][]string)
	mu.Unlock()

	_, ok := Get("ytr nonexistent")
	if ok {
		t.Error("Get returned ok=true for unregistered path")
	}
}
