package dto

// TokenClaims represents the payload decoded from a valid JWT Access Token.
type TokenClaims struct {
	UserID int64
	Email  string
	Role   string
}
