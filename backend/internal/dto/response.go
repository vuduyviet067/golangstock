package dto

// APIResponse represents the standard unified API response envelope.
type APIResponse struct {
	Success bool   `json:"success"`
	Message string `json:"message"`
	Data    any    `json:"data"`
}

// PaginatedData defines the standard envelope payload for paginated lists inside the data field.
type PaginatedData struct {
	Items      any            `json:"items"`
	Pagination PaginationMeta `json:"pagination"`
}

// PaginationMeta contains pagination metadata.
type PaginationMeta struct {
	Page       int   `json:"page"`
	Limit      int   `json:"limit"`
	Total      int64 `json:"total"`
	TotalPages int   `json:"total_pages"`
}
