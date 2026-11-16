package architecture

import (
	"go/parser"
	"go/token"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"testing"
)

const mapperPackage = "internal/mapper"

// allowedMapperImports are the only module packages internal/mapper may
// import; anything else must come from the standard library. The mapper's
// external tests (package mapper_test) may also import the mapper itself.
var allowedMapperImports = map[string]bool{
	modulePath + "/internal/domain": true,
	modulePath + "/internal/dto":    true,
}

// TestMapperDependencies keeps internal/mapper a pure conversion layer: it
// may depend only on domain, dto and the standard library (never on use
// cases, repositories, platform adapters or HTTP handlers), and it declares
// no structs at all, named or anonymous, including in its tests.
func TestMapperDependencies(t *testing.T) {
	dir := filepath.Join(moduleRoot(t), filepath.FromSlash(mapperPackage))
	entries, err := os.ReadDir(dir)
	if err != nil {
		t.Fatalf("read %s: %v", mapperPackage, err)
	}

	fset := token.NewFileSet()
	sources := 0
	for _, e := range entries {
		if e.IsDir() || filepath.Ext(e.Name()) != ".go" {
			continue
		}
		isTest := strings.HasSuffix(e.Name(), "_test.go")
		if !isTest {
			sources++
		}
		name := mapperPackage + "/" + e.Name()
		src, err := os.ReadFile(filepath.Join(dir, e.Name()))
		if err != nil {
			t.Fatalf("read %s: %v", name, err)
		}

		file, err := parser.ParseFile(fset, name, src, parser.ImportsOnly)
		if err != nil {
			t.Fatalf("parse %s: %v", name, err)
		}
		for _, imp := range file.Imports {
			p, err := strconv.Unquote(imp.Path.Value)
			if err != nil {
				t.Fatalf("%s: bad import %s", name, imp.Path.Value)
			}
			if !mapperImportAllowed(p, isTest) {
				t.Errorf("%s imports %q; %s may import only internal/domain, internal/dto and the standard library",
					fset.Position(imp.Pos()), p, mapperPackage)
			}
		}

		structs, _, err := parseFile(fset, name, src)
		if err != nil {
			t.Fatalf("parse %s: %v", name, err)
		}
		for _, s := range structs {
			t.Errorf("%s: struct declared in %s; response shapes belong in internal/dto", s.pos, mapperPackage)
		}
	}
	if sources == 0 {
		t.Fatalf("no source files in %s; is the module root correct?", mapperPackage)
	}
}

func mapperImportAllowed(importPath string, isTest bool) bool {
	if allowedMapperImports[importPath] {
		return true
	}
	if isTest && importPath == modulePath+"/"+mapperPackage {
		return true
	}
	// Standard library paths have no dot in their first element.
	first, _, _ := strings.Cut(importPath, "/")
	return !strings.Contains(first, ".") && first != modulePath
}

func TestMapperImportAllowed(t *testing.T) {
	allowed := []string{"time", "encoding/json", "gostock/internal/domain", "gostock/internal/dto"}
	for _, p := range allowed {
		if !mapperImportAllowed(p, false) {
			t.Errorf("mapperImportAllowed(%q) = false, want true", p)
		}
	}
	denied := []string{
		"gostock/internal/usecase",
		"gostock/internal/handle/http",
		"gostock/internal/repository",
		"gostock/internal/repository/postgres",
		"gostock/internal/platform/jwt",
		"gostock/internal/mapper",
		"golang.org/x/crypto/bcrypt",
		"github.com/jackc/pgx/v5/stdlib",
	}
	for _, p := range denied {
		if mapperImportAllowed(p, false) {
			t.Errorf("mapperImportAllowed(%q) = true, want false", p)
		}
	}
	if !mapperImportAllowed("gostock/internal/mapper", true) {
		t.Error("external mapper tests must be allowed to import the mapper")
	}
}
