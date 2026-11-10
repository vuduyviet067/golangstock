package config

import (
	"errors"
	"fmt"
	"net/netip"
	"slices"
	"strconv"
	"strings"
)

var defaultDevAllowedOrigins = []string{"http://localhost:5173", "http://localhost:3000"}

type origin struct {
	scheme string
	host   string
	port   string
	ip     netip.Addr
}

func (o origin) String() string {
	if o.port == "" {
		return o.scheme + "://" + o.host
	}
	return o.scheme + "://" + o.host + ":" + o.port
}

func parseAllowedOrigins(raw string, set bool, env string) ([]string, error) {
	deployed := isDeployed(env)
	if !set {
		if deployed {
			return nil, fmt.Errorf("CORS_ALLOWED_ORIGINS is required in %s; set a comma-separated list of https origins", deployedLabel(env))
		}
		return slices.Clone(defaultDevAllowedOrigins), nil
	}

	var origins []string
	for _, part := range strings.Split(raw, ",") {
		entry := strings.TrimSpace(part)
		if entry == "" {
			continue
		}
		o, err := normalizeOrigin(entry)
		if err != nil {
			return nil, fmt.Errorf("CORS_ALLOWED_ORIGINS entry %q is invalid: %w", entry, err)
		}
		if deployed {
			if err := checkDeployedOrigin(o); err != nil {
				return nil, fmt.Errorf("CORS_ALLOWED_ORIGINS entry %q is not allowed in %s: %w", entry, deployedLabel(env), err)
			}
		}
		if canonical := o.String(); !slices.Contains(origins, canonical) {
			origins = append(origins, canonical)
		}
	}
	if len(origins) == 0 {
		return nil, errors.New("CORS_ALLOWED_ORIGINS is set but contains no origins")
	}
	return origins, nil
}

func normalizeOrigin(raw string) (origin, error) {
	if strings.Contains(raw, "*") {
		return origin{}, errors.New("wildcards are not supported; list each origin explicitly")
	}
	if strings.EqualFold(raw, "null") {
		return origin{}, errors.New(`the "null" origin is not allowed`)
	}
	for i := 0; i < len(raw); i++ {
		if c := raw[i]; c <= ' ' || c == 0x7f {
			return origin{}, errors.New("must not contain whitespace or control characters")
		}
	}

	scheme, rest, ok := strings.Cut(raw, "://")
	if !ok {
		return origin{}, errors.New("must have the form scheme://host[:port], for example https://app.example.com")
	}
	scheme = strings.ToLower(scheme)
	if scheme != "http" && scheme != "https" {
		return origin{}, fmt.Errorf("scheme must be http or https, got %q", scheme)
	}

	// Accept exactly one trailing slash, a common copy-paste form, and drop it.
	rest = strings.TrimSuffix(rest, "/")
	if strings.ContainsAny(rest, `/?#@\`) {
		return origin{}, errors.New("must not contain a path, query, fragment, or userinfo")
	}

	hostPart, portPart, err := splitHostPort(rest)
	if err != nil {
		return origin{}, err
	}
	host, ip, err := normalizeHost(hostPart)
	if err != nil {
		return origin{}, err
	}
	port, err := normalizePort(scheme, portPart)
	if err != nil {
		return origin{}, err
	}
	return origin{scheme: scheme, host: host, port: port, ip: ip}, nil
}

func splitHostPort(s string) (host, port string, err error) {
	if strings.HasPrefix(s, "[") {
		end := strings.IndexByte(s, ']')
		if end < 0 {
			return "", "", errors.New(`IPv6 address is missing its closing "]"`)
		}
		host, after := s[:end+1], s[end+1:]
		if after == "" {
			return host, "", nil
		}
		if after[0] != ':' {
			return "", "", errors.New("unexpected characters after IPv6 address")
		}
		if after[1:] == "" {
			return "", "", errors.New("port is empty")
		}
		return host, after[1:], nil
	}

	if strings.ContainsAny(s, "[]") {
		return "", "", errors.New(`misplaced "[" or "]"; IPv6 addresses must be bracketed, for example https://[2001:db8::1]`)
	}
	host, port, hasPort := strings.Cut(s, ":")
	if !hasPort {
		return host, "", nil
	}
	if strings.Contains(port, ":") {
		return "", "", errors.New(`too many ":"; IPv6 addresses must be bracketed, for example https://[2001:db8::1]`)
	}
	if port == "" {
		return "", "", errors.New("port is empty")
	}
	return host, port, nil
}

func normalizeHost(h string) (string, netip.Addr, error) {
	if h == "" {
		return "", netip.Addr{}, errors.New("host is empty")
	}

	if strings.HasPrefix(h, "[") {
		addr, err := netip.ParseAddr(h[1 : len(h)-1])
		switch {
		case err != nil || !addr.Is6():
			return "", netip.Addr{}, fmt.Errorf("invalid IPv6 address %q", h)
		case addr.Zone() != "":
			return "", netip.Addr{}, errors.New("IPv6 zone identifiers are not allowed")
		case addr.Is4In6():
			return "", netip.Addr{}, errors.New("IPv4-mapped IPv6 addresses are not allowed; use the IPv4 form")
		}
		return "[" + addr.String() + "]", addr, nil
	}

	h = strings.ToLower(h)
	// Unbracketed hosts cannot contain ":" here, so only IPv4 can parse.
	if addr, err := netip.ParseAddr(h); err == nil {
		return addr.String(), addr, nil
	}
	if !isValidHostname(h) {
		return "", netip.Addr{}, fmt.Errorf("invalid host %q", h)
	}

	labels := strings.Split(h, ".")
	if endsInNumber(labels[len(labels)-1]) {
		return "", netip.Addr{}, fmt.Errorf("invalid IPv4 address %q", h)
	}
	return h, netip.Addr{}, nil
}

func normalizePort(scheme, port string) (string, error) {
	if port == "" {
		return "", nil
	}
	if len(port) > 5 || !isAllDigits(port) {
		return "", fmt.Errorf("invalid port %q", port)
	}
	n, err := strconv.Atoi(port)
	if err != nil || n < 1 || n > 65535 {
		return "", fmt.Errorf("invalid port %q: must be between 1 and 65535", port)
	}
	if (scheme == "http" && n == 80) || (scheme == "https" && n == 443) {
		return "", nil
	}
	return strconv.Itoa(n), nil
}

func checkDeployedOrigin(o origin) error {
	if o.scheme != "https" {
		return errors.New("must use https")
	}
	switch {
	case o.ip.IsValid() && o.ip.IsLoopback():
		return errors.New("loopback addresses are not allowed")
	case o.ip.IsValid() && o.ip.IsUnspecified():
		return errors.New("unspecified addresses such as 0.0.0.0 and [::] are not allowed")
	case o.host == "localhost" || strings.HasSuffix(o.host, ".localhost"):
		return errors.New("localhost hosts are not allowed")
	}
	return nil
}

func isValidHostname(h string) bool {
	if len(h) == 0 || len(h) > 253 {
		return false
	}
	for _, label := range strings.Split(h, ".") {
		if len(label) == 0 || len(label) > 63 {
			return false
		}
		if label[0] == '-' || label[len(label)-1] == '-' {
			return false
		}
		for i := 0; i < len(label); i++ {
			c := label[i]
			if !((c >= 'a' && c <= 'z') || (c >= '0' && c <= '9') || c == '-') {
				return false
			}
		}
	}
	return true
}

func endsInNumber(label string) bool {
	if isAllDigits(label) {
		return true
	}
	if len(label) >= 2 && label[0] == '0' && (label[1] == 'x' || label[1] == 'X') {
		for i := 2; i < len(label); i++ {
			c := label[i]
			if !((c >= '0' && c <= '9') || (c >= 'a' && c <= 'f') || (c >= 'A' && c <= 'F')) {
				return false
			}
		}
		return true
	}
	return false
}

func isAllDigits(s string) bool {
	if s == "" {
		return false
	}
	for i := 0; i < len(s); i++ {
		if s[i] < '0' || s[i] > '9' {
			return false
		}
	}
	return true
}
