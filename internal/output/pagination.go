package output

// PaginatedResult wraps list command results with pagination metadata
// for JSON output per OUT-08. The envelope format is:
// {"items": [...], "pagination": {"cursor": "...", "hasMore": true, "total": N}}.
type PaginatedResult struct {
	// Items contains the list of results.
	Items any `json:"items"`

	// Pagination contains cursor, hasMore, and total metadata.
	Pagination PaginationMeta `json:"pagination"`
}

// PaginationMeta holds pagination state for list commands.
// Cursor is omitted from JSON when empty (first page or no cursor-based pagination).
// Total is omitted when zero (API did not provide a count).
type PaginationMeta struct {
	// Cursor is the opaque pagination token for the next page.
	Cursor string `json:"cursor,omitempty"`

	// HasMore indicates whether more results are available.
	HasMore bool `json:"hasMore"`

	// Total is the total number of results, if known.
	Total int `json:"total,omitempty"`
}
