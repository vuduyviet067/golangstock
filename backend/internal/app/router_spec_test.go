package app

import (
	"encoding/json"
	"flag"
	"fmt"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/DATA-DOG/go-sqlmock"

	productHTTP "gostock/internal/handle/http"
	"gostock/internal/usecase"
)

// Router behavior specification (XG-01b). Every route of the API is called
// with every common method, with and without a trailing slash, without a
// token, with a user token and with an admin token; unknown and malformed
// paths and CORS cases are added. The observed status, error code and Allow
// and Access-Control-Allow-Origin headers are compared with
// testdata/router_spec.golden, which pins the behavior of the router so that
// a change of router implementation shows up line by line.
//
// Regenerate the golden file (only after reviewing every difference) with:
//
//	go test ./internal/app -run TestRouterSpec -update-router-spec

var updateRouterSpec = flag.Bool("update-router-spec", false, "rewrite testdata/router_spec.golden")

const (
	specAllowedOrigin    = "http://allowed.test"
	specDisallowedOrigin = "http://evil.test"
)

// specRoutes are the routes of the API, with an example path.
var specRoutes = []string{
	"/health",
	"/ready",
	"/api/v1/auth/register",
	"/api/v1/auth/login",
	"/api/v1/auth/refresh",
	"/api/v1/auth/logout",
	"/api/v1/auth/me",
	"/api/v1/products",
	"/api/v1/products/1",
	"/api/v1/products/1/movements",
	"/api/v1/products/1/stock-in",
	"/api/v1/products/1/stock-out",
	"/api/v1/users",
	"/api/v1/users/1",
	"/api/v1/stock-movements",
	"/api/v1/dashboard/summary",
	"/api/v1/dashboard/stock-movements",
}

// specOtherPaths are paths that are not routes of the API, or malformed
// variants of one.
var specOtherPaths = []string{
	"/",
	"/api",
	"/api/v1",
	"/api/v1/unknown",
	"/api/v1/auth",
	"/api/v1/auth/me/x",
	"/api/v1/products/abc",
	"/api/v1/products/0",
	"/api/v1/products/%31",
	"/api/v1/products/1%2Fx",
	"/api/v1/products/1/x",
	"/api/v1/products/1/x/movements",
	"/api/v1/products/1/stock-in/x",
	"/api/v1/users/abc",
	"/api/v1/users/1/extra",
	"/api/v1/stock-movements/1",
	"/api/v1/dashboard",
	"/api/v1/dashboard/unknown",
	"/health/x",
}

var specMethods = []string{
	http.MethodGet, http.MethodHead, http.MethodPost, http.MethodPut,
	http.MethodPatch, http.MethodDelete, http.MethodOptions,
}

var specTokens = []string{"", "user-token", "admin-token"}

// specBody returns a valid JSON body for the write routes, so that a request
// that passes routing and authorization reaches the use case.
func specBody(method, path string) string {
	if method != http.MethodPost && method != http.MethodPut && method != http.MethodPatch {
		return ""
	}
	switch {
	case strings.HasSuffix(path, "/auth/register"):
		return `{"email":"new@example.com","password":"password123"}`
	case strings.HasSuffix(path, "/auth/login"):
		return `{"email":"user@example.com","password":"password123"}`
	case strings.HasSuffix(path, "/stock-in"), strings.HasSuffix(path, "/stock-out"):
		return `{"quantity":1}`
	case strings.HasPrefix(path, "/api/v1/products") && method == http.MethodPost:
		return `{"sku":"SKU-NEW","name":"New","price":10,"quantity":1}`
	case strings.HasPrefix(path, "/api/v1/products"):
		return `{"sku":"SKU-001","name":"Renamed","price":10}`
	case strings.HasPrefix(path, "/api/v1/users"):
		return `{"role":"admin"}`
	}
	return `{}`
}

type specRequest struct {
	method, path, token, origin string
	preflight                   bool // adds Access-Control-Request-Method
}

func (s specRequest) key() string {
	auth := "none"
	switch s.token {
	case "user-token":
		auth = "user"
	case "admin-token":
		auth = "admin"
	}
	origin := "-"
	if s.origin != "" {
		origin = s.origin
		if s.preflight {
			origin += "+preflight"
		}
	}
	return fmt.Sprintf("%-7s %-40s auth=%-5s origin=%s", s.method, s.path, auth, origin)
}

// specOutcome summarizes a response: status, error code (or a marker for a
// success or a non-JSON body), and the Allow and Access-Control-Allow-Origin
// headers.
func specOutcome(rec *httptest.ResponseRecorder) string {
	code := "-"
	body := strings.TrimSpace(rec.Body.String())
	switch {
	case body == "":
		code = "(empty)"
	case strings.HasPrefix(rec.Header().Get("Content-Type"), "application/json"):
		var env map[string]any
		if err := json.Unmarshal([]byte(body), &env); err != nil {
			code = "(invalid json)"
		} else if c, ok := env["code"].(string); ok {
			code = c
		} else if s, _ := env["success"].(bool); s {
			code = "(success)"
		} else {
			code = "(no code)"
		}
	default:
		code = fmt.Sprintf("(text %q)", body)
	}
	return fmt.Sprintf("%d %s allow=%q acao=%q", rec.Code, code,
		rec.Header().Get("Allow"), rec.Header().Get("Access-Control-Allow-Origin"))
}

func specServe(h http.Handler, s specRequest) string {
	body := specBody(s.method, s.path)
	req := httptest.NewRequest(s.method, s.path, strings.NewReader(body))
	if body != "" {
		req.Header.Set("Content-Type", "application/json")
	}
	if s.token != "" {
		req.Header.Set("Authorization", "Bearer "+s.token)
	}
	if s.origin != "" {
		req.Header.Set("Origin", s.origin)
	}
	if s.preflight {
		req.Header.Set("Access-Control-Request-Method", http.MethodPost)
	}
	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, req)
	return specOutcome(rec)
}

// specHandler returns the application handler as app.New builds it: the
// router wrapped by the CORS middleware. withDB adds a database so that
// /ready is registered.
func specHandler(t *testing.T, withDB bool) http.Handler {
	t.Helper()
	cors := productHTTP.CORSMiddleware([]string{specAllowedOrigin})
	if !withDB {
		return cors(setupTestRouter())
	}
	db, _, err := sqlmock.New()
	if err != nil {
		t.Fatalf("sqlmock: %v", err)
	}
	t.Cleanup(func() { _ = db.Close() })

	// The same wiring as setupTestRouter, with a database.
	productRepo := &fakeProductRepo{}
	userRepo := &fakeUserRepo{}
	authUC := usecase.NewAuthUseCase(userRepo, &fakeRefreshTokenRepo{}, &fakeTokenManager{},
		15*time.Minute, 7*24*time.Hour, 15*time.Second)
	router := NewRouter(db,
		productHTTP.NewProductHandler(usecase.NewProductUseCase(productRepo)),
		productHTTP.NewAuthHandler(authUC, nil),
		&fakeAuth{},
		productHTTP.NewUserHandler(usecase.NewUserUseCase(userRepo)),
		productHTTP.NewStockMovementHandler(usecase.NewStockMovementUseCase(productRepo, &fakeStockMovementRepo{})),
		productHTTP.NewDashboardHandler(usecase.NewDashboardUseCase(&fakeDashboardRepo{})))
	return cors(router)
}

func TestRouterSpec(t *testing.T) {
	var lines []string
	add := func(h http.Handler, s specRequest, prefix string) {
		lines = append(lines, prefix+s.key()+" -> "+specServe(h, s))
	}

	withoutDB := specHandler(t, false)
	withDB := specHandler(t, true)

	for _, route := range specRoutes {
		h, prefix := withoutDB, ""
		if route == "/ready" {
			h, prefix = withDB, "[db] "
		}
		for _, path := range []string{route, route + "/"} {
			for _, m := range specMethods {
				for _, tok := range specTokens {
					add(h, specRequest{method: m, path: path, token: tok}, prefix)
				}
			}
		}
	}
	// /ready without a database is not registered.
	for _, m := range []string{http.MethodGet, http.MethodPost} {
		add(withoutDB, specRequest{method: m, path: "/ready"}, "[nodb] ")
	}

	for _, path := range specOtherPaths {
		for _, m := range []string{http.MethodGet, http.MethodPost, http.MethodDelete} {
			for _, tok := range specTokens {
				add(withoutDB, specRequest{method: m, path: path, token: tok}, "")
			}
		}
	}

	// CORS: preflight and simple requests from an allowed and an unknown origin.
	for _, path := range []string{"/health", "/api/v1/auth/login", "/api/v1/products", "/api/v1/users/1", "/api/v1/unknown"} {
		for _, origin := range []string{specAllowedOrigin, specDisallowedOrigin} {
			add(withoutDB, specRequest{method: http.MethodOptions, path: path, origin: origin, preflight: true}, "")
			for _, m := range []string{http.MethodGet, http.MethodPost} {
				for _, tok := range []string{"", "admin-token"} {
					add(withoutDB, specRequest{method: m, path: path, token: tok, origin: origin}, "")
				}
			}
		}
	}

	got := strings.Join(lines, "\n") + "\n"
	golden := filepath.Join("testdata", "router_spec.golden")
	if *updateRouterSpec {
		if err := os.MkdirAll("testdata", 0o755); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(golden, []byte(got), 0o644); err != nil {
			t.Fatal(err)
		}
		t.Logf("wrote %s (%d lines)", golden, len(lines))
		return
	}
	wantBytes, err := os.ReadFile(golden)
	if err != nil {
		t.Fatalf("read %s: %v (run with -update-router-spec to create it)", golden, err)
	}
	want := strings.Split(strings.TrimRight(strings.ReplaceAll(string(wantBytes), "\r\n", "\n"), "\n"), "\n")
	if len(want) != len(lines) {
		t.Errorf("golden has %d lines, the router produced %d", len(want), len(lines))
	}
	diffs := 0
	for i := 0; i < len(want) && i < len(lines); i++ {
		if want[i] != lines[i] {
			diffs++
			t.Errorf("line %d differs\n want: %s\n  got: %s", i+1, want[i], lines[i])
		}
	}
	if diffs > 0 {
		t.Logf("%d of %d lines differ", diffs, len(lines))
	}
}
