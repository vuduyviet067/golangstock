package app

import (
	"database/sql"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/DATA-DOG/go-sqlmock"
)

// Static files (DEP-02): the application handler built by newApp with
// STATIC_DIR set, on a temporary directory laid out like the frontend build.

const (
	staticTestIndex  = "<!doctype html><title>GoStock static test</title>"
	staticTestAppJS  = "console.log('gostock static test')"
	staticTestSecret = "OUTSIDE-STATIC-DIR-SECRET"
)

// staticTestDir creates <tmp>/dist with index.html and assets/app.js, and
// files outside it (<tmp>/.env and <tmp>/secret.txt) that no request may
// read. It returns the dist directory.
func staticTestDir(t *testing.T) string {
	t.Helper()
	root := t.TempDir()
	dist := filepath.Join(root, "dist")
	writeStaticTestFile(t, filepath.Join(dist, "index.html"), staticTestIndex)
	writeStaticTestFile(t, filepath.Join(dist, "assets", "app.js"), staticTestAppJS)
	writeStaticTestFile(t, filepath.Join(root, ".env"), "SECRET="+staticTestSecret)
	writeStaticTestFile(t, filepath.Join(root, "secret.txt"), staticTestSecret)
	return dist
}

func writeStaticTestFile(t *testing.T, name, content string) {
	t.Helper()
	if err := os.MkdirAll(filepath.Dir(name), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(name, []byte(content), 0o644); err != nil {
		t.Fatal(err)
	}
}

// staticTestHandler builds the application with newApp (sqlmock database,
// no query expected) and STATIC_DIR set to dir, or unset when dir is "".
func staticTestHandler(t *testing.T, dir string) http.Handler {
	t.Helper()
	if dir == "" {
		t.Setenv("STATIC_DIR", "")
		if err := os.Unsetenv("STATIC_DIR"); err != nil {
			t.Fatal(err)
		}
	} else {
		t.Setenv("STATIC_DIR", dir)
	}

	db, mock, err := sqlmock.New()
	if err != nil {
		t.Fatalf("sqlmock: %v", err)
	}
	mock.ExpectClose()

	application, err := newApp(startTestConfig(), func(string) (*sql.DB, error) { return db, nil }, nil)
	if err != nil {
		t.Fatalf("newApp: %v", err)
	}
	t.Cleanup(func() { _ = application.Close() })
	return application.server.Handler
}

func staticServe(h http.Handler, method, target string) *httptest.ResponseRecorder {
	req := httptest.NewRequest(method, target, nil)
	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, req)
	return rec
}

// envelopeCode returns the error code of a JSON envelope, or fails.
func envelopeCode(t *testing.T, rec *httptest.ResponseRecorder) string {
	t.Helper()
	if ct := rec.Header().Get("Content-Type"); !strings.HasPrefix(ct, "application/json") {
		t.Fatalf("Content-Type = %q, want a JSON envelope; body %q", ct, rec.Body.String())
	}
	var env struct {
		Success bool   `json:"success"`
		Code    string `json:"code"`
	}
	if err := json.Unmarshal(rec.Body.Bytes(), &env); err != nil {
		t.Fatalf("body is not a JSON envelope: %v; body %q", err, rec.Body.String())
	}
	if env.Success {
		t.Fatalf("envelope success = true, want an error; body %q", rec.Body.String())
	}
	return env.Code
}

func TestStatic_ServesIndexForRootAndReactRoutes(t *testing.T) {
	h := staticTestHandler(t, staticTestDir(t))

	for _, target := range []string{"/", "/index.html", "/products", "/dashboard/stock", "/assets"} {
		t.Run(target, func(t *testing.T) {
			rec := staticServe(h, http.MethodGet, target)
			if rec.Code != http.StatusOK {
				t.Fatalf("status = %d, want 200; body %q", rec.Code, rec.Body.String())
			}
			if rec.Body.String() != staticTestIndex {
				t.Errorf("body = %q, want index.html", rec.Body.String())
			}
			if got := rec.Header().Get("Content-Type"); !strings.HasPrefix(got, "text/html") {
				t.Errorf("Content-Type = %q, want text/html", got)
			}
			if got := rec.Header().Get("Cache-Control"); got != "no-cache" {
				t.Errorf("Cache-Control = %q, want no-cache", got)
			}
			if got := rec.Header().Get("X-Content-Type-Options"); got != "nosniff" {
				t.Errorf("X-Content-Type-Options = %q, want nosniff", got)
			}
		})
	}

	t.Run("HEAD /products", func(t *testing.T) {
		rec := staticServe(h, http.MethodHead, "/products")
		if rec.Code != http.StatusOK || rec.Body.Len() != 0 {
			t.Errorf("HEAD = %d with %d body bytes, want 200 without a body", rec.Code, rec.Body.Len())
		}
		if got := rec.Header().Get("Cache-Control"); got != "no-cache" {
			t.Errorf("Cache-Control = %q, want no-cache", got)
		}
	})
}

func TestStatic_ServesAssetsImmutable(t *testing.T) {
	h := staticTestHandler(t, staticTestDir(t))

	rec := staticServe(h, http.MethodGet, "/assets/app.js")
	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, want 200; body %q", rec.Code, rec.Body.String())
	}
	if rec.Body.String() != staticTestAppJS {
		t.Errorf("body = %q, want assets/app.js", rec.Body.String())
	}
	if got := rec.Header().Get("Content-Type"); !strings.Contains(got, "javascript") {
		t.Errorf("Content-Type = %q, want JavaScript", got)
	}
	if got := rec.Header().Get("Cache-Control"); got != "public, max-age=31536000, immutable" {
		t.Errorf("Cache-Control = %q, want public, max-age=31536000, immutable", got)
	}
	if got := rec.Header().Get("X-Content-Type-Options"); got != "nosniff" {
		t.Errorf("X-Content-Type-Options = %q, want nosniff", got)
	}
}

func TestStatic_NotFoundCasesKeepTheEnvelope(t *testing.T) {
	h := staticTestHandler(t, staticTestDir(t))

	cases := []struct{ method, target string }{
		{http.MethodGet, "/assets/missing.js"},
		{http.MethodHead, "/assets/missing.js"},
		{http.MethodGet, "/favicon.ico"},
		{http.MethodGet, "/api"},
		{http.MethodGet, "/api/"},
		{http.MethodGet, "/api/v1"},
		{http.MethodGet, "/api/v1/khong-ton-tai"},
		{http.MethodGet, "/api/v1/products/1/x"},
		{http.MethodGet, "/x/../api/v1/khong-ton-tai"},
		{http.MethodGet, "/health/x"},
		{http.MethodGet, "/ready/x"},
		{http.MethodPost, "/bat-ky"},
		{http.MethodPost, "/"},
		{http.MethodPut, "/products"},
		{http.MethodDelete, "/index.html"},
		{http.MethodPatch, "/assets/app.js"},
	}
	for _, tc := range cases {
		t.Run(tc.method+" "+tc.target, func(t *testing.T) {
			rec := staticServe(h, tc.method, tc.target)
			if rec.Code != http.StatusNotFound {
				t.Fatalf("status = %d, want 404; body %q", rec.Code, rec.Body.String())
			}
			if strings.Contains(rec.Body.String(), staticTestIndex) || strings.Contains(rec.Body.String(), staticTestAppJS) {
				t.Fatalf("a 404 served a static file: %q", rec.Body.String())
			}
			if tc.method == http.MethodHead {
				return // no body to decode
			}
			if code := envelopeCode(t, rec); code != "NOT_FOUND" {
				t.Errorf("code = %q, want NOT_FOUND", code)
			}
		})
	}
}

// API paths answer exactly as without STATIC_DIR.
func TestStatic_APIUnchanged(t *testing.T) {
	dir := staticTestDir(t)

	targets := []struct{ method, target string }{
		{http.MethodGet, "/health"},
		{http.MethodHead, "/health"},
		{http.MethodPost, "/health"},
		{http.MethodGet, "/api/v1/khong-ton-tai"},
		{http.MethodGet, "/api/v1/auth/me"},
		{http.MethodGet, "/api/v1/products"},
		{http.MethodPost, "/api/v1/auth/register"},
		{http.MethodPost, "/bat-ky"},
		{http.MethodOptions, "/products"},
	}

	type outcome struct {
		code        int
		contentType string
		body        string
	}
	observe := func(h http.Handler) []outcome {
		var out []outcome
		for _, tc := range targets {
			rec := staticServe(h, tc.method, tc.target)
			out = append(out, outcome{rec.Code, rec.Header().Get("Content-Type"), rec.Body.String()})
		}
		return out
	}

	without := observe(staticTestHandler(t, ""))
	with := observe(staticTestHandler(t, dir))
	for i, tc := range targets {
		if with[i] != without[i] {
			t.Errorf("%s %s with STATIC_DIR = %+v, without = %+v", tc.method, tc.target, with[i], without[i])
		}
	}

	// /health keeps its JSON body.
	rec := staticServe(staticTestHandler(t, dir), http.MethodGet, "/health")
	if rec.Code != http.StatusOK || !strings.HasPrefix(rec.Header().Get("Content-Type"), "application/json") ||
		!strings.Contains(rec.Body.String(), `"status":"ok"`) {
		t.Errorf("GET /health = %d %q %q, want the JSON health envelope", rec.Code, rec.Header().Get("Content-Type"), rec.Body.String())
	}
}

func TestStatic_WithoutStaticDirServesNothing(t *testing.T) {
	h := staticTestHandler(t, "")
	for _, target := range []string{"/", "/index.html", "/products", "/assets/app.js"} {
		rec := staticServe(h, http.MethodGet, target)
		if rec.Code != http.StatusNotFound {
			t.Errorf("GET %s = %d, want 404", target, rec.Code)
			continue
		}
		if code := envelopeCode(t, rec); code != "NOT_FOUND" {
			t.Errorf("GET %s code = %q, want NOT_FOUND", target, code)
		}
	}
}

func TestStatic_PathTraversalCannotLeaveTheDirectory(t *testing.T) {
	dir := staticTestDir(t)
	h := staticTestHandler(t, dir)

	targets := []string{
		"/../.env",
		"/../secret.txt",
		"/assets/../../secret.txt",
		"/%2e%2e/.env",
		"/%2e%2e/secret.txt",
		"/%2e%2e%2fsecret.txt",
		"/..%2fsecret.txt",
		"/..%2f.env",
		"/%2e%2e%2f%2e%2e%2fsecret.txt",
		"/..%5csecret.txt",
		"/..%5c.env",
		"/assets%5c..%5c..%5csecret.txt",
		"/%2e%2e%5csecret.txt",
		"/C:%5cWindows%5cwin.ini",
		"/secret.txt%00.js",
		"/.env",
	}
	for _, target := range targets {
		t.Run(target, func(t *testing.T) {
			rec := staticServe(h, http.MethodGet, target)
			if strings.Contains(rec.Body.String(), staticTestSecret) {
				t.Fatalf("GET %s read a file outside STATIC_DIR: %d %q", target, rec.Code, rec.Body.String())
			}
			if rec.Code != http.StatusNotFound {
				t.Errorf("GET %s = %d, want 404; body %q", target, rec.Code, rec.Body.String())
			}
		})
	}
}

// Symbolic links inside STATIC_DIR that point outside of it, to a file and to
// a directory, must not expose what they point to: os.Root refuses to follow
// a link out of the root. Creating a link may need a privilege (Windows
// without Developer Mode); the test is skipped then.
func TestStatic_SymlinkCannotLeaveTheDirectory(t *testing.T) {
	dir := staticTestDir(t)
	outside := filepath.Dir(dir)

	links := []struct{ link, target, request string }{
		{filepath.Join(dir, "link.txt"), filepath.Join(outside, "secret.txt"), "/link.txt"},
		{filepath.Join(dir, "linkdir"), outside, "/linkdir/secret.txt"},
	}
	for _, l := range links {
		if err := os.Symlink(l.target, l.link); err != nil {
			t.Skipf("cannot create a symbolic link on this system: %v", err)
		}
	}

	h := staticTestHandler(t, dir)
	for _, l := range links {
		t.Run(l.request, func(t *testing.T) {
			rec := staticServe(h, http.MethodGet, l.request)
			if strings.Contains(rec.Body.String(), staticTestSecret) {
				t.Fatalf("GET %s followed a symbolic link outside STATIC_DIR: %d %q", l.request, rec.Code, rec.Body.String())
			}
			if rec.Code != http.StatusNotFound {
				t.Errorf("GET %s = %d, want 404; body %q", l.request, rec.Code, rec.Body.String())
			}
		})
	}
}

func TestStatic_InvalidStaticDirFailsBeforeTheDatabase(t *testing.T) {
	noIndex := t.TempDir()
	for name, value := range map[string]string{
		"missing directory":  filepath.Join(noIndex, "does-not-exist"),
		"without index.html": noIndex,
	} {
		t.Run(name, func(t *testing.T) {
			t.Setenv("STATIC_DIR", value)
			opener, calls := countingOpener(nil)
			application, err := newApp(startTestConfig(), opener, nil)
			if err == nil {
				_ = application.Close()
				t.Fatal("newApp succeeded, want a STATIC_DIR error")
			}
			if !strings.Contains(err.Error(), "STATIC_DIR") {
				t.Errorf("error = %v, want it to mention STATIC_DIR", err)
			}
			if *calls != 0 {
				t.Errorf("database opened %d times, want 0", *calls)
			}
		})
	}
}
