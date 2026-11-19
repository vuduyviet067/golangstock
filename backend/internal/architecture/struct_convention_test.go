// Package architecture holds tests that enforce project-wide architecture
// conventions. It contains no production code.
package architecture

import (
	"errors"
	"fmt"
	"go/ast"
	"go/parser"
	"go/token"
	"io/fs"
	"os"
	"path"
	"path/filepath"
	"sort"
	"strconv"
	"strings"
	"testing"
)

const modulePath = "gostock"

const (
	domainPackage = "internal/domain"
	dtoPackage    = "internal/dto"
)

var dataStructExceptions = map[string]string{
	"internal/platform/jwt.customClaims": "JWT wire-format claims private to the JWT adapter; it embeds " +
		"jwt.RegisteredClaims, so moving it to internal/dto would make dto depend on the JWT library",
}

// structDecl is a struct type found in Go source.
type structDecl struct {
	name                string // type name; empty for an anonymous struct type
	pos                 token.Position
	hasFields           bool
	allFieldsUnexported bool
}

// packageDecls collects the declarations of one package directory.
type packageDecls struct {
	structs         []structDecl
	methodReceivers map[string]bool // base type names that have methods
}

func TestStructConvention(t *testing.T) {
	root := moduleRoot(t)

	pkgs, err := loadPackages(os.DirFS(root))
	if err != nil {
		t.Fatalf("scan module %s: %v", root, err)
	}
	// Guard against a silent pass when scanning the wrong directory.
	for _, dir := range []string{domainPackage, dtoPackage} {
		if pkg := pkgs[dir]; pkg == nil || len(pkg.structs) == 0 {
			t.Fatalf("scan of %s found no structs in %s; is the module root correct?", root, dir)
		}
	}

	violations := checkConvention(pkgs, dataStructExceptions)
	for _, v := range violations {
		t.Error(v)
	}
	if len(violations) > 0 {
		t.Log("Move data structs to internal/domain (business entities) or internal/dto (everything else). " +
			"Keep behavior structs in the package that owns their behavior. " +
			"Add a dataStructExceptions entry only for a data struct that must stay private to its package.")
	}
}

func checkConvention(pkgs map[string]*packageDecls, exceptions map[string]string) []string {
	var violations []string
	existing := make(map[string]bool)
	needed := make(map[string]bool)

	for dir, pkg := range pkgs {
		inDataPackage := dir == domainPackage || dir == dtoPackage
		for _, s := range pkg.structs {
			if !s.hasFields {
				continue
			}
			if s.name == "" {
				if !inDataPackage {
					violations = append(violations, fmt.Sprintf(
						"%s: anonymous data struct outside %s and %s; declare a named struct in %s",
						s.pos, domainPackage, dtoPackage, dtoPackage))
				}
				continue
			}

			key := dir + "." + s.name
			existing[key] = true
			isBehavior := pkg.methodReceivers[s.name] && s.allFieldsUnexported

			switch {
			case isBehavior && dir == dtoPackage:
				violations = append(violations, fmt.Sprintf(
					"%s: behavior struct %s is declared in %s; keep it in the package that owns its behavior",
					s.pos, key, dtoPackage))
			case isBehavior, inDataPackage:
				// Allowed where it is.
			case exceptions[key] != "":
				needed[key] = true
			default:
				violations = append(violations, fmt.Sprintf(
					"%s: data struct %s is declared outside %s and %s",
					s.pos, key, domainPackage, dtoPackage))
			}
		}
	}

	for key := range exceptions {
		switch {
		case !existing[key]:
			violations = append(violations, fmt.Sprintf(
				"dataStructExceptions entry %q matches no struct declaration; remove it", key))
		case !needed[key]:
			violations = append(violations, fmt.Sprintf(
				"dataStructExceptions entry %q is not needed: the struct is a behavior struct or is declared in %s or %s; remove it",
				key, domainPackage, dtoPackage))
		}
	}

	sort.Strings(violations)
	return violations
}

func loadPackages(fsys fs.FS) (map[string]*packageDecls, error) {
	fset := token.NewFileSet()
	pkgs := make(map[string]*packageDecls)

	err := fs.WalkDir(fsys, ".", func(p string, d fs.DirEntry, err error) error {
		if err != nil {
			return err
		}
		if d.IsDir() {
			if p == "." {
				return nil
			}
			name := d.Name()
			if strings.HasPrefix(name, ".") || strings.HasPrefix(name, "_") ||
				name == "testdata" || name == "vendor" || name == "node_modules" {
				return fs.SkipDir
			}
			if _, err := fs.Stat(fsys, path.Join(p, "go.mod")); err == nil {
				return fs.SkipDir // nested module
			}
			return nil
		}
		if path.Ext(p) != ".go" || strings.HasSuffix(p, "_test.go") {
			return nil
		}

		src, err := fs.ReadFile(fsys, p)
		if err != nil {
			return err
		}
		structs, receivers, err := parseFile(fset, p, src)
		if err != nil {
			return err
		}

		dir := path.Dir(p)
		pkg := pkgs[dir]
		if pkg == nil {
			pkg = &packageDecls{methodReceivers: make(map[string]bool)}
			pkgs[dir] = pkg
		}
		pkg.structs = append(pkg.structs, structs...)
		for _, r := range receivers {
			pkg.methodReceivers[r] = true
		}
		return nil
	})
	return pkgs, err
}

func parseFile(fset *token.FileSet, filename string, src []byte) ([]structDecl, []string, error) {
	file, err := parser.ParseFile(fset, filename, src, parser.SkipObjectResolution)
	if err != nil {
		return nil, nil, err
	}

	named := make(map[*ast.StructType]string)
	var receivers []string
	ast.Inspect(file, func(n ast.Node) bool {
		switch n := n.(type) {
		case *ast.TypeSpec:
			if st, ok := ast.Unparen(n.Type).(*ast.StructType); ok {
				named[st] = n.Name.Name
			}
		case *ast.FuncDecl:
			if n.Recv != nil && len(n.Recv.List) > 0 {
				if name := typeName(n.Recv.List[0].Type); name != "" {
					receivers = append(receivers, name)
				}
			}
		}
		return true
	})

	var structs []structDecl
	ast.Inspect(file, func(n ast.Node) bool {
		if st, ok := n.(*ast.StructType); ok {
			structs = append(structs, structDecl{
				name:                named[st],
				pos:                 fset.Position(st.Pos()),
				hasFields:           len(st.Fields.List) > 0,
				allFieldsUnexported: allFieldsUnexported(st),
			})
		}
		return true
	})
	return structs, receivers, nil
}

func allFieldsUnexported(st *ast.StructType) bool {
	for _, field := range st.Fields.List {
		if len(field.Names) == 0 {
			if ast.IsExported(typeName(field.Type)) {
				return false
			}
			continue
		}
		for _, name := range field.Names {
			if name.IsExported() {
				return false
			}
		}
	}
	return true
}

func typeName(expr ast.Expr) string {
	for {
		switch e := expr.(type) {
		case *ast.Ident:
			return e.Name
		case *ast.SelectorExpr:
			return e.Sel.Name
		case *ast.StarExpr:
			expr = e.X
		case *ast.ParenExpr:
			expr = e.X
		case *ast.IndexExpr:
			expr = e.X
		case *ast.IndexListExpr:
			expr = e.X
		default:
			return ""
		}
	}
}

func moduleRoot(t *testing.T) string {
	t.Helper()

	start, err := os.Getwd()
	if err != nil {
		t.Fatalf("get working directory: %v", err)
	}
	for dir := start; ; {
		gomod := filepath.Join(dir, "go.mod")
		data, err := os.ReadFile(gomod)
		switch {
		case err == nil:
			if got := modulePathOf(data); got != modulePath {
				t.Fatalf("%s declares module %q, want %q", gomod, got, modulePath)
			}
			return dir
		case !errors.Is(err, fs.ErrNotExist):
			t.Fatalf("read %s: %v", gomod, err)
		}

		parent := filepath.Dir(dir)
		if parent == dir {
			t.Fatalf("no go.mod found at or above %s", start)
		}
		dir = parent
	}
}

// modulePathOf returns the module path declared in go.mod content, or "".
func modulePathOf(gomod []byte) string {
	for _, line := range strings.Split(string(gomod), "\n") {
		line = strings.TrimSpace(line)
		rest, ok := strings.CutPrefix(line, "module")
		if !ok || rest == "" || (rest[0] != ' ' && rest[0] != '\t') {
			continue
		}
		rest = strings.TrimSpace(rest)
		if i := strings.Index(rest, "//"); i >= 0 {
			rest = strings.TrimSpace(rest[:i])
		}
		if unquoted, err := strconv.Unquote(rest); err == nil {
			return unquoted
		}
		return rest
	}
	return ""
}
