package dto

type LoginRequest struct {
	Email    string `json:"email"`
	Password string `json:"password"`
}

type RegisterRequest struct {
	Email    string `json:"email"`
	Password string `json:"password"`
}

type AuthUserResponse struct {
	ID        int64  `json:"id"`
	Email     string `json:"email"`
	Role      string `json:"role"`
	CreatedAt string `json:"created_at"`
}

// LoginResult is the outcome of a login or a refresh. User is the response
// body; the tokens are for the auth cookies only (whose lifetimes come from
// the cookie configuration) and never appear in a response body.
type LoginResult struct {
	User         AuthUserResponse
	AccessToken  string
	RefreshToken string
}
