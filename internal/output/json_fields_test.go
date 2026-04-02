package output_test

import (
	"bytes"
	"encoding/json"
	"errors"
	"strings"
	"testing"

	ytrerrors "github.com/slavkluev/ytr/internal/errors"
	"github.com/slavkluev/ytr/internal/output"
)

type testItem struct {
	Key    string `json:"key"`
	Name   string `json:"name"`
	Status string `json:"status"`
}

func TestFilterFields(t *testing.T) {
	item := testItem{Key: "A", Name: "B", Status: "C"}
	result := output.FilterFields(item, []string{"key", "status"})

	if len(result) != 2 {
		t.Fatalf("FilterFields returned %d fields, want 2", len(result))
	}
	if result["key"] != "A" {
		t.Errorf("key = %v, want %q", result["key"], "A")
	}
	if result["status"] != "C" {
		t.Errorf("status = %v, want %q", result["status"], "C")
	}
	if _, ok := result["name"]; ok {
		t.Error("FilterFields should not include unrequested field 'name'")
	}
}

func TestFilterFields_AllFields(t *testing.T) {
	item := testItem{Key: "X", Name: "Y", Status: "Z"}
	result := output.FilterFields(item, []string{"key", "name", "status"})

	if len(result) != 3 {
		t.Fatalf("FilterFields returned %d fields, want 3", len(result))
	}
	if result["key"] != "X" {
		t.Errorf("key = %v, want %q", result["key"], "X")
	}
	if result["name"] != "Y" {
		t.Errorf("name = %v, want %q", result["name"], "Y")
	}
	if result["status"] != "Z" {
		t.Errorf("status = %v, want %q", result["status"], "Z")
	}
}

func TestFilterFields_EmptyFields(t *testing.T) {
	item := testItem{Key: "A", Name: "B", Status: "C"}
	result := output.FilterFields(item, []string{})

	if len(result) != 0 {
		t.Errorf("FilterFields with empty fields returned %d entries, want 0", len(result))
	}
}

func TestValidateFields_Valid(t *testing.T) {
	err := output.ValidateFields([]string{"key", "status"}, []string{"key", "name", "status"})
	if err != nil {
		t.Errorf("ValidateFields with valid fields returned error: %v", err)
	}
}

func TestValidateFields_Invalid(t *testing.T) {
	err := output.ValidateFields([]string{"key", "invalid"}, []string{"key", "name", "status"})
	if err == nil {
		t.Fatal("ValidateFields with invalid field should return error")
	}

	var fieldErr *ytrerrors.InvalidFieldError
	ok := false
	fe := &ytrerrors.InvalidFieldError{}
	if errors.As(err, &fe) {
		fieldErr = fe
		ok = true
	}
	if !ok {
		t.Fatalf("error type = %T, want *errors.InvalidFieldError", err)
	}

	if fieldErr.InvalidField != "invalid" {
		t.Errorf("InvalidField = %q, want %q", fieldErr.InvalidField, "invalid")
	}
	if len(fieldErr.ValidFields) != 3 {
		t.Errorf("ValidFields length = %d, want 3", len(fieldErr.ValidFields))
	}
}

func TestValidateFields_CaseInsensitive(t *testing.T) {
	err := output.ValidateFields([]string{"Key"}, []string{"key"})
	if err != nil {
		t.Errorf("ValidateFields should be case-insensitive, got error: %v", err)
	}
}

func TestPrintFieldHint(t *testing.T) {
	var buf bytes.Buffer
	err := output.PrintFieldHint(&buf, "issue list", []string{"key", "summary", "status"})

	if err == nil {
		t.Fatal("PrintFieldHint should return an ExitError")
	}

	var exitErr *ytrerrors.ExitError
	ee := &ytrerrors.ExitError{}
	if errors.As(err, &ee) {
		exitErr = ee
	} else {
		t.Fatalf("error type = %T, want *errors.ExitError", err)
	}

	if exitErr.ExitCode != 1 {
		t.Errorf("ExitCode = %d, want 1", exitErr.ExitCode)
	}

	out := buf.String()
	if !strings.Contains(out, "issue list") {
		t.Errorf("output should contain command name, got: %q", out)
	}
	if !strings.Contains(out, "key") {
		t.Errorf("output should list field 'key', got: %q", out)
	}
	if !strings.Contains(out, "summary") {
		t.Errorf("output should list field 'summary', got: %q", out)
	}
}

func TestApplyJQ_Simple(t *testing.T) {
	var buf bytes.Buffer
	data := map[string]any{"key": "ISSUE-1"}

	err := output.ApplyJQ(&buf, data, ".key")
	if err != nil {
		t.Fatalf("ApplyJQ returned error: %v", err)
	}

	got := strings.TrimSpace(buf.String())
	if got != "ISSUE-1" {
		t.Errorf("ApplyJQ(.key) = %q, want %q (raw string, no quotes)", got, "ISSUE-1")
	}
}

func TestApplyJQ_ArrayFilter(t *testing.T) {
	var buf bytes.Buffer
	data := map[string]any{
		"items": []any{
			map[string]any{"key": "A"},
			map[string]any{"key": "B"},
		},
	}

	err := output.ApplyJQ(&buf, data, ".items[].key")
	if err != nil {
		t.Fatalf("ApplyJQ returned error: %v", err)
	}

	lines := strings.Split(strings.TrimSpace(buf.String()), "\n")
	if len(lines) != 2 {
		t.Fatalf("ApplyJQ(.items[].key) returned %d lines, want 2", len(lines))
	}
	if lines[0] != "A" {
		t.Errorf("line 0 = %q, want %q", lines[0], "A")
	}
	if lines[1] != "B" {
		t.Errorf("line 1 = %q, want %q", lines[1], "B")
	}
}

func TestApplyJQ_InvalidExpression(t *testing.T) {
	var buf bytes.Buffer
	data := map[string]any{"key": "A"}

	err := output.ApplyJQ(&buf, data, ".[invalid")
	if err == nil {
		t.Fatal("ApplyJQ with invalid expression should return error")
	}

	var exitErr *ytrerrors.ExitError
	ee := &ytrerrors.ExitError{}
	if errors.As(err, &ee) {
		exitErr = ee
	} else {
		t.Fatalf("error type = %T, want *errors.ExitError", err)
	}

	if !strings.Contains(exitErr.Message, "invalid jq expression") {
		t.Errorf("error message = %q, should contain 'invalid jq expression'", exitErr.Message)
	}
}

func TestApplyJQ_NonStringResult(t *testing.T) {
	var buf bytes.Buffer
	data := map[string]any{"count": 42}

	err := output.ApplyJQ(&buf, data, ".count")
	if err != nil {
		t.Fatalf("ApplyJQ returned error: %v", err)
	}

	got := strings.TrimSpace(buf.String())
	if got != "42" {
		t.Errorf("ApplyJQ(.count) = %q, want %q", got, "42")
	}
}

func TestInvalidFieldError_JSONError(t *testing.T) {
	err := ytrerrors.NewInvalidFieldError("badfield", []string{"key", "summary"})

	data, jsonErr := err.JSONError()
	if jsonErr != nil {
		t.Fatalf("JSONError() returned error: %v", jsonErr)
	}

	var result map[string]any
	if unmarshalErr := json.Unmarshal(data, &result); unmarshalErr != nil {
		t.Fatalf("JSONError() produced invalid JSON: %v", unmarshalErr)
	}

	if result["code"] != "invalid_field" {
		t.Errorf("code = %v, want %q", result["code"], "invalid_field")
	}
	if result["invalidField"] != "badfield" {
		t.Errorf("invalidField = %v, want %q", result["invalidField"], "badfield")
	}

	validFields, ok := result["validFields"].([]any)
	if !ok {
		t.Fatalf("validFields type = %T, want []any", result["validFields"])
	}
	if len(validFields) != 2 {
		t.Errorf("validFields length = %d, want 2", len(validFields))
	}
}
