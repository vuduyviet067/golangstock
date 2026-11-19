package dto

type APIResponse struct {
	Success bool   `json:"success"`
	Message string `json:"message"`
	Data    any    `json:"data"`
}

// MessageResponse is a data payload that carries only a message, such as the
// logout response {"message":"logged out"}.
type MessageResponse struct {
	Message string `json:"message"`
}

type PaginatedData struct {
	Items      any            `json:"items"`
	Pagination PaginationMeta `json:"pagination"`
}

type PaginationMeta struct {
	Page       int   `json:"page"`
	Limit      int   `json:"limit"`
	Total      int64 `json:"total"`
	TotalPages int   `json:"total_pages"`
}
