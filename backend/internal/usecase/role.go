package usecase

import "gostock/internal/repository"

const (
	RoleUser  = "user"
	RoleAdmin = repository.RoleAdmin
)

func IsValidRole(role string) bool {
	return role == RoleUser || role == RoleAdmin
}
