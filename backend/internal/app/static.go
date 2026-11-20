package app

import (
	"io"
	"io/fs"
	"net/http"
	"path"
	"strings"
)

// Static files (DEP-02). When STATIC_DIR is set, the API server also serves
// the built frontend, so that the application and its API share one origin
// and the SameSite=Lax authentication cookies need no cross-site setup.
//
// The handler sits between the CORS middleware and the API router:
//
//   - API paths (/api and below, /health, /ready and below) and every method
//     other than GET and HEAD go to the router unchanged, so an unknown API
//     path still gets the 404 NOT_FOUND envelope and never index.html;
//   - a GET or HEAD for a regular file of the directory serves that file;
//   - a GET or HEAD for a path without an extension (a route of the React
//     application, such as /products) serves index.html;
//   - anything else (a missing /assets/missing.js, a malformed path) goes to
//     the router, which answers 404 NOT_FOUND as before.
//
// Files are read through an os.Root (see newApp), so a path can never reach a
// file outside the directory, through ".." or through a symbolic link.

const (
	staticIndexFile = "index.html"

	// Files under assets/ carry a content hash in their name (Vite build
	// output), so they never change and may be cached for a year.
	staticAssetsPrefix    = "assets/"
	cacheControlImmutable = "public, max-age=31536000, immutable"
	// index.html and the other files must be revalidated on every load, so a
	// new deployment is picked up at once.
	cacheControlNoCache = "no-cache"
)

// newStaticHandler returns a handler that serves the files of fsys and passes
// every other request to api.
func newStaticHandler(api http.Handler, fsys fs.FS) http.Handler {
	return &staticHandler{api: api, fsys: fsys}
}

type staticHandler struct {
	api  http.Handler
	fsys fs.FS
}

func (h *staticHandler) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet && r.Method != http.MethodHead {
		h.api.ServeHTTP(w, r)
		return
	}

	cleaned := path.Clean("/" + r.URL.Path)
	if isAPIPath(r.URL.Path) || isAPIPath(cleaned) {
		h.api.ServeHTTP(w, r)
		return
	}

	name, ok := staticFileName(cleaned)
	if !ok {
		h.api.ServeHTTP(w, r)
		return
	}
	if h.serveFile(w, r, name) {
		return
	}
	if path.Ext(name) == "" && h.serveFile(w, r, staticIndexFile) {
		return
	}
	h.api.ServeHTTP(w, r)
}

// isAPIPath reports whether p belongs to the API router: /api, /health and
// /ready, and any path below them.
func isAPIPath(p string) bool {
	for _, prefix := range []string{"/api", "/health", "/ready"} {
		if p == prefix || strings.HasPrefix(p, prefix+"/") {
			return true
		}
	}
	return false
}

// staticFileName turns a cleaned URL path into a name for fsys: "/" is
// index.html, and the name must be a valid fs.FS path without a backslash, a
// colon or a NUL byte (Windows separators and volume names).
func staticFileName(cleaned string) (string, bool) {
	name := strings.TrimPrefix(cleaned, "/")
	if name == "" {
		return staticIndexFile, true
	}
	if strings.ContainsAny(name, "\\:\x00") || !fs.ValidPath(name) {
		return "", false
	}
	return name, true
}

// serveFile serves the regular file name of fsys and reports whether it did.
// A missing file, a directory or any read error leaves the response untouched.
func (h *staticHandler) serveFile(w http.ResponseWriter, r *http.Request, name string) bool {
	f, err := h.fsys.Open(name)
	if err != nil {
		return false
	}
	defer f.Close()

	info, err := f.Stat()
	if err != nil || !info.Mode().IsRegular() {
		return false
	}
	content, ok := f.(io.ReadSeeker)
	if !ok {
		return false
	}

	header := w.Header()
	header.Set("X-Content-Type-Options", "nosniff")
	if strings.HasPrefix(name, staticAssetsPrefix) {
		header.Set("Cache-Control", cacheControlImmutable)
	} else {
		header.Set("Cache-Control", cacheControlNoCache)
	}
	http.ServeContent(w, r, info.Name(), info.ModTime(), content)
	return true
}
