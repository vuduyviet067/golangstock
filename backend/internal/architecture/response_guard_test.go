package architecture

import (
	"fmt"
	"go/ast"
	"go/parser"
	"go/token"
	"io/fs"
	"path"
	"sort"
	"strconv"
	"strings"
	"testing"
)

// Response guards (XG-01a, TD-06). The standard response helper lives in the
// shared package pkg/response:
//
//  1. packages under pkg import nothing from gostock/internal;
//  2. only the HTTP layer (internal/handle/http and internal/app) imports
//     pkg/response;
//  3. the handler tree writes response bodies only through pkg/response: it
//     does not encode JSON itself or write to the http.ResponseWriter.
//
// Like the other guards, each checker works on an fs.FS, runs on the real
// module and on in-memory fixtures (response_guard_cases_test.go), and looks
// at production files only.

const (
	sharedPackageTree = "pkg"
	appPackage        = "internal/app"

	responseHelperImport = modulePath + "/" + responseHelperPackage
)

// responseHelperImporters are the package trees that may import pkg/response.
var responseHelperImporters = []string{handlerPackage, appPackage}

// ---------------------------------------------------------------------------
// Rule 2: only the HTTP layer imports pkg/response.
// ---------------------------------------------------------------------------

// inTree reports whether dir is root or a directory below it.
func inTree(dir, root string) bool {
	return dir == root || strings.HasPrefix(dir, root+"/")
}

// checkResponseHelperImporters reports every production file of the module,
// outside the allowed trees and outside pkg/response itself, that imports
// pkg/response or a package below it.
func checkResponseHelperImporters(fsys fs.FS, allowed []string) ([]string, error) {
	return checkRestrictedImport(fsys,
		func(ip string) bool { return inTree(ip, responseHelperImport) },
		append([]string{responseHelperPackage}, allowed...),
		"only "+strings.Join(allowed, " and ")+" write HTTP responses")
}

// checkRestrictedImport reports every production file of the module, outside
// the allowed directory trees, that imports a package for which restricted
// returns true. Hidden, underscore, testdata, vendor and node_modules
// directories are skipped, like the go tool does.
func checkRestrictedImport(fsys fs.FS, restricted func(importPath string) bool, allowed []string, rule string) ([]string, error) {
	fset := token.NewFileSet()
	var violations []string
	err := fs.WalkDir(fsys, ".", func(p string, d fs.DirEntry, err error) error {
		if err != nil {
			return err
		}
		if d.IsDir() {
			name := d.Name()
			if p != "." && (strings.HasPrefix(name, ".") || strings.HasPrefix(name, "_") ||
				name == "testdata" || name == "vendor" || name == "node_modules") {
				return fs.SkipDir
			}
			return nil
		}
		if path.Ext(p) != ".go" || strings.HasSuffix(p, "_test.go") {
			return nil
		}
		dir := path.Dir(p)
		for _, root := range allowed {
			if inTree(dir, root) {
				return nil
			}
		}
		src, err := fs.ReadFile(fsys, p)
		if err != nil {
			return err
		}
		file, err := parser.ParseFile(fset, p, src, parser.ImportsOnly)
		if err != nil {
			return err
		}
		for _, imp := range file.Imports {
			ip, err := strconv.Unquote(imp.Path.Value)
			if err != nil {
				return fmt.Errorf("%s: bad import %s", p, imp.Path.Value)
			}
			if restricted(ip) {
				violations = append(violations, fmt.Sprintf("%s: imports %q; %s",
					fset.Position(imp.Pos()), ip, rule))
			}
		}
		return nil
	})
	sort.Strings(violations)
	return violations, err
}

// ---------------------------------------------------------------------------
// Rule 3: the handler tree writes bodies only through pkg/response.
// ---------------------------------------------------------------------------

// bannedJSONFuncs are the encoding/json functions that produce a body.
// json.NewDecoder (reading a request) stays allowed.
var bannedJSONFuncs = map[string]bool{"NewEncoder": true, "Marshal": true, "MarshalIndent": true}

// writerFuncs are standard functions whose first argument is the destination
// writer; given an http.ResponseWriter they write a body like w.Write.
var writerFuncs = map[string]map[string]bool{
	"fmt": {"Fprint": true, "Fprintf": true, "Fprintln": true},
	"io":  {"WriteString": true, "Copy": true, "CopyN": true, "CopyBuffer": true},
}

// checkHandlerBodyWrites reports, in the production files of the handler tree,
// every use of json.NewEncoder, json.Marshal or json.MarshalIndent and every
// write of a body to an http.ResponseWriter: x.Write(...) or
// x.ResponseWriter.Write(...) on a variable declared as http.ResponseWriter,
// and fmt.Fprint*/io.WriteString/io.Copy* with such a variable as destination.
// w.WriteHeader (a status without a body) stays allowed.
func checkHandlerBodyWrites(fsys fs.FS) ([]string, error) {
	fset := token.NewFileSet()
	files, err := parseGoFiles(fsys, fset, handlerPackage, true)
	if err != nil {
		return nil, err
	}
	var violations []string
	report := func(pos token.Pos, what string) {
		violations = append(violations, fmt.Sprintf(
			"%s: %s; write the response with gostock/pkg/response", fset.Position(pos), what))
	}
	for _, f := range files {
		jsonNames, jsonDot := importNames(f.file, "encoding/json")
		httpNames, httpDot := importNames(f.file, "net/http")
		pkgNames := map[string]map[string]bool{}
		for pkg := range writerFuncs {
			names, _ := importNames(f.file, pkg)
			pkgNames[pkg] = names
		}

		isResponseWriterType := func(e ast.Expr) bool {
			switch t := e.(type) {
			case *ast.SelectorExpr:
				id, ok := t.X.(*ast.Ident)
				return ok && httpNames[id.Name] && t.Sel.Name == "ResponseWriter"
			case *ast.Ident:
				return httpDot && t.Name == "ResponseWriter"
			}
			return false
		}

		// JSON encoding, called or taken as a value.
		ast.Inspect(f.file, func(n ast.Node) bool {
			switch n := n.(type) {
			case *ast.SelectorExpr:
				if id, ok := n.X.(*ast.Ident); ok && jsonNames[id.Name] && bannedJSONFuncs[n.Sel.Name] {
					report(n.Pos(), "uses json."+n.Sel.Name)
				}
			case *ast.Ident:
				if jsonDot && bannedJSONFuncs[n.Name] {
					report(n.Pos(), "uses json."+n.Name)
				}
			}
			return true
		})

		// Writes to a ResponseWriter, with the writers in scope of each function.
		var walk func(n ast.Node, writers map[string]bool)
		withParams := func(ft *ast.FuncType, outer map[string]bool) map[string]bool {
			scope := map[string]bool{}
			for k := range outer {
				scope[k] = true
			}
			if ft.Params != nil {
				for _, field := range ft.Params.List {
					for _, name := range field.Names {
						scope[name.Name] = isResponseWriterType(field.Type)
					}
				}
			}
			return scope
		}
		isWriter := func(e ast.Expr, writers map[string]bool) bool {
			id, ok := e.(*ast.Ident)
			return ok && writers[id.Name]
		}
		walk = func(root ast.Node, writers map[string]bool) {
			ast.Inspect(root, func(n ast.Node) bool {
				switch n := n.(type) {
				case *ast.FuncLit:
					walk(n.Body, withParams(n.Type, writers))
					return false
				case *ast.ValueSpec:
					if n.Type != nil {
						for _, name := range n.Names {
							writers[name.Name] = isResponseWriterType(n.Type)
						}
					}
				case *ast.CallExpr:
					sel, ok := n.Fun.(*ast.SelectorExpr)
					if !ok {
						break
					}
					if sel.Sel.Name == "Write" {
						if isWriter(sel.X, writers) {
							report(n.Pos(), "writes to the http.ResponseWriter directly")
						} else if inner, ok := sel.X.(*ast.SelectorExpr); ok && inner.Sel.Name == "ResponseWriter" {
							report(n.Pos(), "writes to the embedded http.ResponseWriter directly")
						}
					}
					if pkg, ok := sel.X.(*ast.Ident); ok && len(n.Args) > 0 && isWriter(n.Args[0], writers) {
						for name, funcs := range writerFuncs {
							if pkgNames[name][pkg.Name] && funcs[sel.Sel.Name] {
								report(n.Pos(), "writes to the http.ResponseWriter with "+name+"."+sel.Sel.Name)
							}
						}
					}
				}
				return true
			})
		}
		for _, decl := range f.file.Decls {
			fd, ok := decl.(*ast.FuncDecl)
			if !ok || fd.Body == nil {
				continue
			}
			scope := withParams(fd.Type, nil)
			if fd.Recv != nil {
				for _, field := range fd.Recv.List {
					for _, name := range field.Names {
						scope[name.Name] = isResponseWriterType(field.Type)
					}
				}
			}
			walk(fd.Body, scope)
		}
	}
	sort.Strings(violations)
	return violations, nil
}

// ---------------------------------------------------------------------------
// Tests on the real module.
// ---------------------------------------------------------------------------

func TestSharedPackagesHaveNoInternalImports(t *testing.T) {
	fsys := moduleFS(t)
	requireFiles(t, fsys, responseHelperPackage)
	v, err := checkImports(fsys, sharedPackageTree, isInternalImport,
		"packages under pkg are shared code and must not depend on gostock/internal")
	reportViolations(t, v, err, "Keep pkg self-contained: pass what it needs as standard types, "+
		"or move the code under internal.")
}

func TestOnlyHTTPLayerImportsResponseHelper(t *testing.T) {
	fsys := moduleFS(t)
	requireFiles(t, fsys, responseHelperPackage)
	requireFiles(t, fsys, handlerPackage)
	requireFiles(t, fsys, appPackage)
	v, err := checkResponseHelperImporters(fsys, responseHelperImporters)
	reportViolations(t, v, err, "Return a DTO or an error to the handler and let it write the response.")
}

func TestHandlerWritesBodiesOnlyThroughResponseHelper(t *testing.T) {
	fsys := moduleFS(t)
	requireFiles(t, fsys, handlerPackage)
	v, err := checkHandlerBodyWrites(fsys)
	reportViolations(t, v, err, "Use response.Success/Created/Error/ValidationError/Paginated/NoContent or WriteError, "+
		"so every body has the standard envelope and error code.")
}
