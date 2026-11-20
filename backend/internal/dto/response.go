package dto

// MessageResponse is a data payload that carries only a message, such as the
// logout response {"message":"logged out"}.
type MessageResponse struct {
	Message string `json:"message"`
}
