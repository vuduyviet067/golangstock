package dto

import "time"

type UserResponse struct {
	ID        int64     `json:"id"`
	Email     string    `json:"email"`
	Role      string    `json:"role"`
	CreatedAt time.Time `json:"created_at"`
	UpdatedAt time.Time `json:"updated_at"`
}

type UpdateUserRequest struct {
	Email *string `json:"email"`
	Role  *string `json:"role"`
}

type UpdateUserInput struct {
	Email *string
	Role  *string
}
