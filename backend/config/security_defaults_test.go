package config

import (
	"net/http"
	"strings"
	"testing"
)

// XG-03b: auth cookies are Secure by default except in the local
// environments, and self-registration is off unless ALLOW_REGISTRATION is
// true.

// TestConfig_CookieSecureMatrix pins the Secure rule for every accepted
// APP_ENV (and APP_ENV unset) × COOKIE_SECURE × COOKIE_SAMESITE.
func TestConfig_CookieSecureMatrix(t *testing.T) {
	const (
		unset   = "<unset>"
		reject  = "error"
		secure  = "true"
		plain   = "false"
		sameErr = "COOKIE_SAMESITE=none requires COOKIE_SECURE=true"
		prodErr = "COOKIE_SECURE cannot be false in production"
	)
	// want[cookieSecure] for SameSite lax (or unset) and none.
	type outcome struct{ lax, none string }
	local := map[string]outcome{
		unset:   {plain, reject + ":" + sameErr},
		"true":  {secure, secure},
		"false": {plain, reject + ":" + sameErr},
	}
	deployedNonProd := map[string]outcome{
		unset:   {secure, secure},
		"true":  {secure, secure},
		"false": {plain, reject + ":" + sameErr},
	}
	production := map[string]outcome{
		unset:   {secure, secure},
		"true":  {secure, secure},
		"false": {reject + ":" + prodErr, reject + ":" + prodErr},
	}
	byEnv := map[string]map[string]outcome{
		unset:         local, // APP_ENV unset resolves to development
		"development": local,
		"dev":         local,
		"test":        local,
		"staging":     deployedNonProd,
		"production":  production,
		"prod":        production,
	}

	for appEnv, rows := range byEnv {
		for cookieSecure, want := range rows {
			for _, sameSite := range []string{unset, "lax", "none"} {
				expected := want.lax
				if sameSite == "none" {
					expected = want.none
				}
				name := "APP_ENV=" + appEnv + "/COOKIE_SECURE=" + cookieSecure + "/COOKIE_SAMESITE=" + sameSite
				t.Run(name, func(t *testing.T) {
					setBaseEnv(t)
					if appEnv != unset {
						t.Setenv("APP_ENV", appEnv)
					}
					if deployedTestEnvs[appEnv] {
						setIfUnset(t, "JWT_SECRET", testProdSecret)
						setIfUnset(t, "CORS_ALLOWED_ORIGINS", testDeployedOrigins)
					}
					if cookieSecure != unset {
						t.Setenv("COOKIE_SECURE", cookieSecure)
					}
					if sameSite != unset {
						t.Setenv("COOKIE_SAMESITE", sameSite)
					}

					cfg, err := Load()
					if msg, isErr := strings.CutPrefix(expected, reject+":"); isErr {
						if err == nil || !strings.Contains(err.Error(), msg) {
							t.Fatalf("err = %v, want an error containing %q", err, msg)
						}
						return
					}
					if err != nil {
						t.Fatalf("unexpected error: %v", err)
					}
					if got := cfg.CookieSecure; got != (expected == secure) {
						t.Errorf("CookieSecure = %v, want %s", got, expected)
					}
					wantSameSite := http.SameSiteLaxMode
					if sameSite == "none" {
						wantSameSite = http.SameSiteNoneMode
					}
					if cfg.CookieSameSite != wantSameSite {
						t.Errorf("CookieSameSite = %v, want %v", cfg.CookieSameSite, wantSameSite)
					}
				})
			}
		}
	}
}

// APP_ENV=local is not an accepted environment (unchanged by XG-03b): it is
// rejected at startup, so it never gets the local Secure=false default.
func TestConfig_LocalAppEnvIsRejected(t *testing.T) {
	setBaseEnv(t)
	t.Setenv("APP_ENV", "local")
	if _, err := Load(); err == nil || !strings.Contains(err.Error(), `unsupported environment "local"`) {
		t.Fatalf("err = %v, want unsupported environment", err)
	}
}

func TestConfig_AllowRegistration(t *testing.T) {
	tests := []struct {
		name    string
		value   string
		set     bool
		want    bool
		wantErr string
	}{
		{name: "unset defaults to false", set: false, want: false},
		{name: "empty is false", value: "", set: true, want: false},
		{name: "blank is false", value: "  ", set: true, want: false},
		{name: "true", value: "true", set: true, want: true},
		{name: "TRUE with spaces", value: " TRUE ", set: true, want: true},
		{name: "1", value: "1", set: true, want: true},
		{name: "false", value: "false", set: true, want: false},
		{name: "0", value: "0", set: true, want: false},
		{name: "yes is rejected", value: "yes", set: true, wantErr: `ALLOW_REGISTRATION must be a boolean value (true/false), got "yes"`},
		{name: "typo is rejected", value: "ture", set: true, wantErr: `ALLOW_REGISTRATION must be a boolean value (true/false), got "ture"`},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			setBaseEnv(t)
			if tt.set {
				t.Setenv("ALLOW_REGISTRATION", tt.value)
			}
			cfg, err := Load()
			if tt.wantErr != "" {
				if err == nil || !strings.Contains(err.Error(), tt.wantErr) {
					t.Fatalf("err = %v, want %q", err, tt.wantErr)
				}
				return
			}
			if err != nil {
				t.Fatalf("unexpected error: %v", err)
			}
			if cfg.AllowRegistration != tt.want {
				t.Errorf("AllowRegistration = %v, want %v", cfg.AllowRegistration, tt.want)
			}
		})
	}
}
