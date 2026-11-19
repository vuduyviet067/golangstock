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
	"sort"
	"strconv"
	"strings"
	"testing"
)

// Layer guards added after the mapper refactor (Step 4.1). They protect the
// response flow
//
//	Repository → Usecase → Mapper → Response DTO → HTTP Handler → Response Helper
//
// and the independence of domain and dto. Every checker works on an fs.FS so
// that it runs on the real module (os.DirFS) and on small in-memory fixtures
// (layer_guard_cases_test.go) that prove each rule catches its violations.
// Only production files are checked; _test.go files are skipped.

const (
	handlePackageTree     = "internal/handle"
	handlerPackage        = "internal/handle/http"
	responseHelperPackage = "pkg/response"
	usecasePackage        = "internal/usecase"

	internalPrefix = modulePath + "/internal/"
	dtoImport      = modulePath + "/" + dtoPackage
	domainImport   = modulePath + "/" + domainPackage
	mapperImport   = modulePath + "/" + mapperPackage
)

// The response envelope types (APIResponse, PaginatedData, PaginationMeta) are
// declared in the response helper, pkg/response, not in internal/dto, so they
// are never part of the derived response DTO set and need no exception here.
// response_guard_test.go keeps the helper the only writer of response bodies.

// domainResultAllowlist names the public use case methods, as
// "ReceiverType.Method", that may return domain types because their result is
// not an HTTP response body.
var domainResultAllowlist = map[string]string{
	"AuthUseCase.Authenticate": "returns the authenticated principal that AuthMiddleware stores in the request context " +
		"for authorization (RequireRole, currentActorID, Me); it is never serialized as is",
	"AdminUseCase.CreateAdmin": "called only by the server-side admin command (internal/app, `gostock admin create`), which prints the email; no HTTP handler returns its result",
}

// goFile is one parsed production source file.
type goFile struct {
	path string
	file *ast.File
}

// parseGoFiles parses the non-test .go files of dir and, when recursive is
// true, of every directory below it. A missing dir yields no files.
func parseGoFiles(fsys fs.FS, fset *token.FileSet, dir string, recursive bool) ([]goFile, error) {
	if _, err := fs.Stat(fsys, dir); err != nil {
		if errors.Is(err, fs.ErrNotExist) {
			return nil, nil
		}
		return nil, err
	}
	var files []goFile
	err := fs.WalkDir(fsys, dir, func(p string, d fs.DirEntry, err error) error {
		if err != nil {
			return err
		}
		if d.IsDir() {
			if p != dir && !recursive {
				return fs.SkipDir
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
		f, err := parser.ParseFile(fset, p, src, parser.SkipObjectResolution)
		if err != nil {
			return err
		}
		files = append(files, goFile{path: p, file: f})
		return nil
	})
	return files, err
}

// importNames returns the names under which file refers to importPath ("" if
// it does not import it), and whether it dot-imports it.
func importNames(file *ast.File, importPath string) (names map[string]bool, dot bool) {
	names = map[string]bool{}
	for _, imp := range file.Imports {
		p, err := strconv.Unquote(imp.Path.Value)
		if err != nil || p != importPath {
			continue
		}
		switch {
		case imp.Name == nil:
			names[path.Base(p)] = true
		case imp.Name.Name == ".":
			dot = true
		case imp.Name.Name != "_":
			names[imp.Name.Name] = true
		}
	}
	return names, dot
}

// refersTo reports whether the type expression expr mentions a type from the
// package imported under names (or dot-imported), for which match returns
// true, or a package-local type listed in local.
func refersTo(expr ast.Node, names map[string]bool, dot bool, match func(typeName string) bool, local map[string]bool) bool {
	found := false
	ast.Inspect(expr, func(n ast.Node) bool {
		if found {
			return false
		}
		switch n := n.(type) {
		case *ast.SelectorExpr:
			if id, ok := n.X.(*ast.Ident); ok && names[id.Name] && match(n.Sel.Name) {
				found = true
			}
			return false // do not visit n.Sel as a bare identifier
		case *ast.Ident:
			if (dot && match(n.Name)) || local[n.Name] {
				found = true
			}
		}
		return true
	})
	return found
}

// ---------------------------------------------------------------------------
// Imports: domain and dto import no internal package; the handle tree does
// not import the mapper.
// ---------------------------------------------------------------------------

// checkImports reports every production import under root (recursively) for
// which forbidden returns true.
func checkImports(fsys fs.FS, root string, forbidden func(importPath string) bool, rule string) ([]string, error) {
	fset := token.NewFileSet()
	files, err := parseGoFiles(fsys, fset, root, true)
	if err != nil {
		return nil, err
	}
	var violations []string
	for _, f := range files {
		for _, imp := range f.file.Imports {
			p, err := strconv.Unquote(imp.Path.Value)
			if err != nil {
				return nil, fmt.Errorf("%s: bad import %s", f.path, imp.Path.Value)
			}
			if forbidden(p) {
				violations = append(violations, fmt.Sprintf("%s: imports %q; %s", fset.Position(imp.Pos()), p, rule))
			}
		}
	}
	sort.Strings(violations)
	return violations, nil
}

func isInternalImport(p string) bool { return strings.HasPrefix(p, internalPrefix) }

func isMapperImport(p string) bool {
	return p == mapperImport || strings.HasPrefix(p, mapperImport+"/")
}

// ---------------------------------------------------------------------------
// Handlers do not build response DTOs.
// ---------------------------------------------------------------------------

// responseDTOTypes derives the response DTO type names from the source: the
// dto types the mapper functions return, every dto type named *Response, and
// the dto struct types those contain (e.g. a chart point). Request and input
// DTOs are never in the set.
func responseDTOTypes(fsys fs.FS) (map[string]bool, error) {
	fset := token.NewFileSet()
	dtoFiles, err := parseGoFiles(fsys, fset, dtoPackage, false)
	if err != nil {
		return nil, err
	}
	mapperFiles, err := parseGoFiles(fsys, fset, mapperPackage, false)
	if err != nil {
		return nil, err
	}

	// dto struct name -> same-package type names used by its fields.
	fieldTypes := map[string][]string{}
	for _, f := range dtoFiles {
		for _, decl := range f.file.Decls {
			gd, ok := decl.(*ast.GenDecl)
			if !ok || gd.Tok != token.TYPE {
				continue
			}
			for _, spec := range gd.Specs {
				ts := spec.(*ast.TypeSpec)
				st, ok := ts.Type.(*ast.StructType)
				if !ok {
					continue
				}
				var used []string
				for _, field := range st.Fields.List {
					ast.Inspect(field.Type, func(n ast.Node) bool {
						if _, ok := n.(*ast.SelectorExpr); ok {
							return false // another package's type
						}
						if id, ok := n.(*ast.Ident); ok {
							used = append(used, id.Name)
						}
						return true
					})
				}
				fieldTypes[ts.Name.Name] = used
			}
		}
	}

	set := map[string]bool{}
	for name := range fieldTypes {
		if strings.HasSuffix(name, "Response") {
			set[name] = true
		}
	}
	for _, f := range mapperFiles {
		names, dot := importNames(f.file, dtoImport)
		for _, decl := range f.file.Decls {
			fd, ok := decl.(*ast.FuncDecl)
			if !ok || fd.Type.Results == nil {
				continue
			}
			ast.Inspect(fd.Type.Results, func(n ast.Node) bool {
				switch n := n.(type) {
				case *ast.SelectorExpr:
					if id, ok := n.X.(*ast.Ident); ok && names[id.Name] {
						set[n.Sel.Name] = true
					}
					return false
				case *ast.Ident:
					if dot && fieldTypes[n.Name] != nil {
						set[n.Name] = true
					}
				}
				return true
			})
		}
	}

	// Close over nested dto struct types.
	for changed := true; changed; {
		changed = false
		for name := range set {
			for _, used := range fieldTypes[name] {
				if _, isStruct := fieldTypes[used]; isStruct && !set[used] {
					set[used] = true
					changed = true
				}
			}
		}
	}
	return set, nil
}

// checkHandlerResponseConstruction reports, in the handler package (not the
// response helper), every composite literal, new(), conversion or variable
// declaration of a response DTO type.
func checkHandlerResponseConstruction(fsys fs.FS, responseTypes map[string]bool) ([]string, error) {
	fset := token.NewFileSet()
	files, err := parseGoFiles(fsys, fset, handlerPackage, false)
	if err != nil {
		return nil, err
	}
	isResponse := func(name string) bool { return responseTypes[name] }

	var violations []string
	report := func(pos token.Pos, what string) {
		violations = append(violations, fmt.Sprintf("%s: handler %s a response DTO; the use case must return it from the mapper",
			fset.Position(pos), what))
	}
	for _, f := range files {
		names, dot := importNames(f.file, dtoImport)
		if len(names) == 0 && !dot {
			continue
		}
		refers := func(n ast.Node) bool { return n != nil && refersTo(n, names, dot, isResponse, nil) }
		ast.Inspect(f.file, func(n ast.Node) bool {
			switch n := n.(type) {
			case *ast.CompositeLit:
				if refers(n.Type) {
					report(n.Pos(), "builds")
				}
			case *ast.CallExpr:
				if id, ok := n.Fun.(*ast.Ident); ok && id.Name == "new" && len(n.Args) == 1 && refers(n.Args[0]) {
					report(n.Pos(), "allocates")
				} else if refers(n.Fun) && isTypeExpr(n.Fun) {
					report(n.Pos(), "converts to")
				}
			case *ast.ValueSpec:
				if refers(n.Type) {
					report(n.Pos(), "declares a variable of")
				}
			}
			return true
		})
	}
	sort.Strings(violations)
	return violations, nil
}

// isTypeExpr reports whether a call's Fun is written as a type (a conversion)
// rather than a function value: a qualified or bare identifier, possibly
// parenthesized or a pointer/slice type.
func isTypeExpr(e ast.Expr) bool {
	switch e := e.(type) {
	case *ast.SelectorExpr, *ast.Ident, *ast.ArrayType, *ast.StarExpr, *ast.MapType:
		return true
	case *ast.ParenExpr:
		return isTypeExpr(e.X)
	}
	return false
}

// ---------------------------------------------------------------------------
// Public use case results contain no domain type.
// ---------------------------------------------------------------------------

// checkUsecaseResults reports every exported use case method (and exported
// package-level function) whose results mention a domain type, in any form:
// value, pointer, slice, array, map, channel, function type, generic type
// argument, or a package-local type defined from a domain type. Parameters
// are not checked. Methods listed in allowlist are accepted; stale entries are
// reported.
func checkUsecaseResults(fsys fs.FS, allowlist map[string]string) ([]string, error) {
	fset := token.NewFileSet()
	files, err := parseGoFiles(fsys, fset, usecasePackage, false)
	if err != nil {
		return nil, err
	}

	// Package-local type names whose definition refers to a domain type,
	// e.g. "type Principal = domain.User" or "type Users []domain.User".
	type localType struct {
		file *ast.File
		expr ast.Expr
	}
	defs := map[string]localType{}
	for _, f := range files {
		for _, decl := range f.file.Decls {
			if gd, ok := decl.(*ast.GenDecl); ok && gd.Tok == token.TYPE {
				for _, spec := range gd.Specs {
					ts := spec.(*ast.TypeSpec)
					defs[ts.Name.Name] = localType{file: f.file, expr: ts.Type}
				}
			}
		}
	}
	isDomain := func(string) bool { return true }
	tainted := map[string]bool{}
	for changed := true; changed; {
		changed = false
		for name, def := range defs {
			if tainted[name] {
				continue
			}
			// Behavior structs (use cases) hold repositories, not domain
			// values; only non-struct definitions can stand for a domain type.
			if _, isStruct := def.expr.(*ast.StructType); isStruct {
				continue
			}
			names, dot := importNames(def.file, domainImport)
			if refersTo(def.expr, names, dot, isDomain, tainted) {
				tainted[name] = true
				changed = true
			}
		}
	}

	declared := map[string]bool{}
	returnsDomain := map[string]bool{}
	var violations []string
	for _, f := range files {
		names, dot := importNames(f.file, domainImport)
		for _, decl := range f.file.Decls {
			fd, ok := decl.(*ast.FuncDecl)
			if !ok || !fd.Name.IsExported() {
				continue
			}
			key := fd.Name.Name
			if fd.Recv != nil && len(fd.Recv.List) > 0 {
				recv := typeName(fd.Recv.List[0].Type)
				if !ast.IsExported(recv) {
					continue
				}
				key = recv + "." + fd.Name.Name
			}
			declared[key] = true
			if fd.Type.Results == nil || !refersTo(fd.Type.Results, names, dot, isDomain, tainted) {
				continue
			}
			returnsDomain[key] = true
			if allowlist[key] == "" {
				violations = append(violations, fmt.Sprintf(
					"%s: %s returns a domain type; public use case results must be response DTOs built by the mapper",
					fset.Position(fd.Pos()), key))
			}
		}
	}
	for key := range allowlist {
		switch {
		case !declared[key]:
			violations = append(violations, fmt.Sprintf("domainResultAllowlist entry %q matches no exported use case method; remove it", key))
		case !returnsDomain[key]:
			violations = append(violations, fmt.Sprintf("domainResultAllowlist entry %q is not needed: it returns no domain type; remove it", key))
		}
	}
	sort.Strings(violations)
	return violations, nil
}

// ---------------------------------------------------------------------------
// Domain declarations: structs and vocabulary constants only.
// ---------------------------------------------------------------------------

// basicConstTypes are the predeclared types the domain may build on: the type
// a domain constant may be given, and the underlying type of a named domain
// type (type Money int64, type PieceStatus string).
var basicConstTypes = map[string]bool{
	"string": true, "bool": true, "rune": true, "byte": true,
	"int": true, "int8": true, "int16": true, "int32": true, "int64": true,
	"uint": true, "uint8": true, "uint16": true, "uint32": true, "uint64": true,
	"float32": true, "float64": true,
}

// checkDomainDeclarations allows, in the domain tree:
//   - imports;
//   - struct type declarations;
//   - named basic types: `type X T` where T is one of basicConstTypes and the
//     declaration is not an alias (type Money int64, type PieceStatus string);
//   - constants whose every name has its own literal value (a vocabulary
//     value such as InventoryStatusNormal = "normal"), untyped or typed with a
//     basic type or with a named basic type declared in the same package
//     (const PieceSold PieceStatus = "sold").
//
// It reports functions, methods (also on a named basic type), package
// variables, interfaces, every other type declaration (aliases; slices, maps,
// pointers, funcs, chans; types based on a named or another package's type),
// constants of any other type, and constants that are computed (expressions,
// references, iota or implicit repetition).
func checkDomainDeclarations(fsys fs.FS) ([]string, error) {
	fset := token.NewFileSet()
	files, err := parseGoFiles(fsys, fset, domainPackage, true)
	if err != nil {
		return nil, err
	}
	var violations []string
	report := func(pos token.Pos, format string, args ...any) {
		violations = append(violations, fmt.Sprintf("%s: %s; domain holds only structs, named basic types and vocabulary constants",
			fset.Position(pos), fmt.Sprintf(format, args...)))
	}

	// isNamedBasic reports whether ts is `type X T` with T a basic type.
	isNamedBasic := func(ts *ast.TypeSpec) bool {
		id, ok := ts.Type.(*ast.Ident)
		return ok && !ts.Assign.IsValid() && ts.TypeParams == nil && basicConstTypes[id.Name]
	}
	// First pass: the named basic types of each package (directory), so a
	// constant may use one declared in another file of the same package.
	namedBasic := map[string]map[string]bool{}
	for _, f := range files {
		dir := path.Dir(f.path)
		for _, decl := range f.file.Decls {
			if d, ok := decl.(*ast.GenDecl); ok && d.Tok == token.TYPE {
				for _, spec := range d.Specs {
					if ts := spec.(*ast.TypeSpec); isNamedBasic(ts) {
						if namedBasic[dir] == nil {
							namedBasic[dir] = map[string]bool{}
						}
						namedBasic[dir][ts.Name.Name] = true
					}
				}
			}
		}
	}

	for _, f := range files {
		dir := path.Dir(f.path)
		for _, decl := range f.file.Decls {
			switch d := decl.(type) {
			case *ast.FuncDecl:
				if d.Recv != nil {
					report(d.Pos(), "method %s.%s", typeName(d.Recv.List[0].Type), d.Name.Name)
				} else {
					report(d.Pos(), "function %s", d.Name.Name)
				}
			case *ast.GenDecl:
				switch d.Tok {
				case token.IMPORT:
				case token.VAR:
					for _, spec := range d.Specs {
						for _, name := range spec.(*ast.ValueSpec).Names {
							report(name.Pos(), "package variable %s", name.Name)
						}
					}
				case token.TYPE:
					for _, spec := range d.Specs {
						ts := spec.(*ast.TypeSpec)
						if isNamedBasic(ts) {
							continue // vocabulary type such as Money or PieceStatus
						}
						switch ts.Type.(type) {
						case *ast.StructType:
							if ts.Assign.IsValid() {
								report(ts.Pos(), "alias type %s", ts.Name.Name)
							}
						case *ast.InterfaceType:
							report(ts.Pos(), "interface %s", ts.Name.Name)
						default:
							report(ts.Pos(), "non-struct type %s", ts.Name.Name)
						}
					}
				case token.CONST:
					for _, spec := range d.Specs {
						vs := spec.(*ast.ValueSpec)
						if vs.Type != nil {
							// A basic type, or a named basic type of this package;
							// a selector (time.Duration) or any other type is not.
							if id, ok := vs.Type.(*ast.Ident); !ok || !(basicConstTypes[id.Name] || namedBasic[dir][id.Name]) {
								report(vs.Pos(), "constant %s has a non-basic type", vs.Names[0].Name)
								continue
							}
						}
						if len(vs.Values) != len(vs.Names) {
							report(vs.Pos(), "constant %s has no literal value of its own (implicit repetition or iota)", vs.Names[0].Name)
							continue
						}
						for i, v := range vs.Values {
							if !isLiteralConst(v) {
								report(v.Pos(), "constant %s is computed, not a literal", vs.Names[i].Name)
							}
						}
					}
				}
			}
		}
	}
	sort.Strings(violations)
	return violations, nil
}

// isLiteralConst reports whether e is a plain literal: a basic literal, a
// negated number, or true/false.
func isLiteralConst(e ast.Expr) bool {
	switch e := e.(type) {
	case *ast.BasicLit:
		return true
	case *ast.UnaryExpr:
		lit, ok := e.X.(*ast.BasicLit)
		return ok && e.Op == token.SUB && (lit.Kind == token.INT || lit.Kind == token.FLOAT)
	case *ast.Ident:
		return e.Name == "true" || e.Name == "false"
	case *ast.ParenExpr:
		return isLiteralConst(e.X)
	}
	return false
}

// ---------------------------------------------------------------------------
// Tests on the real module.
// ---------------------------------------------------------------------------

func moduleFS(t *testing.T) fs.FS {
	t.Helper()
	return os.DirFS(moduleRoot(t))
}

func reportViolations(t *testing.T, violations []string, err error, hint string) {
	t.Helper()
	if err != nil {
		t.Fatal(err)
	}
	for _, v := range violations {
		t.Error(v)
	}
	if len(violations) > 0 {
		t.Log(hint)
	}
}

// requireFiles guards against a silent pass when a directory is not found.
func requireFiles(t *testing.T, fsys fs.FS, dir string) {
	t.Helper()
	files, err := parseGoFiles(fsys, token.NewFileSet(), dir, false)
	if err != nil {
		t.Fatal(err)
	}
	if len(files) == 0 {
		t.Fatalf("no production Go files found in %s; is the module root correct?", dir)
	}
}

func TestDomainAndDTOHaveNoInternalImports(t *testing.T) {
	fsys := moduleFS(t)
	for _, dir := range []string{domainPackage, dtoPackage} {
		requireFiles(t, fsys, dir)
		v, err := checkImports(fsys, dir, isInternalImport, dir+" must not depend on other internal packages")
		reportViolations(t, v, err, "domain and dto are the innermost layers: move the dependency outward or the type inward.")
	}
}

func TestHandlerDoesNotImportMapper(t *testing.T) {
	fsys := moduleFS(t)
	requireFiles(t, fsys, handlerPackage)
	v, err := checkImports(fsys, handlePackageTree, isMapperImport, "only use cases call the mapper")
	reportViolations(t, v, err, "Return the response DTO from the use case instead of mapping in the HTTP layer.")
}

func TestHandlerDoesNotBuildResponseDTOs(t *testing.T) {
	fsys := moduleFS(t)
	requireFiles(t, fsys, handlerPackage)
	types, err := responseDTOTypes(fsys)
	if err != nil {
		t.Fatal(err)
	}
	// Sanity checks on the derived set, so a broken derivation cannot pass
	// silently or start banning request DTOs.
	for _, want := range []string{"ProductResponse", "UserResponse", "AuthUserResponse", "StockMovementResponse",
		"DashboardSummaryResponse", "InventoryStatusCount", "StockMovementChartResponse", "StockMovementChartPoint", "MessageResponse"} {
		if !types[want] {
			t.Errorf("response DTO set lacks %s: %v", want, sortedKeys(types))
		}
	}
	for _, notWant := range []string{"APIResponse", "PaginatedData", "CreateProductRequest", "UpdateUserRequest",
		"UpdateUserInput", "StockMovementChartQuery", "LoginRequest", "LoginResult"} {
		if types[notWant] {
			t.Errorf("response DTO set must not contain %s", notWant)
		}
	}
	t.Logf("response DTO types guarded in %s: %v", handlerPackage, sortedKeys(types))

	v, err := checkHandlerResponseConstruction(fsys, types)
	reportViolations(t, v, err, "Handlers pass the use case's DTO to the response helper; they never build one.")
}

func TestUsecasePublicResultsHaveNoDomainTypes(t *testing.T) {
	fsys := moduleFS(t)
	requireFiles(t, fsys, usecasePackage)
	v, err := checkUsecaseResults(fsys, domainResultAllowlist)
	reportViolations(t, v, err, "Map the domain value with internal/mapper and return the DTO. "+
		"Add a domainResultAllowlist entry only for a result that is not an HTTP response body.")
}

func TestDomainDeclarations(t *testing.T) {
	fsys := moduleFS(t)
	requireFiles(t, fsys, domainPackage)
	v, err := checkDomainDeclarations(fsys)
	reportViolations(t, v, err, "Keep behavior and calculations in use cases; domain declares entities and vocabulary constants.")
}

func sortedKeys(m map[string]bool) []string {
	keys := make([]string, 0, len(m))
	for k := range m {
		keys = append(keys, k)
	}
	sort.Strings(keys)
	return keys
}
