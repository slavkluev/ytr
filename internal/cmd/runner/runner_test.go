package runner

import (
	"reflect"
	"slices"
	"testing"

	"github.com/slavkluev/ytr/internal/output"
)

func TestJSONFieldsAreTheNamesFilterFieldsSelects(t *testing.T) {
	type probe struct {
		ID       string `json:"id"`
		Hidden   string `json:"-"`
		Untagged string
		Optional string `json:"optional,omitempty"`
		Bare     string `json:",omitempty"`
	}

	fields := jsonFields(reflect.TypeFor[probe]())

	if want := []string{"id", "optional", "Bare"}; !slices.Equal(fields, want) {
		t.Errorf("fields = %q, want %q", fields, want)
	}

	selected := output.FilterFields(probe{"1", "h", "u", "o", "b"}, fields)
	if len(selected) != len(fields) {
		t.Errorf("FilterFields selected %v for fields %q, want every one of them", selected, fields)
	}
}
