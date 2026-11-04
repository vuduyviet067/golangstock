package config

import (
	"net/http"
	"strings"
	"testing"
	"time"
)

const testBaseDB = "postgres://user:pass@localhost:5432/gostock?sslmode=disable"

func setBaseEnv(t *testing.T) {
	t.Helper()
	t.Setenv("DATABASE_URL", testBaseDB)
}

func TestConfig_HTTPPortValidation(t *testing.T) {
	tests := []struct {
		name      string
		portEnv   string
		wantPort  string
		wantError bool
	}{
		{name: "default port when empty", portEnv: "", wantPort: "8080", wantError: false},
		{name: "valid port 80", portEnv: "80", wantPort: "80", wantError: false},
		{name: "valid port 8080", portEnv: "8080", wantPort: "8080", wantError: false},
		{name: "valid port min 1", portEnv: "1", wantPort: "1", wantError: false},
		{name: "valid port max 65535", portEnv: "65535", wantPort: "65535", wantError: false},
		{name: "invalid zero port", portEnv: "0", wantError: true},
		{name: "invalid negative port", portEnv: "-80", wantError: true},
		{name: "invalid out-of-range port 65536", portEnv: "65536", wantError: true},
		{name: "invalid out-of-range port 99999", portEnv: "99999", wantError: true},
		{name: "invalid non-numeric port", portEnv: "http", wantError: true},
		{name: "invalid mixed alphanumeric port", portEnv: "8080abc", wantError: true},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			setBaseEnv(t)
			if tc.portEnv != "" {
				t.Setenv("HTTP_PORT", tc.portEnv)
			}

			cfg, err := Load()
			if tc.wantError {
				if err == nil {
					t.Fatalf("expected error for HTTP_PORT=%q, got nil", tc.portEnv)
				}
				if !strings.Contains(err.Error(), "HTTP_PORT") {
					t.Errorf("error %q should mention HTTP_PORT", err.Error())
				}
			} else {
				if err != nil {
					t.Fatalf("unexpected error: %v", err)
				}
				if cfg.HTTPPort != tc.wantPort {
					t.Errorf("HTTPPort = %q, want %q", cfg.HTTPPort, tc.wantPort)
				}
			}
		})
	}
}

func TestConfig_CookieSecureValidation(t *testing.T) {
	tests := []struct {
		name       string
		secureEnv  string
		appEnv     string
		wantSecure bool
		wantError  bool
	}{
		{name: "default dev is false", secureEnv: "", appEnv: "development", wantSecure: false, wantError: false},
		{name: "default prod is true", secureEnv: "", appEnv: "production", wantSecure: true, wantError: false},
		{name: "explicit true string", secureEnv: "true", wantSecure: true, wantError: false},
		{name: "explicit false string", secureEnv: "false", wantSecure: false, wantError: false},
		{name: "explicit 1 as true", secureEnv: "1", wantSecure: true, wantError: false},
		{name: "explicit 0 as false", secureEnv: "0", wantSecure: false, wantError: false},
		{name: "case insensitive TRUE", secureEnv: "TRUE", wantSecure: true, wantError: false},
		{name: "case insensitive False", secureEnv: "False", wantSecure: false, wantError: false},
		{name: "invalid string rejected", secureEnv: "yes", wantError: true},
		{name: "invalid number rejected", secureEnv: "2", wantError: true},
		{name: "invalid text rejected", secureEnv: "enabled", wantError: true},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			setBaseEnv(t)
			if tc.appEnv == "production" {
				t.Setenv("APP_ENV", "production")
				t.Setenv("JWT_SECRET", "production-secret-with-at-least-32-bytes-long!")
			}
			if tc.secureEnv != "" {
				t.Setenv("COOKIE_SECURE", tc.secureEnv)
			}

			cfg, err := Load()
			if tc.wantError {
				if err == nil {
					t.Fatalf("expected error for COOKIE_SECURE=%q, got nil", tc.secureEnv)
				}
				if !strings.Contains(err.Error(), "COOKIE_SECURE") {
					t.Errorf("error %q should mention COOKIE_SECURE", err.Error())
				}
			} else {
				if err != nil {
					t.Fatalf("unexpected error: %v", err)
				}
				if cfg.CookieSecure != tc.wantSecure {
					t.Errorf("CookieSecure = %v, want %v", cfg.CookieSecure, tc.wantSecure)
				}
			}
		})
	}
}

func TestConfig_CookieSameSiteValidation(t *testing.T) {
	tests := []struct {
		name         string
		sameSiteEnv  string
		wantSameSite http.SameSite
		wantError    bool
	}{
		{name: "default when empty is Lax", sameSiteEnv: "", wantSameSite: http.SameSiteLaxMode, wantError: false},
		{name: "explicit lax", sameSiteEnv: "lax", wantSameSite: http.SameSiteLaxMode, wantError: false},
		{name: "explicit strict", sameSiteEnv: "strict", wantSameSite: http.SameSiteStrictMode, wantError: false},
		{name: "explicit none", sameSiteEnv: "none", wantSameSite: http.SameSiteNoneMode, wantError: false},
		{name: "case insensitive Lax", sameSiteEnv: "Lax", wantSameSite: http.SameSiteLaxMode, wantError: false},
		{name: "case insensitive STRICT", sameSiteEnv: "STRICT", wantSameSite: http.SameSiteStrictMode, wantError: false},
		{name: "trimmed whitespace", sameSiteEnv: "  none  ", wantSameSite: http.SameSiteNoneMode, wantError: false},
		{name: "invalid string rejected", sameSiteEnv: "invalid", wantError: true},
		{name: "invalid default keyword rejected", sameSiteEnv: "default", wantError: true},
		{name: "invalid off keyword rejected", sameSiteEnv: "off", wantError: true},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			setBaseEnv(t)
			if tc.sameSiteEnv != "" {
				t.Setenv("COOKIE_SAMESITE", tc.sameSiteEnv)
			}

			cfg, err := Load()
			if tc.wantError {
				if err == nil {
					t.Fatalf("expected error for COOKIE_SAMESITE=%q, got nil", tc.sameSiteEnv)
				}
				if !strings.Contains(err.Error(), "COOKIE_SAMESITE") {
					t.Errorf("error %q should mention COOKIE_SAMESITE", err.Error())
				}
			} else {
				if err != nil {
					t.Fatalf("unexpected error: %v", err)
				}
				if cfg.CookieSameSite != tc.wantSameSite {
					t.Errorf("CookieSameSite = %v, want %v", cfg.CookieSameSite, tc.wantSameSite)
				}
			}
		})
	}
}

func TestConfig_JWTValidation(t *testing.T) {
	t.Run("default dev secret in development", func(t *testing.T) {
		setBaseEnv(t)
		cfg, err := Load()
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if cfg.JWTSecret != defaultDevSecret {
			t.Errorf("JWTSecret = %q, want %q", cfg.JWTSecret, defaultDevSecret)
		}
		if len(cfg.JWTSecret) < 32 {
			t.Errorf("expected secret len >= 32, got %d", len(cfg.JWTSecret))
		}
	})

	t.Run("production requires JWT_SECRET", func(t *testing.T) {
		setBaseEnv(t)
		t.Setenv("APP_ENV", "production")
		_, err := Load()
		if err == nil {
			t.Fatal("expected error in production when JWT_SECRET is unset, got nil")
		}
	})

	t.Run("production rejects default dev secret", func(t *testing.T) {
		setBaseEnv(t)
		t.Setenv("APP_ENV", "production")
		t.Setenv("JWT_SECRET", defaultDevSecret)
		_, err := Load()
		if err == nil {
			t.Fatal("expected error in production when using default dev secret, got nil")
		}
	})

	t.Run("custom secret shorter than 32 bytes rejected", func(t *testing.T) {
		setBaseEnv(t)
		t.Setenv("JWT_SECRET", "too-short")
		_, err := Load()
		if err == nil {
			t.Fatal("expected error for secret < 32 bytes, got nil")
		}
	})

	t.Run("custom valid secret accepted", func(t *testing.T) {
		setBaseEnv(t)
		customSecret := "this-is-a-valid-production-secret-of-sufficient-length!"
		t.Setenv("JWT_SECRET", customSecret)
		cfg, err := Load()
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if cfg.JWTSecret != customSecret {
			t.Errorf("JWTSecret = %q, want %q", cfg.JWTSecret, customSecret)
		}
	})
}

func TestConfig_TokenTTLValidation(t *testing.T) {
	t.Run("default token TTLs", func(t *testing.T) {
		setBaseEnv(t)
		cfg, err := Load()
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if cfg.AccessTokenTTL != 15*time.Minute {
			t.Errorf("AccessTokenTTL = %v, want 15m", cfg.AccessTokenTTL)
		}
		if cfg.RefreshTokenTTL != 7*24*time.Hour {
			t.Errorf("RefreshTokenTTL = %v, want 7d", cfg.RefreshTokenTTL)
		}
	})

	t.Run("valid custom token TTLs", func(t *testing.T) {
		setBaseEnv(t)
		t.Setenv("ACCESS_TOKEN_TTL_MINUTES", "30")
		t.Setenv("REFRESH_TOKEN_TTL_DAYS", "14")
		cfg, err := Load()
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if cfg.AccessTokenTTL != 30*time.Minute {
			t.Errorf("AccessTokenTTL = %v, want 30m", cfg.AccessTokenTTL)
		}
		if cfg.RefreshTokenTTL != 14*24*time.Hour {
			t.Errorf("RefreshTokenTTL = %v, want 14d", cfg.RefreshTokenTTL)
		}
	})

	t.Run("invalid negative or zero access token TTL", func(t *testing.T) {
		setBaseEnv(t)
		t.Setenv("ACCESS_TOKEN_TTL_MINUTES", "-1")
		_, err := Load()
		if err == nil {
			t.Fatal("expected error for negative access token TTL, got nil")
		}

		t.Setenv("ACCESS_TOKEN_TTL_MINUTES", "0")
		_, err2 := Load()
		if err2 == nil {
			t.Fatal("expected error for zero access token TTL, got nil")
		}
	})

	t.Run("invalid negative or zero refresh token TTL", func(t *testing.T) {
		setBaseEnv(t)
		t.Setenv("REFRESH_TOKEN_TTL_DAYS", "-5")
		_, err := Load()
		if err == nil {
			t.Fatal("expected error for negative refresh token TTL, got nil")
		}

		t.Setenv("REFRESH_TOKEN_TTL_DAYS", "0")
		_, err2 := Load()
		if err2 == nil {
			t.Fatal("expected error for zero refresh token TTL, got nil")
		}
	})
}

func TestConfig_GracePeriodParsing(t *testing.T) {
	t.Run("default grace period", func(t *testing.T) {
		setBaseEnv(t)
		cfg, err := Load()
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if cfg.RefreshTokenGracePeriod != 15*time.Second {
			t.Errorf("RefreshTokenGracePeriod = %v, want 15s", cfg.RefreshTokenGracePeriod)
		}
	})

	t.Run("parsed as duration string", func(t *testing.T) {
		setBaseEnv(t)
		t.Setenv("REFRESH_TOKEN_GRACE_PERIOD", "30s")
		cfg, err := Load()
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if cfg.RefreshTokenGracePeriod != 30*time.Second {
			t.Errorf("RefreshTokenGracePeriod = %v, want 30s", cfg.RefreshTokenGracePeriod)
		}
	})

	t.Run("parsed as integer seconds", func(t *testing.T) {
		setBaseEnv(t)
		t.Setenv("REFRESH_TOKEN_GRACE_PERIOD", "45")
		cfg, err := Load()
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if cfg.RefreshTokenGracePeriod != 45*time.Second {
			t.Errorf("RefreshTokenGracePeriod = %v, want 45s", cfg.RefreshTokenGracePeriod)
		}
	})

	t.Run("invalid string rejected", func(t *testing.T) {
		setBaseEnv(t)
		t.Setenv("REFRESH_TOKEN_GRACE_PERIOD", "invalid-duration")
		_, err := Load()
		if err == nil {
			t.Fatal("expected error for invalid grace period, got nil")
		}
	})
}

func TestConfig_MissingDatabaseURL(t *testing.T) {
	// Do not set DATABASE_URL
	_, err := Load()
	if err == nil {
		t.Fatal("expected error when DATABASE_URL is missing, got nil")
	}
}
