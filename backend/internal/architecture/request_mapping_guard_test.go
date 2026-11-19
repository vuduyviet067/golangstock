package architecture

import (
	"fmt"
	"go/ast"
	"go/token"
	"io/fs"
	"path"
	"sort"
	"strings"
	"testing"
)

// Request-side layer guards (Step 5.5). They protect the request flow
//
//	HTTP Request DTO → Handler adapter → Usecase Input DTO → Usecase → Mapper → Domain
//
// with two rules: the HTTP layer never constructs a domain Product (it adapts
// the request to an Input DTO and the use case maps that with internal/mapper),
// and public use case methods never take an HTTP request DTO. Like the guards
// in layer_guard_test.go, every checker works on an fs.FS, runs on the real
// module and on in-memory fixtures (request_mapping_guard_cases_test.go), and
// looks at production files only.

// handlerUnbuildableDomainTypes names the domain types (declared in
// internal/domain) that the HTTP layer must not construct. It is deliberately
// not "every domain type": AuthMiddleware keeps a domain.User principal.
// Entries that no longer match a domain type are reported.
var handlerUnbuildableDomainTypes = map[string]string{
	"Product": "request data reaches a domain product only through dto.CreateProductInput and " +
		"dto.UpdateProductInput, which the use case maps with mapper.ProductFromCreateInput / ProductFromUpdateInput",
}

// findRef returns the name of the first type in the type expression expr that
// comes from the package imported under names (or dot-imported) and for which
// match is true, or that is a package-local type listed in local; "" if none.
// It is refersTo (layer_guard_test.go) reporting which type it found.
func findRef(expr ast.Node, names map[string]bool, dot bool, match func(typeName string) bool, local map[string]bool) string {
	found := ""
	ast.Inspect(expr, func(n ast.Node) bool {
		if found != "" {
			return false
		}
		switch n := n.(type) {
		case *ast.SelectorExpr:
			if id, ok := n.X.(*ast.Ident); ok && names[id.Name] && match(n.Sel.Name) {
				found = n.Sel.Name
			}
			return false // do not visit n.Sel as a bare identifier
		case *ast.Ident:
			if (dot && match(n.Name)) || local[n.Name] {
				found = n.Name
			}
		}
		return true
	})
	return found
}

// localTypesReferring returns the names of the non-struct types declared in
// files (one package) whose definition mentions, directly or through another
// such local type, a type of importPath for which match is true: e.g.
// "type Credentials = dto.LoginRequest" or "type product = domain.Product".
// Struct definitions are left out, as in checkUsecaseResults.
func localTypesReferring(files []goFile, importPath string, match func(string) bool) map[string]bool {
	type def struct {
		file *ast.File
		expr ast.Expr
	}
	defs := map[string]def{}
	for _, f := range files {
		for _, decl := range f.file.Decls {
			if gd, ok := decl.(*ast.GenDecl); ok && gd.Tok == token.TYPE {
				for _, spec := range gd.Specs {
					ts := spec.(*ast.TypeSpec)
					if _, isStruct := ts.Type.(*ast.StructType); !isStruct {
						defs[ts.Name.Name] = def{file: f.file, expr: ts.Type}
					}
				}
			}
		}
	}
	tainted := map[string]bool{}
	for changed := true; changed; {
		changed = false
		for name, d := range defs {
			if tainted[name] {
				continue
			}
			names, dot := importNames(d.file, importPath)
			if findRef(d.expr, names, dot, match, tainted) != "" {
				tainted[name] = true
				changed = true
			}
		}
	}
	return tainted
}

// byPackageDir groups parsed files by directory, i.e. by package.
func byPackageDir(files []goFile) map[string][]goFile {
	pkgs := map[string][]goFile{}
	for _, f := range files {
		dir := path.Dir(f.path)
		pkgs[dir] = append(pkgs[dir], f)
	}
	return pkgs
}

// declaredTypeNames returns the names of the types declared in files.
func declaredTypeNames(files []goFile) map[string]bool {
	names := map[string]bool{}
	for _, f := range files {
		for _, decl := range f.file.Decls {
			if gd, ok := decl.(*ast.GenDecl); ok && gd.Tok == token.TYPE {
				for _, spec := range gd.Specs {
					names[spec.(*ast.TypeSpec).Name.Name] = true
				}
			}
		}
	}
	return names
}

// ---------------------------------------------------------------------------
// Guard 1: the HTTP layer does not construct a domain Product.
// ---------------------------------------------------------------------------

// checkHandlerDomainConstruction reports, in the production files of the
// handler package and its subpackages (the response helper), every composite
// literal (value, pointer, slice or map literal), new(), conversion or variable
// declaration of a guarded domain type, found by import path whatever the
// import name, or of a package-local type defined from one. Other domain types
// (domain.User), and guarded types used only as parameters or values received
// from elsewhere, are not reported. Entries of guarded that no domain type
// declares are reported as stale.
func checkHandlerDomainConstruction(fsys fs.FS, guarded map[string]string) ([]string, error) {
	fset := token.NewFileSet()
	domainFiles, err := parseGoFiles(fsys, fset, domainPackage, false)
	if err != nil {
		return nil, err
	}
	declared := declaredTypeNames(domainFiles)

	var violations []string
	for name := range guarded {
		if !declared[name] {
			violations = append(violations, fmt.Sprintf(
				"handlerUnbuildableDomainTypes entry %q matches no type declared in %s; remove it", name, domainPackage))
		}
	}

	files, err := parseGoFiles(fsys, fset, handlerPackage, true)
	if err != nil {
		return nil, err
	}
	isGuarded := func(name string) bool { return guarded[name] != "" }
	for _, pkg := range byPackageDir(files) {
		local := localTypesReferring(pkg, domainImport, isGuarded)
		for _, f := range pkg {
			names, dot := importNames(f.file, domainImport)
			if len(names) == 0 && !dot && len(local) == 0 {
				continue
			}
			ref := func(n ast.Expr) string {
				if n == nil {
					return ""
				}
				return findRef(n, names, dot, isGuarded, local)
			}
			report := func(pos token.Pos, what, typ string) {
				label := "domain." + typ
				if local[typ] {
					label = "local type " + typ + " (defined from a guarded domain type)"
				}
				violations = append(violations, fmt.Sprintf(
					"%s: handler %s %s; adapt the request to a use case Input DTO and let the use case map it (internal/mapper)",
					fset.Position(pos), what, label))
			}
			ast.Inspect(f.file, func(n ast.Node) bool {
				switch n := n.(type) {
				case *ast.CompositeLit:
					if t := ref(n.Type); t != "" {
						report(n.Pos(), "builds", t)
					}
				case *ast.CallExpr:
					if id, ok := n.Fun.(*ast.Ident); ok && id.Name == "new" && len(n.Args) == 1 {
						if t := ref(n.Args[0]); t != "" {
							report(n.Pos(), "allocates", t)
						}
					} else if isTypeExpr(n.Fun) {
						if t := ref(n.Fun); t != "" {
							report(n.Pos(), "converts to", t)
						}
					}
				case *ast.ValueSpec:
					if t := ref(n.Type); t != "" {
						report(n.Pos(), "declares a variable of", t)
					}
				}
				return true
			})
		}
	}
	sort.Strings(violations)
	return violations, nil
}

// ---------------------------------------------------------------------------
// Guard 2: public use case methods do not take HTTP request DTOs.
// ---------------------------------------------------------------------------

// requestDTOTypes derives the HTTP request DTO type names from the source with
// two independent discriminators:
//
//  1. the naming convention: dto types named *Request. The Step 5.5 audit of
//     internal/dto found it exact: the six *Request types are the six request
//     bodies the handlers decode, and no other dto type ends in Request;
//  2. use: dto types the HTTP layer decodes a request into (see
//     decodedDTOTypes), so a request body type that breaks the convention is
//     still caught.
//
// Neither depends on JSON tags (response DTOs have them too). Input DTOs, which
// handler adapters build field by field, and query DTOs built from the URL are
// in neither set.
func requestDTOTypes(fsys fs.FS) (map[string]bool, error) {
	fset := token.NewFileSet()
	dtoFiles, err := parseGoFiles(fsys, fset, dtoPackage, false)
	if err != nil {
		return nil, err
	}
	set := map[string]bool{}
	for name := range declaredTypeNames(dtoFiles) {
		if strings.HasSuffix(name, "Request") {
			set[name] = true
		}
	}
	decoded, err := decodedDTOTypes(fsys)
	if err != nil {
		return nil, err
	}
	for name := range decoded {
		set[name] = true
	}
	return set, nil
}

// decodedDTOTypes returns the dto types that production code of the handler
// package (and subpackages) decodes into: a call of a method or function
// named Decode or Unmarshal given v, &v, &dto.T{} or new(dto.T), where v is a
// variable, parameter or result of the same function declared with type
// dto.T or *dto.T (or initialized from dto.T{}, &dto.T{} or new(dto.T)).
// Variables are matched by name within one top-level function; shadowing is
// not tracked.
func decodedDTOTypes(fsys fs.FS) (map[string]bool, error) {
	fset := token.NewFileSet()
	dtoFiles, err := parseGoFiles(fsys, fset, dtoPackage, false)
	if err != nil {
		return nil, err
	}
	dtoDeclared := declaredTypeNames(dtoFiles)
	files, err := parseGoFiles(fsys, fset, handlerPackage, true)
	if err != nil {
		return nil, err
	}

	set := map[string]bool{}
	for _, f := range files {
		names, dot := importNames(f.file, dtoImport)
		if len(names) == 0 && !dot {
			continue
		}
		// dtoTypeOf returns T for the type expression dto.T or *dto.T.
		dtoTypeOf := func(e ast.Expr) string {
			for {
				switch x := e.(type) {
				case *ast.StarExpr:
					e = x.X
				case *ast.ParenExpr:
					e = x.X
				case *ast.SelectorExpr:
					if id, ok := x.X.(*ast.Ident); ok && names[id.Name] {
						return x.Sel.Name
					}
					return ""
				case *ast.Ident:
					if dot && dtoDeclared[x.Name] {
						return x.Name
					}
					return ""
				default:
					return ""
				}
			}
		}
		// valueType returns T for the value dto.T{}, &dto.T{} or new(dto.T).
		valueType := func(e ast.Expr) string {
			if u, ok := e.(*ast.UnaryExpr); ok && u.Op == token.AND {
				e = u.X
			}
			switch x := e.(type) {
			case *ast.CompositeLit:
				if x.Type != nil {
					return dtoTypeOf(x.Type)
				}
			case *ast.CallExpr:
				if id, ok := x.Fun.(*ast.Ident); ok && id.Name == "new" && len(x.Args) == 1 {
					return dtoTypeOf(x.Args[0])
				}
			}
			return ""
		}

		for _, decl := range f.file.Decls {
			fd, ok := decl.(*ast.FuncDecl)
			if !ok || fd.Body == nil {
				continue
			}
			vars := map[string]string{}
			addFields := func(fl *ast.FieldList) {
				if fl == nil {
					return
				}
				for _, field := range fl.List {
					if t := dtoTypeOf(field.Type); t != "" {
						for _, n := range field.Names {
							vars[n.Name] = t
						}
					}
				}
			}
			addFields(fd.Type.Params)
			addFields(fd.Type.Results)
			ast.Inspect(fd.Body, func(n ast.Node) bool {
				switch n := n.(type) {
				case *ast.FuncLit:
					addFields(n.Type.Params)
					addFields(n.Type.Results)
				case *ast.ValueSpec:
					if n.Type != nil {
						if t := dtoTypeOf(n.Type); t != "" {
							for _, id := range n.Names {
								vars[id.Name] = t
							}
						}
					}
					for i, v := range n.Values {
						if t := valueType(v); t != "" && i < len(n.Names) {
							vars[n.Names[i].Name] = t
						}
					}
				case *ast.AssignStmt:
					if n.Tok == token.DEFINE && len(n.Lhs) == len(n.Rhs) {
						for i, rhs := range n.Rhs {
							if id, ok := n.Lhs[i].(*ast.Ident); ok {
								if t := valueType(rhs); t != "" {
									vars[id.Name] = t
								}
							}
						}
					}
				}
				return true
			})
			ast.Inspect(fd.Body, func(n ast.Node) bool {
				call, ok := n.(*ast.CallExpr)
				if !ok {
					return true
				}
				var fn string
				switch fun := call.Fun.(type) {
				case *ast.SelectorExpr:
					fn = fun.Sel.Name
				case *ast.Ident:
					fn = fun.Name
				}
				if fn != "Decode" && fn != "Unmarshal" {
					return true
				}
				for _, arg := range call.Args {
					if t := valueType(arg); t != "" {
						set[t] = true
						continue
					}
					a := arg
					if u, ok := a.(*ast.UnaryExpr); ok && u.Op == token.AND {
						a = u.X
					}
					if id, ok := a.(*ast.Ident); ok && vars[id.Name] != "" {
						set[vars[id.Name]] = true
					}
				}
				return true
			})
		}
	}
	return set, nil
}

// checkUsecaseParams reports every exported use case method (of an exported
// receiver type) and exported package-level function, the same public scope
// as checkUsecaseResults, with a parameter whose type mentions a request DTO
// in any form: value, pointer, slice, array, map, channel, variadic, function
// type, generic type argument, or a package-local type defined from one.
// Results are not checked, and other dto types (Input, Query, Response) are
// allowed.
func checkUsecaseParams(fsys fs.FS, requestTypes map[string]bool) ([]string, error) {
	fset := token.NewFileSet()
	files, err := parseGoFiles(fsys, fset, usecasePackage, false)
	if err != nil {
		return nil, err
	}
	isRequest := func(name string) bool { return requestTypes[name] }
	local := localTypesReferring(files, dtoImport, isRequest)

	var violations []string
	for _, f := range files {
		names, dot := importNames(f.file, dtoImport)
		for _, decl := range f.file.Decls {
			fd, ok := decl.(*ast.FuncDecl)
			if !ok || !fd.Name.IsExported() || fd.Type.Params == nil {
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
			for _, field := range fd.Type.Params.List {
				t := findRef(field.Type, names, dot, isRequest, local)
				if t == "" {
					continue
				}
				label := "dto." + t
				if local[t] {
					label = "local type " + t + " (defined from a request DTO)"
				}
				violations = append(violations, fmt.Sprintf(
					"%s: %s takes %s, an HTTP request DTO; the handler adapts the request to a use case Input DTO (e.g. dto.CreateProductInput)",
					fset.Position(field.Pos()), key, label))
				break // one violation per method
			}
		}
	}
	sort.Strings(violations)
	return violations, nil
}

// ---------------------------------------------------------------------------
// Tests on the real module.
// ---------------------------------------------------------------------------

func TestHandlerDoesNotBuildDomainProduct(t *testing.T) {
	fsys := moduleFS(t)
	requireFiles(t, fsys, handlerPackage)
	requireFiles(t, fsys, domainPackage)
	v, err := checkHandlerDomainConstruction(fsys, handlerUnbuildableDomainTypes)
	reportViolations(t, v, err, "Build a dto.*Input in a handler adapter and pass it to the use case, which maps it with internal/mapper.")
}

func TestUsecaseDoesNotTakeRequestDTOs(t *testing.T) {
	fsys := moduleFS(t)
	requireFiles(t, fsys, dtoPackage)
	requireFiles(t, fsys, usecasePackage)

	types, err := requestDTOTypes(fsys)
	if err != nil {
		t.Fatal(err)
	}
	decoded, err := decodedDTOTypes(fsys)
	if err != nil {
		t.Fatal(err)
	}
	// Sanity checks, so a broken derivation cannot pass silently or start
	// banning input, query or response DTOs. Both discriminators must find
	// today's request bodies on their own.
	for _, want := range []string{"LoginRequest", "RegisterRequest", "CreateProductRequest",
		"UpdateProductRequest", "StockQuantityRequest", "UpdateUserRequest"} {
		if !types[want] {
			t.Errorf("request DTO set lacks %s: %v", want, sortedKeys(types))
		}
		if !decoded[want] {
			t.Errorf("decode-target derivation did not find %s: %v", want, sortedKeys(decoded))
		}
	}
	for _, notWant := range []string{"CreateProductInput", "UpdateProductInput", "UpdateUserInput",
		"StockMovementChartQuery", "LoginResult", "TokenClaims", "AppConfig", "HTTPError",
		"ProductResponse", "AuthUserResponse", "APIResponse", "MessageResponse"} {
		if types[notWant] {
			t.Errorf("request DTO set must not contain %s", notWant)
		}
	}
	t.Logf("request DTO types guarded in %s parameters: %v", usecasePackage, sortedKeys(types))

	v, err := checkUsecaseParams(fsys, types)
	reportViolations(t, v, err, "Decode the request in the handler, adapt it to a dto.*Input (no JSON tags) and pass that to the use case.")
}
