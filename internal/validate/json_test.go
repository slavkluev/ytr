package validate

import (
	stderrors "errors"
	"slices"
	"strings"
	"testing"

	"github.com/slavkluev/ytr/internal/errors"
)

// strictReq mirrors the shape of a tracker request body: a flat struct with
// json tags and no free-form container.
type strictReq struct {
	Summary *string `json:"summary,omitempty"`
	Queue   *string `json:"queue,omitempty"`
}

func TestUnmarshalStrictRejectsUnknownFields(t *testing.T) {
	var got strictReq

	err := UnmarshalStrict([]byte(`{"queue":"PROJ","size":["L"],"bogus":1}`), &got)

	if err == nil {
		t.Fatal("expected an error for unknown fields, got nil")
	}
	for _, field := range []string{"bogus", "size"} {
		if !strings.Contains(err.Error(), field) {
			t.Errorf("error must name unknown field %q, got %q", field, err.Error())
		}
	}
}

func TestUnmarshalStrictMatchesFieldNamesCaseInsensitively(t *testing.T) {
	var got strictReq

	// encoding/json matches keys case-insensitively, so input accepted today
	// must keep working.
	if err := UnmarshalStrict([]byte(`{"SUMMARY":"caseless"}`), &got); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if got.Summary == nil || *got.Summary != "caseless" {
		t.Errorf("summary = %v, want %q", got.Summary, "caseless")
	}
}

func TestUnmarshalStrictReportsUnknownFieldsMachineReadably(t *testing.T) {
	var got strictReq

	err := UnmarshalStrict([]byte(`{"size":["L"],"bogus":1}`), &got)

	var fieldErr *errors.InvalidFieldError
	if !stderrors.As(err, &fieldErr) {
		t.Fatalf("error type = %T, want *errors.InvalidFieldError", err)
	}
	if !slices.Equal(fieldErr.InvalidFields, []string{"bogus", "size"}) {
		t.Errorf("InvalidFields = %v, want [bogus size]", fieldErr.InvalidFields)
	}
	if !slices.Equal(fieldErr.ValidFields, []string{"queue", "summary"}) {
		t.Errorf("ValidFields = %v, want [queue summary]", fieldErr.ValidFields)
	}
}

func TestUnmarshalStrictSkipsCheckForNonStructTargets(t *testing.T) {
	got := map[string]any{}

	// A map accepts any key, so there is nothing to call unknown.
	if err := UnmarshalStrict([]byte(`{"anything":1}`), &got); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if got["anything"] != float64(1) {
		t.Errorf("decoded map = %v, want key anything", got)
	}
}

// TestUnmarshalStrictPreservesUnmarshalBehavior guards the behaviour inherited
// from json.Unmarshal: everything the callers relied on before the strict
// check must still hold.
func TestUnmarshalStrictPreservesUnmarshalBehavior(t *testing.T) {
	tests := []struct {
		name    string
		input   string
		wantErr bool
	}{
		{name: "known fields", input: `{"queue":"PROJ","summary":"hi"}`, wantErr: false},
		{name: "empty object", input: `{}`, wantErr: false},
		{name: "null", input: `null`, wantErr: false},
		{name: "malformed json", input: `{"queue":`, wantErr: true},
		{name: "array instead of object", input: `[{"queue":"PROJ"}]`, wantErr: true},
		{name: "second object after the first", input: `{"queue":"A"} {"queue":"B"}`, wantErr: true},
		{name: "wrong value type", input: `{"queue":42}`, wantErr: true},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			var got strictReq

			err := UnmarshalStrict([]byte(tt.input), &got)

			if (err != nil) != tt.wantErr {
				t.Errorf("UnmarshalStrict(%s) error = %v, wantErr %v", tt.input, err, tt.wantErr)
			}
		})
	}
}

func TestUnmarshalStrictRejectsFieldsExcludedFromJSON(t *testing.T) {
	var got struct {
		Summary *string `json:"summary,omitempty"`
		Secret  string  `json:"-"`
	}

	// json:"-" means the key can never be set, so accepting it would drop it
	// silently — the very failure this helper exists to prevent.
	err := UnmarshalStrict([]byte(`{"Secret":"x"}`), &got)

	if err == nil {
		t.Fatal("expected an error for a field excluded from JSON, got nil")
	}
}

func TestUnmarshalRequestJSONWrapsFormatErrors(t *testing.T) {
	var got strictReq

	err := UnmarshalRequestJSON([]byte(`{"queue":`), &got)

	var exitErr *errors.ExitError
	if !stderrors.As(err, &exitErr) {
		t.Fatalf("error type = %T, want *errors.ExitError", err)
	}
	if !strings.Contains(exitErr.Message, "invalid JSON input") {
		t.Errorf("Message = %q, want it to report invalid JSON input", exitErr.Message)
	}
	if !strings.Contains(exitErr.Suggestion, "strictReq") {
		t.Errorf("Suggestion = %q, want it to name the request type", exitErr.Suggestion)
	}
}

func TestUnmarshalRequestJSONKeepsUnknownFieldsError(t *testing.T) {
	var got strictReq

	err := UnmarshalRequestJSON([]byte(`{"size":["L"]}`), &got)

	var fieldErr *errors.InvalidFieldError
	if !stderrors.As(err, &fieldErr) {
		t.Fatalf("error type = %T, want *errors.InvalidFieldError", err)
	}
	if !slices.Equal(fieldErr.InvalidFields, []string{"size"}) {
		t.Errorf("InvalidFields = %v, want [size]", fieldErr.InvalidFields)
	}
}
