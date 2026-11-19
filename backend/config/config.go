package config

import (
	"errors"
	"fmt"
	"net/http"
	"os"
	"strconv"
	"strings"
	"time"

	"gostock/internal/dto"
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

func IsProduction(cfg dto.AppConfig) bool {
	return isProduction(cfg.AppEnv)
}

func isProduction(env string) bool {
	return env == EnvProduction || env == EnvProd
}

func isDeployed(env string) bool {
	return env == EnvStaging || isProduction(env)
}

func deployedLabel(env string) string {
	if isProduction(env) {
		return EnvProduction
	}
	return env
}

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

func LoadDatabaseURL() (string, error) {
	databaseURL := os.Getenv("DATABASE_URL")
	if databaseURL == "" {
		return "", errors.New("DATABASE_URL environment variable is required")
	}
	return databaseURL, nil
}

// LoadExpectedDatabaseName reads EXPECTED_DATABASE_NAME, the name of the
// database the process must be connected to (checked with current_database()
// after connecting). Unset means no check. A set value must be non-empty and
// free of leading or trailing whitespace; it is returned unchanged.
func LoadExpectedDatabaseName() (string, error) {
	name, set := os.LookupEnv("EXPECTED_DATABASE_NAME")
	if !set {
		return "", nil
	}
	if strings.TrimSpace(name) == "" {
		return "", errors.New("EXPECTED_DATABASE_NAME is set but empty; unset it or set it to the expected database name")
	}
	if name != strings.TrimSpace(name) {
		return "", errors.New("EXPECTED_DATABASE_NAME cannot contain leading or trailing whitespace")
	}
	return name, nil
}

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

func Load() (dto.AppConfig, error) {
	databaseURL, err := LoadDatabaseURL()
	if err != nil {
		return dto.AppConfig{}, err
	}

	expectedDatabaseName, err := LoadExpectedDatabaseName()
	if err != nil {
		return dto.AppConfig{}, err
	}

	port := os.Getenv("HTTP_PORT")
	if port == "" {
		port = defaultHTTPPort
	} else {
		portNum, err := strconv.Atoi(port)
		if err != nil || portNum < 1 || portNum > 65535 {
			return dto.AppConfig{}, fmt.Errorf("HTTP_PORT must be a valid port number between 1 and 65535, got %q", port)
		}
	}

	appEnv, err := resolveEnvironment()
	if err != nil {
		return dto.AppConfig{}, err
	}

	jwtSecret := os.Getenv("JWT_SECRET")
	if isDeployed(appEnv) {
		if err := validateDeployedJWTSecret(jwtSecret, appEnv); err != nil {
			return dto.AppConfig{}, err
		}
	} else if jwtSecret == "" {
		jwtSecret = defaultDevSecret
	}

	if len(jwtSecret) < 32 {
		return dto.AppConfig{}, errors.New("JWT_SECRET must be at least 32 bytes for HS256")
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
			return dto.AppConfig{}, errors.New("ACCESS_TOKEN_TTL_MINUTES must be a positive integer")
		}
		accessTokenTTL = time.Duration(mins) * time.Minute
	}

	refreshTokenTTL := time.Duration(defaultRefreshTTLDays) * 24 * time.Hour
	if val := os.Getenv("REFRESH_TOKEN_TTL_DAYS"); val != "" {
		days, err := strconv.Atoi(val)
		if err != nil || days <= 0 {
			return dto.AppConfig{}, errors.New("REFRESH_TOKEN_TTL_DAYS must be a positive integer")
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
			return dto.AppConfig{}, errors.New("REFRESH_TOKEN_GRACE_PERIOD must be a non-negative duration or integer seconds")
		}
	}

	cookieSecure := false
	if val := os.Getenv("COOKIE_SECURE"); val != "" {
		parsed, err := strconv.ParseBool(strings.TrimSpace(val))
		if err != nil {
			return dto.AppConfig{}, fmt.Errorf("COOKIE_SECURE must be a boolean value (true/false), got %q: %w", val, err)
		}
		cookieSecure = parsed
	} else if isProduction(appEnv) {
		cookieSecure = true
	}

	if isProduction(appEnv) && !cookieSecure {
		return dto.AppConfig{}, errors.New("COOKIE_SECURE cannot be false in production")
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
			return dto.AppConfig{}, fmt.Errorf("COOKIE_SAMESITE must be one of 'lax', 'strict', or 'none', got %q", rawVal)
		}
	}

	if cookieSameSite == http.SameSiteNoneMode && !cookieSecure {
		return dto.AppConfig{}, errors.New("COOKIE_SAMESITE=none requires COOKIE_SECURE=true")
	}

	rawOrigins, originsSet := os.LookupEnv("CORS_ALLOWED_ORIGINS")
	allowedOrigins, err := parseAllowedOrigins(rawOrigins, originsSet, appEnv)
	if err != nil {
		return dto.AppConfig{}, err
	}

	return dto.AppConfig{
		AppEnv:                  appEnv,
		DatabaseURL:             databaseURL,
		ExpectedDatabaseName:    expectedDatabaseName,
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
