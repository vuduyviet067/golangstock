package dto

import "time"

// UserResponse represents a user in the user-management API responses.
type UserResponse struct {
	ID        int64     `json:"id"`
	Email     string    `json:"email"`
	Role      string    `json:"role"`
	CreatedAt time.Time `json:"created_at"`
	UpdatedAt time.Time `json:"updated_at"`
}

// UpdateUserRequest represents the PATCH /api/v1/users/{id} body.
type UpdateUserRequest struct {
	Email *string `json:"email"`
	Role  *string `json:"role"`
}

// UpdateUserInput carries the optional fields of a user update; a nil field
// is left unchanged.
type UpdateUserInput struct {
	Email *string
	Role  *string
}
