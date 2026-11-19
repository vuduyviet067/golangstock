package mapper

import "gostock/internal/dto"

// LogoutMessage is the message of the logout response; it appears both as the
// envelope message and as data.message.
const LogoutMessage = "logged out"

// ToMessageResponse wraps a message as a data payload: {"message": message}.
func ToMessageResponse(message string) dto.MessageResponse {
	return dto.MessageResponse{Message: message}
}

// ToLogoutResponse is the data payload of the logout response:
// {"message":"logged out"}.
func ToLogoutResponse() dto.MessageResponse {
	return ToMessageResponse(LogoutMessage)
}
