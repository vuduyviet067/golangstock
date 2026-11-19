package dto

import (
	"net/http"
	"time"
)

type AppConfig struct {
	AppEnv      string
	DatabaseURL string
	// ExpectedDatabaseName, when set, is the database the process must be
	// connected to; startup fails otherwise. Empty means no check.
	ExpectedDatabaseName    string
	HTTPPort                string
	JWTSecret               string
	JWTIssuer               string
	JWTAudience             string
	AccessTokenTTL          time.Duration
	RefreshTokenTTL         time.Duration
	RefreshTokenGracePeriod time.Duration
	CookieSecure            bool
	CookieSameSite          http.SameSite
	AllowedOrigins          []string
	// AllowRegistration enables POST /api/v1/auth/register (ALLOW_REGISTRATION,
	// default false).
	AllowRegistration bool
}
