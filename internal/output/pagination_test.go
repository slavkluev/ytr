package output_test

import (
	"encoding/json"
	"testing"

	"github.com/slavkluev/ytr/internal/output"
)

func TestPaginatedResult_JSON(t *testing.T) {
	result := output.PaginatedResult{
		Items: []string{"a", "b"},
		Pagination: output.PaginationMeta{
			Cursor:  "cursor123",
			HasMore: true,
			Total:   new(42),
		},
	}

	data, err := json.Marshal(result)
	if err != nil {
		t.Fatalf("Marshal() error: %v", err)
	}

	var decoded map[string]json.RawMessage
	if err := json.Unmarshal(data, &decoded); err != nil {
		t.Fatalf("Unmarshal() error: %v", err)
	}

	if _, ok := decoded["items"]; !ok {
		t.Error("JSON missing 'items' key")
	}
	if _, ok := decoded["pagination"]; !ok {
		t.Error("JSON missing 'pagination' key")
	}
}

func TestPaginationMeta_KeepsEveryKey(t *testing.T) {
	tests := []struct {
		name string
		meta output.PaginationMeta
		want string
	}{
		{"whole list", output.WholeList(3), `{"cursor":"","hasMore":false,"total":3}`},
		{"empty whole list", output.WholeList(0), `{"cursor":"","hasMore":false,"total":0}`},
		{
			"uncounted page",
			output.PaginationMeta{Cursor: "c", HasMore: true},
			`{"cursor":"c","hasMore":true,"total":null}`,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			data, err := json.Marshal(tt.meta)
			if err != nil {
				t.Fatalf("Marshal() error: %v", err)
			}
			if string(data) != tt.want {
				t.Errorf("JSON = %s, want %s", data, tt.want)
			}
		})
	}
}
