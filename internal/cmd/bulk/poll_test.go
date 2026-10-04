package bulk

import (
	"strings"
	"testing"
)

func TestReadIssueKeys_FromArgs(t *testing.T) {
	keys, err := readIssueKeys([]string{"PROJ-1", "PROJ-2"}, nil)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if len(keys) != 2 || keys[0] != "PROJ-1" || keys[1] != "PROJ-2" {
		t.Errorf("expected [PROJ-1, PROJ-2], got %v", keys)
	}
}

func TestReadIssueKeys_DedupesArgs(t *testing.T) {
	keys, err := readIssueKeys([]string{"PROJ-1", "PROJ-2", "PROJ-1", "PROJ-2", "PROJ-3"}, nil)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	want := []string{"PROJ-1", "PROJ-2", "PROJ-3"}
	if len(keys) != len(want) {
		t.Fatalf("expected %v (deduped, order-preserving), got %v", want, keys)
	}
	for i, k := range want {
		if keys[i] != k {
			t.Errorf("position %d: expected %q, got %q (full: %v)", i, k, keys[i], keys)
		}
	}
}

func TestParseFieldFlags_SingleField(t *testing.T) {
	vals, err := parseFieldFlags([]string{"priority=critical"})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if vals["priority"] != "critical" {
		t.Errorf("expected priority=critical, got %v", vals["priority"])
	}
}

func TestParseFieldFlags_MultipleFields(t *testing.T) {
	vals, err := parseFieldFlags([]string{"priority=critical", "status=open"})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if vals["priority"] != "critical" {
		t.Errorf("expected priority=critical, got %v", vals["priority"])
	}
	if vals["status"] != "open" {
		t.Errorf("expected status=open, got %v", vals["status"])
	}
}

func TestParseFieldFlags_ValueWithEquals(t *testing.T) {
	vals, err := parseFieldFlags([]string{"summary=a=b=c"})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if vals["summary"] != "a=b=c" {
		t.Errorf("expected summary=a=b=c, got %v", vals["summary"])
	}
}

func TestParseFieldFlags_EmptyValue(t *testing.T) {
	vals, err := parseFieldFlags([]string{"key="})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if vals["key"] != "" {
		t.Errorf("expected key='', got %v", vals["key"])
	}
}

func TestParseFieldFlags_NoEquals(t *testing.T) {
	_, err := parseFieldFlags([]string{"invalid"})
	if err == nil {
		t.Fatal("expected error for missing =, got nil")
	}

	if got := err.Error(); !strings.Contains(got, "invalid field format") {
		t.Errorf("expected 'invalid field format' in error, got: %v", err)
	}
}

func TestParseFieldFlags_EmptyKey(t *testing.T) {
	_, err := parseFieldFlags([]string{"=value"})
	if err == nil {
		t.Fatal("expected error for empty key, got nil")
	}

	if got := err.Error(); !strings.Contains(got, "invalid field format") {
		t.Errorf("expected 'invalid field format' in error, got: %v", err)
	}
}
