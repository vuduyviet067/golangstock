package http

import (
	"net/http"
	"net/url"
	"strings"
	"sync"

	"github.com/go-chi/chi/v5"

	"gostock/pkg/response"
)

// Routing support for the chi router built in internal/app: the path
// parameter helper used by the handlers, and the central 404 and 405
// responses.

// pathParam returns the path parameter name of the matched route, decoded
// (chi matches the escaped path, so "%31" is returned as "1"). It is the only
// place where handlers read path parameters. A value that cannot be decoded
// is returned as "", which the handlers reject like a missing parameter.
func pathParam(r *http.Request, name string) string {
	value, err := url.PathUnescape(chi.URLParam(r, name))
	if err != nil {
		return ""
	}
	return value
}

// routeMethods are the methods a route can be registered for, in the order
// they are listed in an Allow header.
var routeMethods = []string{
	http.MethodGet, http.MethodHead, http.MethodPost, http.MethodPut, http.MethodPatch,
	http.MethodDelete, http.MethodConnect, http.MethodOptions, http.MethodTrace,
}

// RouteTable answers which methods a path is routed for. It is a flat copy of
// the router's routes, taken with chi.Walk on first use, when every route is
// registered. The router's own Match cannot answer this: at the root of a
// mounted subrouter (e.g. "/api/v1/products") chi matches its mount stub for
// every method. In the copy a route keeps no trailing slash, so
// "/api/v1/products/" is not the route "/api/v1/products".
type RouteTable struct {
	routes chi.Routes
	once   sync.Once
	flat   *chi.Mux
}

// NewRouteTable returns the route table of routes.
func NewRouteTable(routes chi.Routes) *RouteTable {
	return &RouteTable{routes: routes}
}

func (t *RouteTable) build() {
	t.flat = chi.NewRouter()
	noop := http.HandlerFunc(func(http.ResponseWriter, *http.Request) {})
	_ = chi.Walk(t.routes, func(method, route string, _ http.Handler, _ ...func(http.Handler) http.Handler) error {
		if len(route) > 1 {
			route = strings.TrimSuffix(route, "/")
		}
		t.flat.Method(method, route, noop)
		return nil
	})
}

// Methods lists the methods path is routed for, in Allow header order.
func (t *RouteTable) Methods(path string) []string {
	t.once.Do(t.build)
	var methods []string
	for _, m := range routeMethods {
		if t.flat.Match(chi.NewRouteContext(), m, path) {
			methods = append(methods, m)
		}
	}
	return methods
}

// routePath is the path chi routes on: the escaped path when it differs from
// the default encoding, the decoded path otherwise.
func routePath(r *http.Request) string {
	if r.URL.RawPath != "" {
		return r.URL.RawPath
	}
	if r.URL.Path == "" {
		return "/"
	}
	return r.URL.Path
}

// RequireRoute answers 404 before any other middleware when the path is not a
// route for any method, so an unknown path is NOT_FOUND whether or not the
// request is authenticated.
func (t *RouteTable) RequireRoute(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if len(t.Methods(routePath(r))) == 0 {
			NotFound(w, r)
			return
		}
		next.ServeHTTP(w, r)
	})
}

// MethodNotAllowed writes the 405 response, with the methods of the path in
// the Allow header as "GET, POST".
func (t *RouteTable) MethodNotAllowed(w http.ResponseWriter, r *http.Request) {
	if methods := t.Methods(routePath(r)); len(methods) > 0 {
		w.Header().Set("Allow", strings.Join(methods, ", "))
	}
	response.Error(w, http.StatusMethodNotAllowed, response.CodeMethodNotAllowed, "method not allowed")
}

// NotFound writes the 404 response for a path that is not a route.
func NotFound(w http.ResponseWriter, r *http.Request) {
	response.Error(w, http.StatusNotFound, response.CodeNotFound, "not found")
}
