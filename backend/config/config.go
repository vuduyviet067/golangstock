package config

import (
	"errors"
	"fmt"
	"net/http"
	"os"
	"strconv"
	"strings"
	"time"
)

const (
	defaultDevSecret        = "dev-only-secret-key-that-is-at-least-32-bytes-long!"
	defaultJWTIssuer        = "gostock"
	defaultJWTAudience      = "gostock-api"
	defaultAccessTTLMin     = 15
	defaultRefreshTTLDays   = 7
	defaultRefreshGraceSecs = 15
	defaultHTTPPort         = "8080"
)

type Config struct {
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

func Load() (Config, error) {
	databaseURL := os.Getenv("DATABASE_URL")
	if databaseURL == "" {
		return Config{}, errors.New("DATABASE_URL environment variable is required")
	}

	port := os.Getenv("HTTP_PORT")
	if port == "" {
		port = defaultHTTPPort
	} else {
		portNum, err := strconv.Atoi(port)
		if err != nil || portNum < 1 || portNum > 65535 {
			return Config{}, fmt.Errorf("HTTP_PORT must be a valid port number between 1 and 65535, got %q", port)
		}
	}

	appEnv := os.Getenv("APP_ENV")
	if appEnv == "" {
		appEnv = os.Getenv("ENV")
	}

	jwtSecret := os.Getenv("JWT_SECRET")
	if appEnv == "production" {
		if jwtSecret == "" {
			return Config{}, errors.New("JWT_SECRET environment variable is required in production")
		}
		if jwtSecret == defaultDevSecret {
			return Config{}, errors.New("cannot use default development secret in production")
		}
	} else if jwtSecret == "" {
		jwtSecret = defaultDevSecret
	}

	if len(jwtSecret) < 32 {
		return Config{}, errors.New("JWT_SECRET must be at least 32 bytes for HS256")
	}

	jwtIssuer := os.Getenv("JWT_ISSUER")
	if jwtIssuer == "" {
		jwtIssuer = defaultJWTIssuer
	}

	jwtAudience := os.Getenv("JWT_AUDIENCE")
	if jwtAudience == "" {
		jwtAudience = defaultJWTAudience
	}

	accessTokenTTL := time.Duration(defaultAccessTTLMin) * time.Minute
	if val := os.Getenv("ACCESS_TOKEN_TTL_MINUTES"); val != "" {
		mins, err := strconv.Atoi(val)
		if err != nil || mins <= 0 {
			return Config{}, errors.New("ACCESS_TOKEN_TTL_MINUTES must be a positive integer")
		}
		accessTokenTTL = time.Duration(mins) * time.Minute
	}

	refreshTokenTTL := time.Duration(defaultRefreshTTLDays) * 24 * time.Hour
	if val := os.Getenv("REFRESH_TOKEN_TTL_DAYS"); val != "" {
		days, err := strconv.Atoi(val)
		if err != nil || days <= 0 {
			return Config{}, errors.New("REFRESH_TOKEN_TTL_DAYS must be a positive integer")
		}
		refreshTokenTTL = time.Duration(days) * 24 * time.Hour
	}

	refreshTokenGracePeriod := time.Duration(defaultRefreshGraceSecs) * time.Second
	if val := os.Getenv("REFRESH_TOKEN_GRACE_PERIOD"); val != "" {
		if d, err := time.ParseDuration(val); err == nil && d >= 0 {
			refreshTokenGracePeriod = d
		} else if secs, err := strconv.Atoi(val); err == nil && secs >= 0 {
			refreshTokenGracePeriod = time.Duration(secs) * time.Second
		} else {
			return Config{}, errors.New("REFRESH_TOKEN_GRACE_PERIOD must be a non-negative duration or integer seconds")
		}
	}

	cookieSecure := false
	if val := os.Getenv("COOKIE_SECURE"); val != "" {
		parsed, err := strconv.ParseBool(val)
		if err != nil {
			return Config{}, fmt.Errorf("COOKIE_SECURE must be a boolean value (true/false), got %q: %w", val, err)
		}
		cookieSecure = parsed
	} else if appEnv == "production" {
		cookieSecure = true
	}

	cookieSameSite := http.SameSiteLaxMode
	if rawVal := os.Getenv("COOKIE_SAMESITE"); rawVal != "" {
		val := strings.ToLower(strings.TrimSpace(rawVal))
		switch val {
		case "lax":
			cookieSameSite = http.SameSiteLaxMode
		case "strict":
			cookieSameSite = http.SameSiteStrictMode
		case "none":
			cookieSameSite = http.SameSiteNoneMode
		default:
			return Config{}, fmt.Errorf("COOKIE_SAMESITE must be one of 'lax', 'strict', or 'none', got %q", rawVal)
		}
	}

	allowedOrigins := []string{"http://localhost:5173", "http://localhost:3000"}
	if val := os.Getenv("CORS_ALLOWED_ORIGINS"); val != "" {
		parts := strings.Split(val, ",")
		allowedOrigins = make([]string, 0, len(parts))
		for _, p := range parts {
			trimmed := strings.TrimSpace(p)
			if trimmed != "" {
				allowedOrigins = append(allowedOrigins, trimmed)
			}
		}
	}

	return Config{
		DatabaseURL:             databaseURL,
		HTTPPort:                port,
		JWTSecret:               jwtSecret,
		JWTIssuer:               jwtIssuer,
		JWTAudience:             jwtAudience,
		AccessTokenTTL:          accessTokenTTL,
		RefreshTokenTTL:         refreshTokenTTL,
		RefreshTokenGracePeriod: refreshTokenGracePeriod,
		CookieSecure:            cookieSecure,
		CookieSameSite:          cookieSameSite,
		AllowedOrigins:          allowedOrigins,
	}, nil
}
