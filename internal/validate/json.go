package validate

import (
	"encoding/json"
	stderrors "errors"
	"fmt"
	"reflect"
	"slices"
	"strings"

	"github.com/slavkluev/ytr/internal/errors"
)

// UnmarshalStrict decodes data into v and fails when the input carries keys
// that v has no field for. Plain json.Unmarshal drops such keys silently,
// which turns a typo or an unsupported field into an empty request body the
// API happily accepts.
//
// Only top-level keys are checked: values are handed to json.Unmarshal
// untouched, so custom UnmarshalJSON implementations keep working, and keys
// nested inside objects are not inspected.
func UnmarshalStrict(data []byte, v any) error {
	var raw map[string]json.RawMessage
	if err := json.Unmarshal(data, &raw); err != nil {
		// Not a JSON object: let json.Unmarshal report the error it always has.
		return json.Unmarshal(data, v)
	}

	known, ok := jsonFieldNames(v)
	if !ok {
		// Not a struct: the target accepts any key, nothing to check.
		return json.Unmarshal(data, v)
	}

	var unknown []string
	for key := range raw {
		if !slices.ContainsFunc(known, func(name string) bool {
			return strings.EqualFold(name, key)
		}) {
			unknown = append(unknown, key)
		}
	}

	if len(unknown) > 0 {
		slices.Sort(unknown)
		return errors.NewUnknownFieldsError(unknown, known)
	}

	return json.Unmarshal(data, v)
}

// jsonFieldNames returns the JSON key of every exported field of v, sorted,
// and reports whether v is a struct at all. Callers compare against the names
// with strings.EqualFold, the way encoding/json matches keys to fields.
// Fields tagged json:"-" are skipped; untagged fields keep their Go name,
// matching encoding/json.
func jsonFieldNames(v any) ([]string, bool) {
	t := reflect.TypeOf(v)
	for t != nil && t.Kind() == reflect.Pointer {
		t = t.Elem()
	}
	if t == nil || t.Kind() != reflect.Struct {
		return nil, false
	}

	names := make([]string, 0, t.NumField())
	for field := range t.Fields() {
		if !field.IsExported() {
			continue
		}

		name, _, _ := strings.Cut(field.Tag.Get("json"), ",")
		switch name {
		case "-":
			continue
		case "":
			name = field.Name
		}
		names = append(names, name)
	}

	slices.Sort(names)
	return names, true
}

// UnmarshalRequestJSON decodes a --from-json request body into v, rejecting
// unknown keys and reporting a malformed body as a user error that names the
// expected request type.
func UnmarshalRequestJSON(data []byte, v any) error {
	err := UnmarshalStrict(data, v)
	if err == nil {
		return nil
	}

	var fieldErr *errors.InvalidFieldError
	if stderrors.As(err, &fieldErr) {
		return err
	}

	return errors.NewUserError(
		fmt.Sprintf("invalid JSON input: %s", err),
		fmt.Sprintf("Provide valid JSON matching the %s format", requestTypeName(v)),
	)
}

// requestTypeName returns the Go type name behind v, used to point the user at
// the request shape the input must match.
func requestTypeName(v any) string {
	t := reflect.TypeOf(v)
	for t != nil && t.Kind() == reflect.Pointer {
		t = t.Elem()
	}
	if t == nil {
		return "request"
	}
	return t.Name()
}
