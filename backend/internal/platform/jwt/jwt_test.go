package jwt

import (
	"errors"
	"testing"
	"time"

	"github.com/golang-jwt/jwt/v5"
)

const (
	testSecret   = "super-secret-key-that-is-at-least-32-bytes-long!"
	testIssuer   = "test-gostock"
	testAudience = "test-gostock-api"
)

func TestNewJWTService_SecretLength(t *testing.T) {
	_, err := NewJWTService("short-secret", testIssuer, testAudience, 15*time.Minute)
	if !errors.Is(err, ErrInvalidSecret) {
		t.Fatalf("expected ErrInvalidSecret, got %v", err)
	}

	svc, err := NewJWTService(testSecret, testIssuer, testAudience, 15*time.Minute)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if svc == nil {
		t.Fatal("expected non-nil service")
	}
}

func TestJWTService_GenerateAndValidateValidToken(t *testing.T) {
	svc, err := NewJWTService(testSecret, testIssuer, testAudience, 15*time.Minute)
	if err != nil {
		t.Fatalf("init service: %v", err)
	}

	tokenStr, expiresAt, err := svc.GenerateAccessToken(42, "user@example.com", "user")
	if err != nil {
		t.Fatalf("GenerateAccessToken() error = %v", err)
	}
	if tokenStr == "" {
		t.Fatal("expected non-empty token string")
	}
	if !expiresAt.After(time.Now()) {
		t.Fatalf("expected expiresAt in the future, got %v", expiresAt)
	}

	claims, err := svc.ValidateAccessToken(tokenStr)
	if err != nil {
		t.Fatalf("ValidateAccessToken() error = %v", err)
	}
	if claims == nil {
		t.Fatal("expected non-nil claims")
	}
	if claims.UserID != 42 {
		t.Errorf("UserID = %d, want 42", claims.UserID)
	}
	if claims.Email != "user@example.com" {
		t.Errorf("Email = %q, want user@example.com", claims.Email)
	}
	if claims.Role != "user" {
		t.Errorf("Role = %q, want user", claims.Role)
	}
}

func TestJWTService_ValidateExpiredToken(t *testing.T) {
	// Create service with negative TTL to simulate immediate expiration
	svc, err := NewJWTService(testSecret, testIssuer, testAudience, time.Millisecond)
	if err != nil {
		t.Fatalf("init service: %v", err)
	}

	tokenStr, _, err := svc.GenerateAccessToken(42, "user@example.com", "user")
	if err != nil {
		t.Fatalf("GenerateAccessToken() error = %v", err)
	}

	// Wait for token to expire
	time.Sleep(10 * time.Millisecond)

	_, err = svc.ValidateAccessToken(tokenStr)
	if !errors.Is(err, ErrExpiredToken) {
		t.Fatalf("ValidateAccessToken() error = %v, want ErrExpiredToken", err)
	}
}

func TestJWTService_ValidateInvalidSignature(t *testing.T) {
	svc1, _ := NewJWTService(testSecret, testIssuer, testAudience, 15*time.Minute)
	svc2, _ := NewJWTService("another-secret-key-that-is-at-least-32-bytes-long!", testIssuer, testAudience, 15*time.Minute)

	tokenStr, _, err := svc1.GenerateAccessToken(1, "viet@example.com", "admin")
	if err != nil {
		t.Fatalf("GenerateAccessToken() error = %v", err)
	}

	// Validating with svc2 (which has a different secret)
	_, err = svc2.ValidateAccessToken(tokenStr)
	if !errors.Is(err, ErrInvalidSignature) {
		t.Fatalf("ValidateAccessToken() error = %v, want ErrInvalidSignature", err)
	}
}

func TestJWTService_ValidateWrongIssuer(t *testing.T) {
	svc1, _ := NewJWTService(testSecret, "wrong-issuer", testAudience, 15*time.Minute)
	svc2, _ := NewJWTService(testSecret, testIssuer, testAudience, 15*time.Minute)

	tokenStr, _, err := svc1.GenerateAccessToken(1, "viet@example.com", "user")
	if err != nil {
		t.Fatalf("GenerateAccessToken() error = %v", err)
	}

	_, err = svc2.ValidateAccessToken(tokenStr)
	if !errors.Is(err, ErrInvalidIssuer) {
		t.Fatalf("ValidateAccessToken() error = %v, want ErrInvalidIssuer", err)
	}
}

func TestJWTService_ValidateWrongAudience(t *testing.T) {
	svc1, _ := NewJWTService(testSecret, testIssuer, "wrong-audience", 15*time.Minute)
	svc2, _ := NewJWTService(testSecret, testIssuer, testAudience, 15*time.Minute)

	tokenStr, _, err := svc1.GenerateAccessToken(1, "viet@example.com", "user")
	if err != nil {
		t.Fatalf("GenerateAccessToken() error = %v", err)
	}

	_, err = svc2.ValidateAccessToken(tokenStr)
	if !errors.Is(err, ErrInvalidAudience) {
		t.Fatalf("ValidateAccessToken() error = %v, want ErrInvalidAudience", err)
	}
}

func TestJWTService_RejectAlgorithmNone(t *testing.T) {
	svc, _ := NewJWTService(testSecret, testIssuer, testAudience, 15*time.Minute)

	// Manually create an unsigned token with alg="none"
	claims := customClaims{
		Email: "viet@example.com",
		Role:  "admin",
		RegisteredClaims: jwt.RegisteredClaims{
			Subject:   "1",
			Issuer:    testIssuer,
			Audience:  jwt.ClaimStrings{testAudience},
			ExpiresAt: jwt.NewNumericDate(time.Now().Add(time.Hour)),
		},
	}
	token := jwt.NewWithClaims(jwt.SigningMethodNone, claims)
	tokenStr, err := token.SignedString(jwt.UnsafeAllowNoneSignatureType)
	if err != nil {
		t.Fatalf("failed to create none-alg token: %v", err)
	}

	_, err = svc.ValidateAccessToken(tokenStr)
	if !errors.Is(err, ErrInvalidAlgorithm) {
		t.Fatalf("ValidateAccessToken() error = %v, want ErrInvalidAlgorithm", err)
	}
}

func TestJWTService_RejectDifferentHMACAlgorithm(t *testing.T) {
	svc, _ := NewJWTService(testSecret, testIssuer, testAudience, 15*time.Minute)

	// Create token signed with HS384 instead of HS256
	claims := customClaims{
		Email: "viet@example.com",
		Role:  "user",
		RegisteredClaims: jwt.RegisteredClaims{
			Subject:   "1",
			Issuer:    testIssuer,
			Audience:  jwt.ClaimStrings{testAudience},
			ExpiresAt: jwt.NewNumericDate(time.Now().Add(time.Hour)),
		},
	}
	token := jwt.NewWithClaims(jwt.SigningMethodHS384, claims)
	tokenStr, err := token.SignedString([]byte(testSecret))
	if err != nil {
		t.Fatalf("failed to create HS384 token: %v", err)
	}

	_, err = svc.ValidateAccessToken(tokenStr)
	if !errors.Is(err, ErrInvalidAlgorithm) {
		t.Fatalf("ValidateAccessToken() error = %v, want ErrInvalidAlgorithm", err)
	}
}

func TestJWTService_InvalidSubjectOrRole(t *testing.T) {
	svc, _ := NewJWTService(testSecret, testIssuer, testAudience, 15*time.Minute)

	t.Run("generate with invalid role", func(t *testing.T) {
		_, _, err := svc.GenerateAccessToken(1, "test@example.com", "superadmin")
		if !errors.Is(err, ErrInvalidRole) {
			t.Fatalf("expected ErrInvalidRole, got %v", err)
		}
	})

	t.Run("generate with invalid user ID", func(t *testing.T) {
		_, _, err := svc.GenerateAccessToken(0, "test@example.com", "user")
		if !errors.Is(err, ErrInvalidSubject) {
			t.Fatalf("expected ErrInvalidSubject, got %v", err)
		}
	})

	t.Run("validate token with non-numeric subject", func(t *testing.T) {
		claims := customClaims{
			Email: "viet@example.com",
			Role:  "user",
			RegisteredClaims: jwt.RegisteredClaims{
				Subject:   "not-a-number",
				Issuer:    testIssuer,
				Audience:  jwt.ClaimStrings{testAudience},
				ExpiresAt: jwt.NewNumericDate(time.Now().Add(time.Hour)),
			},
		}
		token := jwt.NewWithClaims(jwt.SigningMethodHS256, claims)
		tokenStr, _ := token.SignedString([]byte(testSecret))

		_, err := svc.ValidateAccessToken(tokenStr)
		if !errors.Is(err, ErrInvalidSubject) {
			t.Fatalf("expected ErrInvalidSubject, got %v", err)
		}
	})

	t.Run("validate token with invalid role claim", func(t *testing.T) {
		claims := customClaims{
			Email: "viet@example.com",
			Role:  "unrecognized-role",
			RegisteredClaims: jwt.RegisteredClaims{
				Subject:   "10",
				Issuer:    testIssuer,
				Audience:  jwt.ClaimStrings{testAudience},
				ExpiresAt: jwt.NewNumericDate(time.Now().Add(time.Hour)),
			},
		}
		token := jwt.NewWithClaims(jwt.SigningMethodHS256, claims)
		tokenStr, _ := token.SignedString([]byte(testSecret))

		_, err := svc.ValidateAccessToken(tokenStr)
		if !errors.Is(err, ErrInvalidRole) {
			t.Fatalf("expected ErrInvalidRole, got %v", err)
		}
	})
}
