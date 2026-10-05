package output

// PaginatedResult is the envelope every list command prints:
// {"items": [...], "pagination": {"cursor": "...", "hasMore": true, "total": N}}.
type PaginatedResult struct {
	// Items contains the list of results.
	Items any `json:"items"`

	// Pagination contains cursor, hasMore, and total metadata.
	Pagination PaginationMeta `json:"pagination"`
}

// PaginationMeta is the pagination of an envelope. Every key is present on
// every page, an empty or last one included, so a reader never has to tell a
// missing key from a value.
type PaginationMeta struct {
	// Cursor is the value --cursor takes to fetch the next page, "" when there
	// is none.
	Cursor string `json:"cursor"`

	// HasMore indicates whether more results are available.
	HasMore bool `json:"hasMore"`

	// Total is the number of items in the whole list: what Tracker counted for
	// a page, the item count for a list fetched whole. It is nil, printed as
	// null, only for a page of a list Tracker sends no count for.
	Total *int `json:"total"`
}

// WholeList is the pagination of a list fetched to its end, which holds count
// items: no next page, and no cursor to fetch one.
func WholeList(count int) PaginationMeta {
	return PaginationMeta{Total: &count}
}
