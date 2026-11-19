package config

import (
	"slices"
	"strings"
	"testing"
)

// TestParseAllowedOrigins pins the unset-versus-explicitly-set contract
// directly, independent of the process environment.
func TestParseAllowedOrigins(t *testing.T) {
	defaults := []string{"http://localhost:5173", "http://localhost:3000"}
	const noOrigins = "CORS_ALLOWED_ORIGINS is set but contains no origins"

	tests := []struct {
		name        string
		raw         string
		set         bool
		env         string
		want        []string
		errContains string
	}{
		// Unset.
		{name: "development unset uses defaults", env: "development", want: defaults},
		{name: "dev unset uses defaults", env: "dev", want: defaults},
		{name: "test unset uses defaults", env: "test", want: defaults},
		{name: "staging unset rejected", env: "staging", errContains: "CORS_ALLOWED_ORIGINS is required in staging"},
		{name: "production unset rejected", env: "production", errContains: "CORS_ALLOWED_ORIGINS is required in production"},
		{name: "prod unset rejected", env: "prod", errContains: "CORS_ALLOWED_ORIGINS is required in production"},

		// Explicitly set with no entries: an error in every environment.
		{name: "development set to empty rejected", raw: "", set: true, env: "development", errContains: noOrigins},
		{name: "test set to empty rejected", raw: "", set: true, env: "test", errContains: noOrigins},
		{name: "development set to whitespace rejected", raw: "   ", set: true, env: "development", errContains: noOrigins},
		{name: "development set to commas only rejected", raw: ",,", set: true, env: "development", errContains: noOrigins},
		{name: "development set to commas and spaces rejected", raw: " , , ", set: true, env: "development", errContains: noOrigins},
		{name: "staging set to empty rejected", raw: "", set: true, env: "staging", errContains: noOrigins},
		{name: "production set to empty rejected", raw: "", set: true, env: "production", errContains: noOrigins},

		// Explicitly set with entries.
		{name: "empty segments skipped", raw: ",http://localhost:8081,, ,", set: true, env: "development", want: []string{"http://localhost:8081"}},
		{
			name: "normalized and deduplicated in order",
			raw:  "https://b.example.com, HTTPS://A.example.com:443/, https://b.example.com/",
			set:  true, env: "production",
			want: []string{"https://b.example.com", "https://a.example.com"},
		},
		{
			name: "one invalid entry rejects the whole list",
			raw:  "http://localhost:5173, https://x.example.com/path",
			set:  true, env: "development",
			errContains: `entry "https://x.example.com/path" is invalid`,
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			got, err := parseAllowedOrigins(tc.raw, tc.set, tc.env)
			if tc.errContains != "" {
				if err == nil {
					t.Fatalf("expected error containing %q, got origins %q", tc.errContains, got)
				}
				if !strings.Contains(err.Error(), tc.errContains) {
					t.Errorf("error %q should contain %q", err.Error(), tc.errContains)
				}
				return
			}
			if err != nil {
				t.Fatalf("unexpected error: %v", err)
			}
			if !slices.Equal(got, tc.want) {
				t.Errorf("parseAllowedOrigins = %q, want %q", got, tc.want)
			}
		})
	}

	t.Run("defaults are returned as a copy", func(t *testing.T) {
		got, err := parseAllowedOrigins("", false, "development")
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		got[0] = "mutated"
		if defaultDevAllowedOrigins[0] == "mutated" {
			t.Fatal("caller mutation leaked into defaultDevAllowedOrigins")
		}
	})
}

func TestNormalizeOrigin_Valid(t *testing.T) {
	tests := []struct {
		input string
		want  string
	}{
		// Already canonical.
		{"http://localhost:5173", "http://localhost:5173"},
		{"https://app.example.com", "https://app.example.com"},
		{"https://sub.app.example.com", "https://sub.app.example.com"},
		{"https://a-b.example.com", "https://a-b.example.com"},
		{"https://xn--bcher-kva.example", "https://xn--bcher-kva.example"},

		// Case normalization.
		{"HTTP://LocalHost:5173", "http://localhost:5173"},
		{"HTTPS://EXAMPLE.COM", "https://example.com"},

		// Exactly one trailing slash is accepted and removed.
		{"https://app.example.com/", "https://app.example.com"},
		{"HTTPS://App.Example.com:443/", "https://app.example.com"},
		{"http://[::1]:8080/", "http://[::1]:8080"},

		// Default ports removed; non-default ports kept.
		{"http://example.com:80", "http://example.com"},
		{"https://example.com:443", "https://example.com"},
		{"http://example.com:443", "http://example.com:443"},
		{"https://example.com:80", "https://example.com:80"},
		{"http://example.com:8080", "http://example.com:8080"},
		{"https://example.com:8443", "https://example.com:8443"},
		{"http://example.com:08080", "http://example.com:8080"},
		{"http://localhost:1", "http://localhost:1"},
		{"http://localhost:65535", "http://localhost:65535"},

		// IPv4.
		{"http://127.0.0.1:8080", "http://127.0.0.1:8080"},
		{"https://192.168.1.1:8443", "https://192.168.1.1:8443"},
		{"http://127.0.0.1:80", "http://127.0.0.1"},
		{"https://127.0.0.1:443", "https://127.0.0.1"},

		// IPv6: bracketed, compressed, lowercase.
		{"http://[::1]", "http://[::1]"},
		{"https://[2001:db8::1]", "https://[2001:db8::1]"},
		{"http://[0:0:0:0:0:0:0:1]", "http://[::1]"},
		{"https://[2001:DB8::1]:8443", "https://[2001:db8::1]:8443"},
		{"http://[::1]:80", "http://[::1]"},
		{"https://[::1]:443", "https://[::1]"},
	}

	for _, tc := range tests {
		t.Run(tc.input, func(t *testing.T) {
			got, err := normalizeOrigin(tc.input)
			if err != nil {
				t.Fatalf("normalizeOrigin(%q): unexpected error: %v", tc.input, err)
			}
			if got.String() != tc.want {
				t.Errorf("normalizeOrigin(%q) = %q, want %q", tc.input, got.String(), tc.want)
			}
		})
	}
}

func TestNormalizeOrigin_Invalid(t *testing.T) {
	tests := []struct {
		input       string
		errContains string
	}{
		// Wildcards and null.
		{"*", "wildcards"},
		{"https://*.example.com", "wildcards"},
		{"null", `"null" origin`},
		{"NULL", `"null" origin`},

		// Missing or unsupported scheme.
		{"", "scheme://host"},
		{"example.com", "scheme://host"},
		{"localhost:5173", "scheme://host"},
		{"javascript:alert(1)", "scheme://host"},
		{"data:text/html,test", "scheme://host"},
		{"ftp://example.com", "scheme must be http or https"},
		{"ws://example.com", "scheme must be http or https"},
		{"file:///etc/passwd", "scheme must be http or https"},
		{"null://example.com", "scheme must be http or https"},

		// Empty host.
		{"http://", "host is empty"},
		{"https://", "host is empty"},
		{"https:///", "host is empty"},
		{"http://:8080", "host is empty"},

		// Paths other than a single trailing "/", queries, fragments, userinfo.
		{"https://example.com//", "path, query, fragment, or userinfo"},
		{"https://example.com/api", "path, query, fragment, or userinfo"},
		{"https://example.com/api/", "path, query, fragment, or userinfo"},
		{"https://example.com?q=1", "path, query, fragment, or userinfo"},
		{"https://example.com/?q=1", "path, query, fragment, or userinfo"},
		{"https://example.com#section", "path, query, fragment, or userinfo"},
		{"https://example.com/#section", "path, query, fragment, or userinfo"},
		{"https://user@example.com", "path, query, fragment, or userinfo"},
		{"https://user:pass@example.com", "path, query, fragment, or userinfo"},
		{`https://example.com\path`, "path, query, fragment, or userinfo"},

		// Whitespace and control characters.
		{"https://exa mple.com", "whitespace or control characters"},
		{" https://example.com", "whitespace or control characters"},
		{"https://example.com\t", "whitespace or control characters"},
		{"https://example.com\x00", "whitespace or control characters"},

		// Ports.
		{"http://localhost:", "port is empty"},
		{"https://127.0.0.1:", "port is empty"},
		{"http://[::1]:", "port is empty"},
		{"http://localhost:abc", "invalid port"},
		{"http://localhost:+80", "invalid port"},
		{"http://localhost:0", "between 1 and 65535"},
		{"http://localhost:65536", "between 1 and 65535"},
		{"http://localhost:99999", "between 1 and 65535"},
		{"http://localhost:123456", "invalid port"},
		{"http://localhost:80:80", `too many ":"`},

		// Hostname syntax.
		{"http://-localhost", "invalid host"},
		{"http://localhost-", "invalid host"},
		{"http://local..host", "invalid host"},
		{"http://.localhost", "invalid host"},
		{"http://localhost.", "invalid host"},
		{"http://local_host", "invalid host"},
		{"https://bücher.example", "invalid host"},

		// Hosts a browser would parse as (malformed or shorthand) IPv4.
		{"http://999.999.999.999", "invalid IPv4 address"},
		{"http://300.0.0.1", "invalid IPv4 address"},
		{"http://127.000.000.001", "invalid IPv4 address"},
		{"http://127.1", "invalid IPv4 address"},
		{"http://12345", "invalid IPv4 address"},
		{"http://0x7f", "invalid IPv4 address"},
		{"http://0x7f.0.0.1", "invalid IPv4 address"},
		{"https://example.123", "invalid IPv4 address"},

		// IPv6 syntax.
		{"http://::1", "IPv6 addresses must be bracketed"},
		{"http://::1:8080", "IPv6 addresses must be bracketed"},
		{"http://::1]", "IPv6 addresses must be bracketed"},
		{"http://[::1", `missing its closing "]"`},
		{"http://[::1]x", "unexpected characters after IPv6 address"},
		{"http://[invalid-ipv6]", "invalid IPv6 address"},
		{"http://[127.0.0.1]", "invalid IPv6 address"},
		{"http://[:::1]", "invalid IPv6 address"},
		{"http://[]", "invalid IPv6 address"},
		{"http://[::1]:abc", "invalid port"},
		{"http://[::1]:70000", "between 1 and 65535"},
		{"http://[fe80::1%25eth0]", "zone identifiers"},

		// IPv4-mapped IPv6 is rejected in every environment.
		{"http://[::ffff:127.0.0.1]", "IPv4-mapped IPv6"},
		{"http://[::ffff:7f00:1]", "IPv4-mapped IPv6"},
		{"https://[::FFFF:192.0.2.1]", "IPv4-mapped IPv6"},
	}

	for _, tc := range tests {
		t.Run(tc.input, func(t *testing.T) {
			got, err := normalizeOrigin(tc.input)
			if err == nil {
				t.Fatalf("normalizeOrigin(%q) = %q, want error containing %q", tc.input, got.String(), tc.errContains)
			}
			if !strings.Contains(err.Error(), tc.errContains) {
				t.Errorf("normalizeOrigin(%q) error = %q, want it to contain %q", tc.input, err.Error(), tc.errContains)
			}
		})
	}
}

func TestCheckDeployedOrigin(t *testing.T) {
	tests := []struct {
		input       string
		errContains string // empty means the origin is allowed in staging and production
	}{
		// Allowed.
		{"https://app.example.com", ""},
		{"https://app.example.com:8443", ""},
		{"https://[2001:db8::1]", ""},
		{"https://10.0.0.1", ""},
		{"https://localhost.example.com", ""}, // "localhost" label, but not a .localhost host
		{"https://notlocalhost.com", ""},
		{"https://mylocalhost", ""},

		// https is required.
		{"http://app.example.com", "must use https"},
		{"http://localhost:5173", "must use https"},

		// localhost and its subdomains.
		{"https://localhost", "localhost hosts"},
		{"https://LOCALHOST", "localhost hosts"},
		{"https://localhost:8443", "localhost hosts"},
		{"https://api.localhost", "localhost hosts"},
		{"https://a.b.localhost", "localhost hosts"},

		// Loopback addresses: all of 127.0.0.0/8 and ::1.
		{"https://127.0.0.1", "loopback addresses"},
		{"https://127.1.2.3", "loopback addresses"},
		{"https://127.255.255.254:8443", "loopback addresses"},
		{"https://[::1]", "loopback addresses"},
		{"https://[0:0:0:0:0:0:0:1]", "loopback addresses"},

		// Unspecified addresses (reported separately, not as loopback).
		{"https://0.0.0.0", "unspecified addresses"},
		{"https://[::]", "unspecified addresses"},
		{"https://[0:0:0:0:0:0:0:0]", "unspecified addresses"},
	}

	for _, tc := range tests {
		t.Run(tc.input, func(t *testing.T) {
			o, err := normalizeOrigin(tc.input)
			if err != nil {
				t.Fatalf("normalizeOrigin(%q): unexpected error: %v", tc.input, err)
			}
			err = checkDeployedOrigin(o)
			if tc.errContains == "" {
				if err != nil {
					t.Errorf("checkDeployedOrigin(%q): unexpected error: %v", tc.input, err)
				}
				return
			}
			if err == nil {
				t.Fatalf("checkDeployedOrigin(%q): expected error containing %q, got nil", tc.input, tc.errContains)
			}
			if !strings.Contains(err.Error(), tc.errContains) {
				t.Errorf("checkDeployedOrigin(%q) error = %q, want it to contain %q", tc.input, err.Error(), tc.errContains)
			}
			if tc.errContains == "unspecified addresses" && strings.Contains(err.Error(), "loopback") {
				t.Errorf("unspecified-address error must not describe the address as loopback: %q", err.Error())
			}
		})
	}
}
