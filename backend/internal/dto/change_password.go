package dto

// ChangePasswordRequest is the body of POST /api/v1/auth/change-password.
type ChangePasswordRequest struct {
	CurrentPassword string `json:"current_password"`
	NewPassword     string `json:"new_password"`
}
