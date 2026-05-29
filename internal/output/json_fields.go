package output

import (
	"encoding/json"
	"fmt"
	"io"
	"reflect"
	"slices"
	"strings"

	"github.com/itchyny/gojq"

	"github.com/slavkluev/ytr/internal/errors"
)

// ValidateFields checks requested fields against allowed fields.
// Case-insensitive comparison.
func ValidateFields(requested, allowed []string) error {
	for _, f := range requested {
		found := false
		for _, a := range allowed {
			if strings.EqualFold(f, a) {
				found = true
				break
			}
		}
		if !found {
			return errors.NewInvalidFieldError(f, allowed)
		}
	}
	return nil
}

// NormalizeFields maps requested field names to their canonical (json tag) form.
// Assumes ValidateFields has already passed.
func NormalizeFields(requested, allowed []string) []string {
	result := make([]string, len(requested))
	for i, f := range requested {
		for _, a := range allowed {
			if strings.EqualFold(f, a) {
				result[i] = a
				break
			}
		}
	}
	return result
}

// FilterFields extracts only the requested fields from a struct using json tags.
//
// Precondition: data must be a flat struct (or pointer to one) whose exported
// fields carry json tags — the item structs each command builds for output. A
// pointer is dereferenced; a non-struct yields an empty result rather than a
// reflect panic.
//
// `omitempty` is honored so the filtered view matches the full-JSON contract:
// an empty omitempty field is dropped (not emitted as "" or null), letting
// agents distinguish "absent" from a real empty/null value.
func FilterFields(data any, fields []string) map[string]any {
	result := make(map[string]any, len(fields))

	v := reflect.Indirect(reflect.ValueOf(data))
	if v.Kind() != reflect.Struct {
		return result
	}
	t := v.Type()

	requested := make(map[string]bool, len(fields))
	for _, f := range fields {
		requested[f] = true
	}

	for i := range t.NumField() {
		field := t.Field(i)
		jsonTag := field.Tag.Get("json")
		if jsonTag == "" || jsonTag == "-" {
			continue
		}
		parts := strings.Split(jsonTag, ",")
		name := parts[0]
		if name == "" {
			name = field.Name
		}
		if !requested[name] {
			continue
		}
		fv := v.Field(i)
		if hasOmitempty(parts[1:]) && isEmptyValue(fv) {
			continue
		}
		result[name] = fv.Interface()
	}
	return result
}

// hasOmitempty reports whether the json tag options contain "omitempty".
func hasOmitempty(opts []string) bool {
	return slices.Contains(opts, "omitempty")
}

// isEmptyValue mirrors encoding/json's emptiness test used for omitempty, so
// FilterFields drops exactly the values the full json.Marshal would.
func isEmptyValue(v reflect.Value) bool {
	switch v.Kind() { //nolint:exhaustive // mirrors encoding/json: unlisted kinds are never empty
	case reflect.Array, reflect.Map, reflect.Slice, reflect.String:
		return v.Len() == 0
	case reflect.Bool:
		return !v.Bool()
	case reflect.Int, reflect.Int8, reflect.Int16, reflect.Int32, reflect.Int64:
		return v.Int() == 0
	case reflect.Uint, reflect.Uint8, reflect.Uint16, reflect.Uint32, reflect.Uint64, reflect.Uintptr:
		return v.Uint() == 0
	case reflect.Float32, reflect.Float64:
		return v.Float() == 0
	case reflect.Interface, reflect.Pointer:
		return v.IsNil()
	}
	return false
}

// PrintFieldHint outputs available fields as a formatted list to the writer
// and returns an ExitError with exit code 1.
func PrintFieldHint(w io.Writer, commandName string, fields []string) error {
	_, _ = fmt.Fprintf(w, "Specify one or more comma-separated field names for JSON output.\n\n")
	_, _ = fmt.Fprintf(w, "Available fields for %s:\n", commandName)
	for _, f := range fields {
		_, _ = fmt.Fprintf(w, "  %s\n", f)
	}
	return &errors.ExitError{
		ExitCode: errors.ExitUserError,
		Code:     "field_hint",
		Message:  "no fields specified",
	}
}

// ApplyJQ parses and executes a jq expression against JSON data.
// Input data is marshaled to JSON then unmarshaled to any (gojq requires
// map[string]any / []any, not custom structs).
// String results are printed without quotes (raw output, like jq -r).
// Non-string results are JSON-encoded.
func ApplyJQ(w io.Writer, data any, expression string) error {
	jsonBytes, err := json.Marshal(data)
	if err != nil {
		return fmt.Errorf("failed to marshal data for jq: %w", err)
	}
	var input any
	if unmarshalErr := json.Unmarshal(jsonBytes, &input); unmarshalErr != nil {
		return fmt.Errorf("failed to prepare data for jq: %w", unmarshalErr)
	}

	query, err := gojq.Parse(expression)
	if err != nil {
		return errors.NewUserError(
			fmt.Sprintf("invalid jq expression: %s", err),
			"Check jq syntax. Example: --jq '.[] | .key'",
		)
	}

	code, err := gojq.Compile(query)
	if err != nil {
		return errors.NewUserError(
			fmt.Sprintf("failed to compile jq expression: %s", err),
			"Simplify the jq expression and retry",
		)
	}

	iter := code.Run(input)
	for {
		v, ok := iter.Next()
		if !ok {
			break
		}
		if err, ok := v.(error); ok {
			return errors.NewUserError(
				fmt.Sprintf("jq execution error: %s", err),
				"Check that the jq expression matches the data structure",
			)
		}
		// Raw string output (like jq -r): print strings without quotes
		if s, ok := v.(string); ok {
			_, _ = fmt.Fprintln(w, s)
		} else {
			jsonOut, err := json.Marshal(v)
			if err != nil {
				return fmt.Errorf("failed to marshal jq result: %w", err)
			}
			_, _ = fmt.Fprintln(w, string(jsonOut))
		}
	}
	return nil
}
