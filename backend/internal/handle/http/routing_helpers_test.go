package http

import (
	"context"
	"io"
	"net/http"
	"net/http/httptest"

	"github.com/go-chi/chi/v5"
)

// Helpers for tests that call a handler directly. Since the move to chi
// (XG-01b) the router, not the handler, reads the path parameters and checks
// the method; these helpers give a directly called handler the same input.

// paramRoutes holds the routes of the API that have path parameters, for
// every method. Only their patterns matter.
var paramRoutes = func() *chi.Mux {
	r := chi.NewRouter()
	noop := func(http.ResponseWriter, *http.Request) {}
	for _, pattern := range []string{
		"/api/v1/products/{id}",
		"/api/v1/products/{id}/movements",
		"/api/v1/products/{id}/stock-in",
		"/api/v1/products/{id}/stock-out",
		"/api/v1/users/{id}",
	} {
		r.HandleFunc(pattern, noop)
	}
	return r
}()

// newRequest is httptest.NewRequest for a handler called directly. When the
// target matches a route of the API with path parameters, the request carries
// them as the router sets them; any other target carries none, so the
// handler sees a missing parameter.
func newRequest(method, target string, body io.Reader) *http.Request {
	return withRouteParams(httptest.NewRequest(method, target, body))
}

// withRouteParams attaches the chi route context of req's path.
func withRouteParams(req *http.Request) *http.Request {
	rctx := chi.NewRouteContext()
	if !paramRoutes.Match(rctx, http.MethodGet, routePath(req)) {
		rctx = chi.NewRouteContext()
	}
	return req.WithContext(context.WithValue(req.Context(), chi.RouteCtxKey, rctx))
}

// serveRoute serves req through a chi router with h registered for method on
// pattern and the application's 405 handler, so a wrong method gets the same
// 405 response and Allow header as through the application router. A path
// that does not match pattern still reaches h, without path parameters, so
// the handler's own rejection of a missing or malformed ID stays covered.
func serveRoute(w http.ResponseWriter, req *http.Request, method, pattern string, h http.HandlerFunc) {
	r := chi.NewRouter()
	routes := NewRouteTable(r)
	r.MethodNotAllowed(routes.MethodNotAllowed)
	r.NotFound(h)
	r.Method(method, pattern, h)
	// Drop a route context set by newRequest: this router builds its own.
	req = req.WithContext(context.WithValue(req.Context(), chi.RouteCtxKey, (*chi.Context)(nil)))
	r.ServeHTTP(w, req)
}
