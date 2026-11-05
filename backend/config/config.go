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

	EnvDevelopment = "development"
	EnvDev         = "dev"
	EnvTest        = "test"
	EnvStaging     = "staging"
	EnvProduction  = "production"
	EnvProd        = "prod"
)

type Config struct {
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

// IsProduction reports whether the configured environment is production or prod.
func (c Config) IsProduction() bool {
	return isProduction(c.AppEnv)
}

func isProduction(env string) bool {
	return env == EnvProduction || env == EnvProd
}

// isDeployed reports whether env is a deployed environment (staging or
// production). Deployed environments must configure JWT_SECRET and
// CORS_ALLOWED_ORIGINS explicitly; development defaults never apply to them.
func isDeployed(env string) bool {
	return env == EnvStaging || isProduction(env)
}

// deployedLabel returns the environment name used in configuration errors,
// collapsing the "prod" alias to "production".
func deployedLabel(env string) string {
	if isProduction(env) {
		return EnvProduction
	}
	return env
}

// validateDeployedJWTSecret enforces JWT_SECRET hygiene for staging and
// production: the secret must be set, must not be the publicly known
// development secret, and must not have surrounding whitespace.
//
// This helper does not enforce the 32-byte minimum length; Load checks that
// for every environment after this helper returns.
//
// These checks reject missing, whitespace-only, padded, and default values.
// Together with Load's length check, they still cannot prove the secret has
// enough entropy; it must be generated randomly, for example with:
// openssl rand -base64 48
//
// Error messages never include the secret value.
func validateDeployedJWTSecret(secret, env string) error {
	label := deployedLabel(env)
	trimmed := strings.TrimSpace(secret)
	if trimmed == "" {
		return fmt.Errorf("JWT_SECRET environment variable is required in %s; generate one with: openssl rand -base64 48", label)
	}
	if trimmed == defaultDevSecret {
		return fmt.Errorf("cannot use default development secret in %s", label)
	}
	if secret != trimmed {
		return fmt.Errorf("JWT_SECRET cannot contain leading or trailing whitespace in %s", label)
	}
	return nil
}

// LoadDatabaseURL validates and returns DATABASE_URL. It reads nothing else,
// so programs that only need database access (such as cmd/createadmin) are not
// subject to HTTP-server settings like JWT_SECRET or CORS_ALLOWED_ORIGINS.
// Load uses it too, so the DATABASE_URL rule is defined in one place.
func LoadDatabaseURL() (string, error) {
	databaseURL := os.Getenv("DATABASE_URL")
	if databaseURL == "" {
		return "", errors.New("DATABASE_URL environment variable is required")
	}
	return databaseURL, nil
}

// resolveEnvironment determines and validates the application environment from APP_ENV and ENV.
// Trimming and case normalization are applied. APP_ENV takes precedence over ENV.
// If neither is set or non-empty, it defaults to "development".
func resolveEnvironment() (string, error) {
	rawAppEnv := strings.ToLower(strings.TrimSpace(os.Getenv("APP_ENV")))
	rawEnv := strings.ToLower(strings.TrimSpace(os.Getenv("ENV")))

	var env string
	if rawAppEnv != "" {
		env = rawAppEnv
	} else if rawEnv != "" {
		env = rawEnv
	} else {
		env = EnvDevelopment
	}

	switch env {
	case EnvDevelopment, EnvDev, EnvTest, EnvStaging, EnvProduction, EnvProd:
		return env, nil
	default:
		return "", fmt.Errorf("unsupported environment %q: must be one of 'development', 'dev', 'test', 'staging', 'production', or 'prod'", env)
	}
}

// Load reads and validates the HTTP server configuration from the environment.
//
// Staging and production (see isDeployed) require an explicit JWT_SECRET and
// an explicit https-only CORS_ALLOWED_ORIGINS allowlist; development and test
// fall back to built-in defaults for both.
func Load() (Config, error) {
	databaseURL, err := LoadDatabaseURL()
	if err != nil {
		return Config{}, err
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

	appEnv, err := resolveEnvironment()
	if err != nil {
		return Config{}, err
	}

	jwtSecret := os.Getenv("JWT_SECRET")
	if isDeployed(appEnv) {
		if err := validateDeployedJWTSecret(jwtSecret, appEnv); err != nil {
			return Config{}, err
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
		parsed, err := strconv.ParseBool(strings.TrimSpace(val))
		if err != nil {
			return Config{}, fmt.Errorf("COOKIE_SECURE must be a boolean value (true/false), got %q: %w", val, err)
		}
		cookieSecure = parsed
	} else if isProduction(appEnv) {
		cookieSecure = true
	}

	if isProduction(appEnv) && !cookieSecure {
		return Config{}, errors.New("COOKIE_SECURE cannot be false in production")
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

	if cookieSameSite == http.SameSiteNoneMode && !cookieSecure {
		return Config{}, errors.New("COOKIE_SAMESITE=none requires COOKIE_SECURE=true")
	}

	// Validated last, so errors in earlier settings are reported first.
	// LookupEnv distinguishes "unset" (development defaults apply) from
	// "explicitly set to an empty value" (an error in every environment).
	rawOrigins, originsSet := os.LookupEnv("CORS_ALLOWED_ORIGINS")
	allowedOrigins, err := parseAllowedOrigins(rawOrigins, originsSet, appEnv)
	if err != nil {
		return Config{}, err
	}

	return Config{
		AppEnv:                  appEnv,
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
