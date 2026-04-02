package validate

import (
	"bytes"
	stderrors "errors"
	"io"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/slavkluev/ytr/internal/errors"
)

// neverEndingReader is an io.Reader that always returns 'x' bytes.
type neverEndingReader struct{}

func (neverEndingReader) Read(p []byte) (int, error) {
	for i := range p {
		p[i] = 'x'
	}
	return len(p), nil
}

func TestValidateIssueKey(t *testing.T) {
	tests := []struct {
		name    string
		key     string
		wantErr bool
	}{
		{name: "valid simple key", key: "PROJ-123", wantErr: false},
		{name: "valid single digit", key: "PROJ-1", wantErr: false},
		{name: "valid underscore in queue", key: "PROJ_V2-42", wantErr: false},
		{name: "valid large number", key: "ABC-99999", wantErr: false},
		{name: "leading zero", key: "PROJ-0", wantErr: true},
		{name: "leading zero multi digit", key: "PROJ-0123", wantErr: true},
		{name: "lowercase letters", key: "proj-123", wantErr: true},
		{name: "mixed case", key: "Proj-123", wantErr: true},
		{name: "digits first", key: "123-PROJ", wantErr: true},
		{name: "empty string", key: "", wantErr: true},
		{name: "no dash and number", key: "PROJ", wantErr: true},
		{name: "dash without number", key: "PROJ-", wantErr: true},
		{name: "only number", key: "123", wantErr: true},
		{name: "spaces in key", key: "PROJ 123", wantErr: true},
		{name: "double dash", key: "PROJ--123", wantErr: true},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := ValidateIssueKey(tt.key)
			if (err != nil) != tt.wantErr {
				t.Errorf("ValidateIssueKey(%q) error = %v, wantErr %v", tt.key, err, tt.wantErr)
			}
		})
	}
}

func TestValidateNoControlChars(t *testing.T) {
	tests := []struct {
		name      string
		fieldName string
		value     string
		wantErr   bool
		wantRune  string
	}{
		{name: "normal text", fieldName: "summary", value: "hello world", wantErr: false},
		{name: "null byte", fieldName: "summary", value: "hello\x00world", wantErr: true, wantRune: "U+0000"},
		{name: "newline allowed", fieldName: "summary", value: "line1\nline2", wantErr: false},
		{name: "tab allowed", fieldName: "summary", value: "tab\there", wantErr: false},
		{name: "carriage return allowed", fieldName: "summary", value: "cr\rhere", wantErr: false},
		{name: "bell character", fieldName: "summary", value: "bell\x07here", wantErr: true, wantRune: "U+0007"},
		{name: "escape character", fieldName: "description", value: "esc\x1bhere", wantErr: true, wantRune: "U+001B"},
		{name: "backspace", fieldName: "summary", value: "back\x08space", wantErr: true, wantRune: "U+0008"},
		{name: "empty string", fieldName: "summary", value: "", wantErr: false},
		{name: "unicode text", fieldName: "summary", value: "Привет мир", wantErr: false},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := ValidateNoControlChars(tt.fieldName, tt.value)
			if (err != nil) != tt.wantErr {
				t.Errorf("ValidateNoControlChars(%q, %q) error = %v, wantErr %v",
					tt.fieldName, tt.value, err, tt.wantErr)
			}
			if tt.wantErr && err != nil && tt.wantRune != "" {
				if !strings.Contains(err.Error(), tt.wantRune) {
					t.Errorf("error message %q should contain %q", err.Error(), tt.wantRune)
				}
			}
		})
	}
}

func TestParseJSONInput(t *testing.T) {
	t.Run("inline JSON string", func(t *testing.T) {
		input := `{"summary":"test"}`
		data, err := ParseJSONInput(input)
		if err != nil {
			t.Fatalf("ParseJSONInput(%q) unexpected error: %v", input, err)
		}
		if string(data) != input {
			t.Errorf("ParseJSONInput(%q) = %q, want %q", input, string(data), input)
		}
	})

	t.Run("stdin via ParseJSONInputFrom", func(t *testing.T) {
		stdinContent := `{"queue":"PROJ","summary":"from stdin"}`
		reader := bytes.NewBufferString(stdinContent)
		data, err := ParseJSONInputFrom("-", reader)
		if err != nil {
			t.Fatalf("ParseJSONInputFrom(\"-\", ...) unexpected error: %v", err)
		}
		if string(data) != stdinContent {
			t.Errorf("got %q, want %q", string(data), stdinContent)
		}
	})

	t.Run("file via @path", func(t *testing.T) {
		tmpDir := t.TempDir()
		tmpFile := filepath.Join(tmpDir, "input.json")
		content := `{"queue":"PROJ","summary":"from file"}`
		if err := os.WriteFile(tmpFile, []byte(content), 0644); err != nil {
			t.Fatalf("failed to write temp file: %v", err)
		}

		data, err := ParseJSONInput("@" + tmpFile)
		if err != nil {
			t.Fatalf("ParseJSONInput(@file) unexpected error: %v", err)
		}
		if string(data) != content {
			t.Errorf("got %q, want %q", string(data), content)
		}
	})

	t.Run("nonexistent file", func(t *testing.T) {
		_, err := ParseJSONInput("@/tmp/nonexistent_ytr_test_file.json")
		if err == nil {
			t.Fatal("ParseJSONInput(@nonexistent) expected error, got nil")
		}
		if !strings.Contains(err.Error(), "failed to read JSON input") {
			t.Errorf("error %q should contain 'failed to read JSON input'", err.Error())
		}
	})

	t.Run("empty stdin", func(t *testing.T) {
		reader := bytes.NewBufferString("")
		data, err := ParseJSONInputFrom("-", reader)
		if err != nil {
			t.Fatalf("ParseJSONInputFrom(\"-\", empty) unexpected error: %v", err)
		}
		if len(data) != 0 {
			t.Errorf("expected empty data, got %q", string(data))
		}
	})

	t.Run("stdin exceeds size limit", func(t *testing.T) {
		reader := io.LimitReader(neverEndingReader{}, maxJSONInputSize+1)
		_, err := ParseJSONInputFrom("-", reader)
		if err == nil {
			t.Fatal("expected error for oversized stdin, got nil")
		}
		if !strings.Contains(err.Error(), "exceeds maximum size") {
			t.Errorf("error %q should contain 'exceeds maximum size'", err.Error())
		}
	})

	t.Run("stdin at exact size limit", func(t *testing.T) {
		data := make([]byte, maxJSONInputSize)
		for i := range data {
			data[i] = 'x'
		}
		reader := bytes.NewReader(data)
		result, err := ParseJSONInputFrom("-", reader)
		if err != nil {
			t.Fatalf("unexpected error for exactly-at-limit stdin: %v", err)
		}
		if len(result) != maxJSONInputSize {
			t.Errorf("got %d bytes, want %d", len(result), maxJSONInputSize)
		}
	})
}

func TestValidateNumericID(t *testing.T) {
	tests := []struct {
		name      string
		arg       string
		label     string
		wantVal   int
		wantErr   bool
		wantInErr string
	}{
		{name: "valid 42", arg: "42", label: "comment ID", wantVal: 42, wantErr: false},
		{name: "valid 1", arg: "1", label: "link ID", wantVal: 1, wantErr: false},
		{name: "zero", arg: "0", label: "comment ID", wantErr: true, wantInErr: `invalid comment ID "0"`},
		{name: "negative", arg: "-1", label: "comment ID", wantErr: true, wantInErr: `invalid comment ID "-1"`},
		{name: "non-numeric", arg: "abc", label: "comment ID", wantErr: true, wantInErr: `invalid comment ID "abc"`},
		{name: "empty", arg: "", label: "comment ID", wantErr: true, wantInErr: `invalid comment ID ""`},
		{name: "float", arg: "1.5", label: "comment ID", wantErr: true, wantInErr: `invalid comment ID "1.5"`},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			val, err := ValidateNumericID(tt.arg, tt.label)
			if (err != nil) != tt.wantErr {
				t.Errorf("ValidateNumericID(%q, %q) error = %v, wantErr %v",
					tt.arg, tt.label, err, tt.wantErr)
				return
			}
			if !tt.wantErr {
				if val != tt.wantVal {
					t.Errorf("ValidateNumericID(%q, %q) = %d, want %d",
						tt.arg, tt.label, val, tt.wantVal)
				}
				return
			}
			var exitErr *errors.ExitError
			if !stderrors.As(err, &exitErr) {
				t.Errorf("expected *errors.ExitError, got %T", err)
				return
			}
			if exitErr.ExitCode != 1 {
				t.Errorf("expected ExitCode=1, got %d", exitErr.ExitCode)
			}
			if !strings.Contains(err.Error(), tt.wantInErr) {
				t.Errorf("error %q should contain %q", err.Error(), tt.wantInErr)
			}
		})
	}
}

func TestParsePageCursor(t *testing.T) {
	tests := []struct {
		name      string
		cursor    string
		wantPage  int
		wantErr   bool
		wantInErr string
	}{
		{name: "empty cursor uses first page", cursor: "", wantPage: 1},
		{name: "page 2", cursor: "2", wantPage: 2},
		{name: "zero invalid", cursor: "0", wantErr: true, wantInErr: `invalid cursor "0"`},
		{name: "negative invalid", cursor: "-1", wantErr: true, wantInErr: `invalid cursor "-1"`},
		{name: "non numeric invalid", cursor: "abc", wantErr: true, wantInErr: `invalid cursor "abc"`},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			page, err := ParsePageCursor(tt.cursor)
			if (err != nil) != tt.wantErr {
				t.Errorf("ParsePageCursor(%q) error = %v, wantErr %v", tt.cursor, err, tt.wantErr)
				return
			}
			if tt.wantErr {
				if !strings.Contains(err.Error(), tt.wantInErr) {
					t.Errorf("error %q should contain %q", err.Error(), tt.wantInErr)
				}
				return
			}
			if page != tt.wantPage {
				t.Errorf("ParsePageCursor(%q) = %d, want %d", tt.cursor, page, tt.wantPage)
			}
		})
	}
}
