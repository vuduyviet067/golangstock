package mapper

import (
	"gostock/internal/domain"
	"gostock/internal/dto"
)

// ToUserResponse maps a user to the user management body. The password hash
// is never copied. Timestamps are copied unchanged, so JSON renders them as
// RFC3339Nano and the zero time as "0001-01-01T00:00:00Z".
func ToUserResponse(u domain.User) dto.UserResponse {
	return dto.UserResponse{
		ID:        u.ID,
		Email:     u.Email,
		Role:      u.Role,
		CreatedAt: u.CreatedAt,
		UpdatedAt: u.UpdatedAt,
	}
}

// ToUserResponses maps a page of users, keeping their order. It never returns
// nil, so an empty or nil page is serialized as [].
func ToUserResponses(users []domain.User) []dto.UserResponse {
	items := make([]dto.UserResponse, len(users))
	for i, u := range users {
		items[i] = ToUserResponse(u)
	}
	return items
}
