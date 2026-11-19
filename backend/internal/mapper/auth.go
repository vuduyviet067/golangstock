package mapper

import (
	"time"

	"gostock/internal/domain"
	"gostock/internal/dto"
)

// ToAuthUserResponse maps a user to the body returned by register, login,
// refresh and me. The password hash is never copied.
//
// created_at is formatted with time.RFC3339 (seconds, the value's own offset).
// The zero time becomes "": /auth/me builds its user from JWT claims, which
// carry no creation time.
func ToAuthUserResponse(u domain.User) dto.AuthUserResponse {
	return dto.AuthUserResponse{
		ID:        u.ID,
		Email:     u.Email,
		Role:      u.Role,
		CreatedAt: formatAuthTime(u.CreatedAt),
	}
}

func formatAuthTime(t time.Time) string {
	if t.IsZero() {
		return ""
	}
	return t.Format(time.RFC3339)
}
