package dto

import (
	"net/http"
	"time"
)

// AppConfig is the validated HTTP server configuration. It is produced by
// config.Load, which owns reading and validating the environment.
type AppConfig struct {
	AppEnv                  string
	DatabaseURL             string
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
}
