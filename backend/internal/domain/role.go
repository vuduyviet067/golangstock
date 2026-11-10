package domain

// User roles. These values are stored in users.role (constrained by
// users_role_check), carried in JWT claims, and returned by the API.
const (
	RoleUser  = "user"
	RoleAdmin = "admin"
)

// IsValidRole reports whether role is exactly one of the defined roles. It
// does not trim or change case; callers normalize input first if needed.
func IsValidRole(role string) bool {
	return role == RoleUser || role == RoleAdmin
}
