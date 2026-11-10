package jwt

import (
	"errors"
	"fmt"
	"strconv"
	"time"

	"gostock/internal/domain"
	"gostock/internal/dto"
	"gostock/internal/usecase"

	"github.com/golang-jwt/jwt/v5"
)

var (
	ErrInvalidSecret    = errors.New("jwt secret must be at least 32 bytes")
	ErrInvalidToken     = errors.New("invalid token")
	ErrExpiredToken     = errors.New("token has expired")
	ErrInvalidSignature = errors.New("invalid token signature")
	ErrInvalidAlgorithm = errors.New("invalid signing algorithm")
	ErrInvalidIssuer    = errors.New("invalid token issuer")
	ErrInvalidAudience  = errors.New("invalid token audience")
	ErrInvalidSubject   = errors.New("invalid token subject")
	ErrInvalidRole      = errors.New("invalid user role in token")
)

type customClaims struct {
	Email string `json:"email"`
	Role  string `json:"role"`
	jwt.RegisteredClaims
}

type JWTService struct {
	secret   []byte
	issuer   string
	audience string
	ttl      time.Duration
}

func NewJWTService(secret, issuer, audience string, ttl time.Duration) (*JWTService, error) {
	if len(secret) < 32 {
		return nil, ErrInvalidSecret
	}
	if ttl <= 0 {
		return nil, errors.New("jwt ttl must be positive")
	}

	return &JWTService{
		secret:   []byte(secret),
		issuer:   issuer,
		audience: audience,
		ttl:      ttl,
	}, nil
}

func (s *JWTService) GenerateAccessToken(userID int64, email, role string) (string, time.Time, error) {
	if userID <= 0 {
		return "", time.Time{}, ErrInvalidSubject
	}
	if !domain.IsValidRole(role) {
		return "", time.Time{}, ErrInvalidRole
	}

	now := time.Now()
	expiresAt := now.Add(s.ttl)

	claims := customClaims{
		Email: email,
		Role:  role,
		RegisteredClaims: jwt.RegisteredClaims{
			Subject:   strconv.FormatInt(userID, 10),
			Issuer:    s.issuer,
			Audience:  jwt.ClaimStrings{s.audience},
			IssuedAt:  jwt.NewNumericDate(now),
			ExpiresAt: jwt.NewNumericDate(expiresAt),
		},
	}

	token := jwt.NewWithClaims(jwt.SigningMethodHS256, claims)
	signedToken, err := token.SignedString(s.secret)
	if err != nil {
		return "", time.Time{}, fmt.Errorf("sign jwt: %w", err)
	}

	return signedToken, expiresAt, nil
}

func (s *JWTService) ValidateAccessToken(tokenStr string) (*dto.TokenClaims, error) {
	if tokenStr == "" {
		return nil, ErrInvalidToken
	}

	token, err := jwt.ParseWithClaims(
		tokenStr,
		&customClaims{},
		func(t *jwt.Token) (interface{}, error) {
			// Enforce HS256 strictly. Reject 'none', RSA, ECDSA, etc.
			if t.Method == nil || t.Method.Alg() != jwt.SigningMethodHS256.Alg() {
				return nil, fmt.Errorf("%w: expected HS256, got %v", ErrInvalidAlgorithm, t.Header["alg"])
			}
			return s.secret, nil
		},
		jwt.WithIssuer(s.issuer),
		jwt.WithAudience(s.audience),
		jwt.WithExpirationRequired(),
	)

	if err != nil {
		switch {
		case errors.Is(err, jwt.ErrTokenExpired):
			return nil, ErrExpiredToken
		case errors.Is(err, jwt.ErrSignatureInvalid):
			return nil, ErrInvalidSignature
		case errors.Is(err, jwt.ErrTokenInvalidIssuer):
			return nil, ErrInvalidIssuer
		case errors.Is(err, jwt.ErrTokenInvalidAudience):
			return nil, ErrInvalidAudience
		case errors.Is(err, ErrInvalidAlgorithm):
			return nil, ErrInvalidAlgorithm
		default:
			return nil, fmt.Errorf("%w: %v", ErrInvalidToken, err)
		}
	}

	claims, ok := token.Claims.(*customClaims)
	if !ok || !token.Valid {
		return nil, ErrInvalidToken
	}

	userID, err := strconv.ParseInt(claims.Subject, 10, 64)
	if err != nil || userID <= 0 {
		return nil, ErrInvalidSubject
	}

	if !domain.IsValidRole(claims.Role) {
		return nil, ErrInvalidRole
	}

	return &dto.TokenClaims{
		UserID: userID,
		Email:  claims.Email,
		Role:   claims.Role,
	}, nil
}

var _ usecase.TokenManager = (*JWTService)(nil)
