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
			Total:   42,
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

func TestPaginationMeta_OmitsEmpty(t *testing.T) {
	meta := output.PaginationMeta{
		HasMore: false,
		Total:   10,
	}

	data, err := json.Marshal(meta)
	if err != nil {
		t.Fatalf("Marshal() error: %v", err)
	}

	jsonStr := string(data)
	if contains(jsonStr, `"cursor"`) {
		t.Errorf("JSON contains 'cursor' key when empty: %s", jsonStr)
	}
}

func contains(s, substr string) bool {
	for i := 0; i <= len(s)-len(substr); i++ {
		if s[i:i+len(substr)] == substr {
			return true
		}
	}
	return false
}
