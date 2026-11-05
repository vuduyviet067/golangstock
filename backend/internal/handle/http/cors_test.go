package http

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"gostock/internal/handle/http/response"
)

// testAllowedOrigins is a canonical allowlist, as config.Load would produce it.
var testAllowedOrigins = []string{
	"http://localhost:5173",
	"http://localhost:3000",
	"https://app.gostock.com",
}

var writeMethods = []string{http.MethodPost, http.MethodPut, http.MethodPatch, http.MethodDelete}

// newCORSTestHandler wraps a downstream handler that records whether it ran.
func newCORSTestHandler(allowed []string) (http.Handler, *bool) {
	called := new(bool)
	next := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		*called = true
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte("ok"))
	})
	return CORSMiddleware(allowed)(next), called
}

// serveCORS sends a request with the given Origin; an empty origin sends none.
func serveCORS(h http.Handler, method, path, origin string) *httptest.ResponseRecorder {
	req := httptest.NewRequest(method, path, nil)
	if origin != "" {
		req.Header.Set("Origin", origin)
	}
	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, req)
	return rec
}

// countVaryOrigin counts "Origin" tokens across all Vary values, case-insensitively.
func countVaryOrigin(h http.Header) int {
	n := 0
	for _, v := range h.Values("Vary") {
		for _, part := range strings.Split(v, ",") {
			if strings.EqualFold(strings.TrimSpace(part), "Origin") {
				n++
			}
		}
	}
	return n
}

func assertVaryOriginOnce(t *testing.T, rec *httptest.ResponseRecorder) {
	t.Helper()
	if n := countVaryOrigin(rec.Header()); n != 1 {
		t.Errorf("Vary must list Origin exactly once, got %d in %q", n, rec.Header().Values("Vary"))
	}
}

func assertNoCORSHeaders(t *testing.T, rec *httptest.ResponseRecorder) {
	t.Helper()
	for _, h := range []string{
		"Access-Control-Allow-Origin",
		"Access-Control-Allow-Credentials",
		"Access-Control-Allow-Methods",
		"Access-Control-Allow-Headers",
	} {
		if got := rec.Header().Get(h); got != "" {
			t.Errorf("%s = %q, want it absent", h, got)
		}
	}
}

// assertForbiddenEnvelope checks the project's standard 403 error envelope.
func assertForbiddenEnvelope(t *testing.T, rec *httptest.ResponseRecorder) {
	t.Helper()
	if rec.Code != http.StatusForbidden {
		t.Fatalf("status = %d, want 403", rec.Code)
	}
	if ct := rec.Header().Get("Content-Type"); !strings.HasPrefix(ct, "application/json") {
		t.Errorf("Content-Type = %q, want application/json", ct)
	}
	var resp response.Response
	if err := json.Unmarshal(rec.Body.Bytes(), &resp); err != nil {
		t.Fatalf("decode error envelope: %v (body %q)", err, rec.Body.String())
	}
	if resp.Success {
		t.Error("success = true, want false")
	}
	if resp.Message != "forbidden: cross-origin request not allowed" {
		t.Errorf("message = %q, want the standard cross-origin message", resp.Message)
	}
}

func TestCORSMiddleware_AllowedOriginReachesHandlerForAllMethods(t *testing.T) {
	methods := []string{http.MethodGet, http.MethodHead, http.MethodPost, http.MethodPut, http.MethodPatch, http.MethodDelete}

	for _, origin := range testAllowedOrigins {
		for _, method := range methods {
			t.Run(origin+"/"+method, func(t *testing.T) {
				h, called := newCORSTestHandler(testAllowedOrigins)
				rec := serveCORS(h, method, "/api/v1/products", origin)

				if !*called {
					t.Fatal("downstream handler was not called for an allowed origin")
				}
				if rec.Code != http.StatusOK {
					t.Fatalf("status = %d, want 200", rec.Code)
				}
				if got := rec.Header().Get("Access-Control-Allow-Origin"); got != origin {
					t.Errorf("Access-Control-Allow-Origin = %q, want exactly %q", got, origin)
				}
				if got := rec.Header().Get("Access-Control-Allow-Credentials"); got != "true" {
					t.Errorf("Access-Control-Allow-Credentials = %q, want true", got)
				}
				assertVaryOriginOnce(t, rec)
			})
		}
	}
}

func TestCORSMiddleware_UnknownOriginWritesRejected(t *testing.T) {
	for _, method := range writeMethods {
		t.Run(method, func(t *testing.T) {
			h, called := newCORSTestHandler(testAllowedOrigins)
			rec := serveCORS(h, method, "/api/v1/users/1", "http://evil.com")

			if *called {
				t.Fatalf("downstream handler must not run for a %s from an unknown origin", method)
			}
			assertForbiddenEnvelope(t, rec)
			assertNoCORSHeaders(t, rec)
			assertVaryOriginOnce(t, rec)
		})
	}
}

func TestCORSMiddleware_UnknownOriginSafeMethodsPassWithoutCORSHeaders(t *testing.T) {
	for _, method := range []string{http.MethodGet, http.MethodHead} {
		t.Run(method, func(t *testing.T) {
			h, called := newCORSTestHandler(testAllowedOrigins)
			rec := serveCORS(h, method, "/api/v1/products", "http://evil.com")

			if !*called {
				t.Fatal("downstream handler should run; the browser enforces CORS on the response")
			}
			if rec.Code != http.StatusOK {
				t.Fatalf("status = %d, want 200", rec.Code)
			}
			assertNoCORSHeaders(t, rec)
			assertVaryOriginOnce(t, rec)
		})
	}
}

func TestCORSMiddleware_NoOriginPassesThroughAllMethods(t *testing.T) {
	// Non-browser clients (curl, server-to-server) send no Origin and must not be blocked.
	methods := []string{http.MethodGet, http.MethodHead, http.MethodPost, http.MethodPut, http.MethodPatch, http.MethodDelete}
	for _, method := range methods {
		t.Run(method, func(t *testing.T) {
			h, called := newCORSTestHandler(testAllowedOrigins)
			rec := serveCORS(h, method, "/api/v1/products", "")

			if !*called {
				t.Fatalf("downstream handler must run for a %s without Origin", method)
			}
			if rec.Code != http.StatusOK {
				t.Fatalf("status = %d, want 200", rec.Code)
			}
			assertNoCORSHeaders(t, rec)
			assertVaryOriginOnce(t, rec)
		})
	}
}

func TestCORSMiddleware_Preflight(t *testing.T) {
	newPreflight := func(origin string) *http.Request {
		req := httptest.NewRequest(http.MethodOptions, "/api/v1/users/1", nil)
		if origin != "" {
			req.Header.Set("Origin", origin)
		}
		req.Header.Set("Access-Control-Request-Method", http.MethodPatch)
		req.Header.Set("Access-Control-Request-Headers", "Content-Type, Authorization")
		return req
	}

	t.Run("allowed origin", func(t *testing.T) {
		h, called := newCORSTestHandler(testAllowedOrigins)
		rec := httptest.NewRecorder()
		h.ServeHTTP(rec, newPreflight("http://localhost:5173"))

		if *called {
			t.Error("preflight must be answered by the middleware, not the router")
		}
		if rec.Code != http.StatusNoContent {
			t.Fatalf("status = %d, want 204", rec.Code)
		}
		if rec.Body.Len() != 0 {
			t.Errorf("body = %q, want empty", rec.Body.String())
		}
		if got := rec.Header().Get("Access-Control-Allow-Origin"); got != "http://localhost:5173" {
			t.Errorf("Access-Control-Allow-Origin = %q, want http://localhost:5173", got)
		}
		if got := rec.Header().Get("Access-Control-Allow-Credentials"); got != "true" {
			t.Errorf("Access-Control-Allow-Credentials = %q, want true", got)
		}
		methods := rec.Header().Get("Access-Control-Allow-Methods")
		for _, m := range []string{"GET", "POST", "PUT", "PATCH", "DELETE", "OPTIONS"} {
			if !strings.Contains(methods, m) {
				t.Errorf("Access-Control-Allow-Methods %q is missing %s", methods, m)
			}
		}
		allowedHeaders := rec.Header().Get("Access-Control-Allow-Headers")
		for _, hdr := range []string{"Content-Type", "Authorization"} {
			if !strings.Contains(allowedHeaders, hdr) {
				t.Errorf("Access-Control-Allow-Headers %q is missing %s", allowedHeaders, hdr)
			}
		}
		assertVaryOriginOnce(t, rec)
	})

	t.Run("disallowed origin", func(t *testing.T) {
		h, called := newCORSTestHandler(testAllowedOrigins)
		rec := httptest.NewRecorder()
		h.ServeHTTP(rec, newPreflight("http://evil.com"))

		if *called {
			t.Error("preflight must not reach the router")
		}
		if rec.Code != http.StatusNoContent {
			t.Fatalf("status = %d, want 204", rec.Code)
		}
		assertNoCORSHeaders(t, rec)
		assertVaryOriginOnce(t, rec)
	})

	t.Run("no origin", func(t *testing.T) {
		h, called := newCORSTestHandler(testAllowedOrigins)
		rec := httptest.NewRecorder()
		h.ServeHTTP(rec, newPreflight(""))

		if *called {
			t.Error("OPTIONS must not reach the router")
		}
		if rec.Code != http.StatusNoContent {
			t.Fatalf("status = %d, want 204", rec.Code)
		}
		assertNoCORSHeaders(t, rec)
	})
}

// TestCORSMiddleware_RequestOriginMatchedExactly verifies that request Origin
// values are compared exactly and never normalized into an allowed origin.
func TestCORSMiddleware_RequestOriginMatchedExactly(t *testing.T) {
	nonMatching := []string{
		"HTTP://LOCALHOST:5173",                 // case differs
		"http://LocalHost:5173",                 // host case differs
		"http://localhost:5173/",                // trailing slash
		"https://app.gostock.com:443",           // explicit default port
		"http://localhost:5173:5173",            // malformed port
		"http://localhost",                      // different port
		"https://localhost:5173",                // different scheme
		"http://localhost:5173.evil.com",        // allowed origin as a prefix
		"http://evil.com/http://localhost:5173", // allowed origin as a suffix
		"http://user@localhost:5173",            // userinfo
		"null",
	}

	for _, origin := range nonMatching {
		t.Run(origin, func(t *testing.T) {
			t.Run("GET passes without CORS headers", func(t *testing.T) {
				h, called := newCORSTestHandler(testAllowedOrigins)
				rec := serveCORS(h, http.MethodGet, "/api/v1/products", origin)
				if !*called {
					t.Fatal("downstream handler should run for GET")
				}
				assertNoCORSHeaders(t, rec)
			})
			t.Run("POST rejected", func(t *testing.T) {
				h, called := newCORSTestHandler(testAllowedOrigins)
				rec := serveCORS(h, http.MethodPost, "/api/v1/products", origin)
				if *called {
					t.Fatal("downstream handler must not run")
				}
				assertForbiddenEnvelope(t, rec)
			})
		})
	}
}

func TestCORSMiddleware_EmptyAllowlist(t *testing.T) {
	h, called := newCORSTestHandler(nil)

	rec := serveCORS(h, http.MethodPost, "/api/v1/products", "http://localhost:5173")
	if *called {
		t.Fatal("with no allowed origins, a cross-origin write must not reach the handler")
	}
	assertForbiddenEnvelope(t, rec)

	*called = false
	rec = serveCORS(h, http.MethodPost, "/api/v1/products", "")
	if !*called || rec.Code != http.StatusOK {
		t.Fatalf("request without Origin must pass through: called=%v status=%d", *called, rec.Code)
	}
}

func TestCORSMiddleware_VaryHeader(t *testing.T) {
	tests := []struct {
		name     string
		existing []string // Vary lines set before the middleware runs
		origin   string
	}{
		{name: "added when absent"},
		{name: "added when absent with Origin", origin: "http://localhost:5173"},
		{name: "kept alongside another value", existing: []string{"Accept-Encoding"}},
		{name: "not duplicated when already present", existing: []string{"Origin"}},
		{name: "not duplicated when present in lowercase", existing: []string{"origin"}, origin: "http://localhost:5173"},
		{name: "not duplicated inside a comma list", existing: []string{"Accept-Encoding, ORIGIN"}},
		{name: "multiple lines preserved", existing: []string{"Accept-Encoding", "Cookie"}, origin: "http://evil.com"},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			cors, _ := newCORSTestHandler(testAllowedOrigins)
			// Simulate an outer layer that set Vary before the CORS middleware ran.
			h := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				for _, v := range tc.existing {
					w.Header().Add("Vary", v)
				}
				cors.ServeHTTP(w, r)
			})
			rec := serveCORS(h, http.MethodGet, "/api/v1/products", tc.origin)

			assertVaryOriginOnce(t, rec)
			got := strings.Join(rec.Header().Values("Vary"), ", ")
			for _, v := range tc.existing {
				if !strings.Contains(got, v) {
					t.Errorf("Vary %q lost existing value %q", got, v)
				}
			}
		})
	}

	t.Run("value added by the downstream handler is preserved", func(t *testing.T) {
		next := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			w.Header().Add("Vary", "Accept-Encoding")
			w.WriteHeader(http.StatusOK)
		})
		rec := serveCORS(CORSMiddleware(testAllowedOrigins)(next), http.MethodGet, "/data", "")

		got := strings.Join(rec.Header().Values("Vary"), ", ")
		if !strings.Contains(got, "Accept-Encoding") {
			t.Errorf("Vary %q lost Accept-Encoding", got)
		}
		assertVaryOriginOnce(t, rec)
	})
}
