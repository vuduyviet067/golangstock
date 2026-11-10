package main

import (
	"go/ast"
	"go/parser"
	"go/token"
	"io/fs"
	"path/filepath"
	"slices"
	"sort"
	"strings"
	"testing"
)

// Struct declaration convention (CR-16)
//
// Production data structs must be declared in one of dataStructPackages:
// business entities and value objects in internal/domain, every other data
// structure (requests, responses, inputs, results, configuration) in
// internal/dto.
//
// A struct may stay in its own package only when it is listed in
// structExceptions. Valid reasons are:
//   - it carries behavior: it owns package-specific behavior, dependencies,
//     state, or lifecycle (handlers, use cases, repository implementations,
//     infrastructure services, the application), or
//   - it is an encapsulated implementation detail whose invariants depend on
//     staying unexported in its package.
//
// Structs declared in _test.go files are exempt.
//
// Adding an exception should be rare and reviewed: a new plain data struct
// belongs in domain or dto instead.

// dataStructPackages are the module-relative package directories where
// production data structs are declared. Only these exact packages qualify;
// subpackages such as internal/dto/handler do not.
var dataStructPackages = []string{
	"internal/domain",
	"internal/dto",
}

// structExceptions lists the production structs allowed outside
// dataStructPackages, keyed by "<module-relative package dir>.<TypeName>",
// with the reason for each. An entry whose struct no longer exists fails the
// test, so the list cannot go stale.
var structExceptions = map[string]string{
	// Behavior-carrying structs.
	"internal/app.App":                                    "application lifecycle: Run, Close",
	"internal/handle/http.AuthHandler":                    "HTTP handler",
	"internal/handle/http.ProductHandler":                 "HTTP handler",
	"internal/handle/http.UserHandler":                    "HTTP handler",
	"internal/handle/http.ReadinessHandler":               "HTTP handler",
	"internal/handle/http.CookieManager":                  "sets and clears auth cookies",
	"internal/platform/jwt.JWTService":                    "signs and validates access tokens",
	"internal/repository/postgres.ProductRepository":      "PostgreSQL repository implementation",
	"internal/repository/postgres.UserRepository":         "PostgreSQL repository implementation",
	"internal/repository/postgres.RefreshTokenRepository": "PostgreSQL repository implementation",
	"internal/usecase.AdminUseCase":                       "application use case",
	"internal/usecase.AuthUseCase":                        "application use case",
	"internal/usecase.ProductUseCase":                     "application use case",
	"internal/usecase.UserUseCase":                        "application use case",

	// Encapsulated implementation details.
	"config.origin":                      "validated CORS origin; unexported so only normalizeOrigin can construct it",
	"internal/platform/jwt.customClaims": "JWT wire claims private to the adapter; embeds jwt.RegisteredClaims",
}

// structDecl is a struct type found in Go source.
type structDecl struct {
	name string // type name; empty for an anonymous struct type
	pos  token.Position
}

func TestProductionStructsAreDeclaredInDomainOrDTO(t *testing.T) {
	fset := token.NewFileSet()
	found := make(map[string]bool)
	var violations []string

	err := filepath.WalkDir(".", func(path string, d fs.DirEntry, err error) error {
		if err != nil {
			return err
		}
		if d.IsDir() {
			name := d.Name()
			if path != "." && (strings.HasPrefix(name, ".") || strings.HasPrefix(name, "_") ||
				name == "testdata" || name == "vendor" || name == "node_modules") {
				return filepath.SkipDir
			}
			return nil
		}
		if !strings.HasSuffix(path, ".go") || strings.HasSuffix(path, "_test.go") {
			return nil
		}

		dir := filepath.ToSlash(filepath.Dir(path))
		if isDataStructPackage(dir) {
			return nil
		}

		decls, err := findStructDecls(fset, path, nil)
		if err != nil {
			return err
		}
		for _, decl := range decls {
			if decl.name == "" {
				violations = append(violations, decl.pos.String()+": anonymous struct type; declare a named struct in internal/domain or internal/dto")
				continue
			}
			key := dir + "." + decl.name
			if _, ok := structExceptions[key]; ok {
				found[key] = true
				continue
			}
			violations = append(violations, decl.pos.String()+": struct "+key+" is declared outside internal/domain and internal/dto")
		}
		return nil
	})
	if err != nil {
		t.Fatalf("scan module: %v", err)
	}

	sort.Strings(violations)
	for _, v := range violations {
		t.Error(v)
	}
	if len(violations) > 0 {
		t.Log("Move data structs to internal/domain (business entities) or internal/dto (everything else). " +
			"Add a structExceptions entry only for a behavior-carrying struct or an encapsulated implementation detail.")
	}

	var stale []string
	for key := range structExceptions {
		if !found[key] {
			stale = append(stale, key)
		}
	}
	sort.Strings(stale)
	for _, key := range stale {
		t.Errorf("structExceptions entry %q matches no struct declaration; remove it", key)
	}
}

func isDataStructPackage(dir string) bool {
	return slices.Contains(dataStructPackages, dir)
}

// findStructDecls parses one Go source file and returns every struct type it
// declares: named struct types (including function-local types and aliases)
// and anonymous struct types such as `var req struct{ ... }`. The empty
// struct{} carries no data and is ignored. src follows parser.ParseFile: nil
// reads filename from disk.
//
// It inspects syntax only, so a defined type whose underlying struct comes
// from another package (type T other.S) is not reported.
func findStructDecls(fset *token.FileSet, filename string, src any) ([]structDecl, error) {
	file, err := parser.ParseFile(fset, filename, src, parser.SkipObjectResolution)
	if err != nil {
		return nil, err
	}

	named := make(map[*ast.StructType]string)
	ast.Inspect(file, func(n ast.Node) bool {
		if spec, ok := n.(*ast.TypeSpec); ok {
			if st, ok := ast.Unparen(spec.Type).(*ast.StructType); ok {
				named[st] = spec.Name.Name
			}
		}
		return true
	})

	var decls []structDecl
	ast.Inspect(file, func(n ast.Node) bool {
		st, ok := n.(*ast.StructType)
		if !ok {
			return true
		}
		name, isNamed := named[st]
		if !isNamed && len(st.Fields.List) == 0 {
			return true
		}
		decls = append(decls, structDecl{name: name, pos: fset.Position(st.Pos())})
		return true
	})
	return decls, nil
}

func TestFindStructDecls(t *testing.T) {
	tests := []struct {
		name string
		src  string
		want []string // type names; "" for an anonymous struct
	}{
		{
			name: "named struct",
			src:  "package p\ntype User struct{ ID int }",
			want: []string{"User"},
		},
		{
			name: "grouped and generic declarations",
			src:  "package p\ntype (\n\tA struct{ X int }\n\tB[T any] struct{ V T }\n)",
			want: []string{"A", "B"},
		},
		{
			name: "named empty struct is still a declaration",
			src:  "package p\ntype marker struct{}",
			want: []string{"marker"},
		},
		{
			name: "alias to a struct literal",
			src:  "package p\ntype A = struct{ X int }",
			want: []string{"A"},
		},
		{
			name: "function-local named struct",
			src:  "package p\nfunc f() { type row struct{ ID int }; _ = row{} }",
			want: []string{"row"},
		},
		{
			name: "anonymous struct variable",
			src:  "package p\nfunc f() { var req struct{ Quantity int }; _ = req }",
			want: []string{""},
		},
		{
			name: "anonymous struct nested in a named struct",
			src:  "package p\ntype A struct{ Inner struct{ X int } }",
			want: []string{"A", ""},
		},
		{
			name: "empty struct idioms are ignored",
			src:  "package p\nvar ch chan struct{}\nvar set = map[string]struct{}{\"a\": struct{}{}}",
			want: nil,
		},
		{
			name: "non-struct types are ignored",
			src:  "package p\ntype I interface{ M() }\ntype F func()\ntype K string",
			want: nil,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			decls, err := findStructDecls(token.NewFileSet(), "src.go", tt.src)
			if err != nil {
				t.Fatalf("findStructDecls: %v", err)
			}
			var got []string
			for _, d := range decls {
				got = append(got, d.name)
			}
			if !slices.Equal(got, tt.want) {
				t.Errorf("got %q, want %q", got, tt.want)
			}
		})
	}
}
