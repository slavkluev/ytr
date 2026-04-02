// Package validate provides input validation functions for the ytr CLI.
package validate

import (
	"fmt"
	"io"
	"os"
	"regexp"
	"strconv"
	"strings"

	"github.com/slavkluev/ytr/internal/errors"
)

// maxJSONInputSize is the maximum allowed size for JSON input read from stdin.
// Protects against unbounded memory consumption from piped input.
const maxJSONInputSize = 10 << 20 // 10 MB

// issueKeyRegexp matches a valid Yandex Tracker issue key: uppercase letters
// (with optional underscores/digits) followed by a dash and a positive number.
var issueKeyRegexp = regexp.MustCompile(`^[A-Z][A-Z0-9_]+-[1-9][0-9]*$`)

// ValidateIssueKey validates that key matches the Yandex Tracker issue key format.
// Valid keys: PROJ-123, PROJ_V2-42. Invalid: proj-123, PROJ-0, PROJ, empty.
func ValidateIssueKey(key string) error {
	if !issueKeyRegexp.MatchString(key) {
		return errors.NewUserError(
			fmt.Sprintf("invalid issue key %q: expected format QUEUE-123", key),
			"Issue keys must be uppercase letters followed by a dash and a number (e.g., PROJ-42)",
		)
	}
	return nil
}

// ValidateNoControlChars checks that value contains no control characters
// except tab (0x09), newline (0x0A), and carriage return (0x0D).
// Returns a structured error identifying the offending character and position.
func ValidateNoControlChars(fieldName, value string) error {
	for i, r := range value {
		if r < 0x20 && r != '\t' && r != '\n' && r != '\r' {
			return errors.NewUserError(
				fmt.Sprintf("control character U+%04X at position %d in %s", r, i, fieldName),
				"Remove control characters from the input",
			)
		}
	}
	return nil
}

// ParseJSONInput reads JSON data from one of three sources:
//   - "-" reads from os.Stdin.
//   - "@path" reads from the file at path.
//   - anything else is treated as inline JSON.
func ParseJSONInput(value string) ([]byte, error) {
	return ParseJSONInputFrom(value, os.Stdin)
}

// ParseJSONInputFrom reads JSON data from the given source, using the provided
// reader for stdin. This variant enables testing without replacing os.Stdin.
func ParseJSONInputFrom(value string, stdin io.Reader) ([]byte, error) {
	switch {
	case value == "-":
		data, err := io.ReadAll(io.LimitReader(stdin, maxJSONInputSize+1))
		if err != nil {
			return nil, fmt.Errorf("failed to read JSON input: %w", err)
		}
		if len(data) > maxJSONInputSize {
			return nil, errors.NewUserError(
				"JSON input exceeds maximum size (10 MB)",
				"Use @file.json for large inputs instead of stdin",
			)
		}
		return data, nil

	case strings.HasPrefix(value, "@"):
		path := value[1:]
		data, err := os.ReadFile(path)
		if err != nil {
			return nil, fmt.Errorf("failed to read JSON input: %w", err)
		}
		return data, nil

	default:
		return []byte(value), nil
	}
}

// ValidateStringID checks that arg is a non-empty string ID.
// Returns the trimmed value and nil on success, or an ExitError with
// a descriptive message using label (e.g., "link ID").
func ValidateStringID(arg, label string) (string, error) {
	trimmed := strings.TrimSpace(arg)
	if trimmed == "" {
		return "", errors.NewUserError(
			fmt.Sprintf("invalid %s: expected a non-empty value", label),
			"Provide a valid "+label,
		)
	}
	return trimmed, nil
}

// ValidateNumericID parses arg as a positive integer.
// Returns the parsed value and nil on success, or an ExitError with
// a descriptive message using label (e.g., "comment ID").
func ValidateNumericID(arg, label string) (int, error) {
	n, err := strconv.Atoi(arg)
	if err != nil || n < 1 {
		return 0, errors.NewUserError(
			fmt.Sprintf("invalid %s %q: expected a positive integer", label, arg),
			fmt.Sprintf("Use a numeric %s (e.g., 42)", label),
		)
	}
	return n, nil
}

// ParsePageCursor parses cursor as a positive 1-based page number.
// Empty cursor means the first page.
func ParsePageCursor(cursor string) (int, error) {
	if cursor == "" {
		return 1, nil
	}

	page, err := strconv.Atoi(cursor)
	if err != nil || page < 1 {
		return 0, errors.NewUserError(
			fmt.Sprintf("invalid cursor %q: expected a positive page number", cursor),
			"Use --cursor 1, --cursor 2, and so on",
		)
	}

	return page, nil
}
