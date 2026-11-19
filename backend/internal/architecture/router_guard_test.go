package architecture

import (
	"io/fs"
	"strings"
	"testing"
)

// Router guard (XG-01b): the chi router is an HTTP-layer detail. Only the
// packages that build the router (internal/app) and serve requests
// (internal/handle/http) import github.com/go-chi/chi, in any major version
// or subpackage; use cases, repositories, domain, dto, mapper and pkg stay
// independent of it. Production files only, like the other guards.

const chiModule = "github.com/go-chi/chi"

// routerImporters are the package trees that may import chi.
var routerImporters = []string{handlerPackage, appPackage}

// isChiImport reports whether p is chi or a package of it: the module root
// (v1), a major version module (github.com/go-chi/chi/v5) or a subpackage
// (github.com/go-chi/chi/v5/middleware). github.com/go-chi/cors and other
// go-chi modules are not the router.
func isChiImport(p string) bool {
	return inTree(p, chiModule)
}

// checkRouterImporters reports every production file outside the allowed
// trees that imports chi.
func checkRouterImporters(fsys fs.FS) ([]string, error) {
	return checkRestrictedImport(fsys, isChiImport, routerImporters,
		"only "+strings.Join(routerImporters, " and ")+" use the chi router")
}

func TestOnlyHTTPLayerImportsRouter(t *testing.T) {
	fsys := moduleFS(t)
	requireFiles(t, fsys, handlerPackage)
	requireFiles(t, fsys, appPackage)
	v, err := checkRouterImporters(fsys)
	reportViolations(t, v, err, "Keep routing in internal/app and path parameters in internal/handle/http (pathParam); "+
		"pass plain values to the use cases.")
}
