package architecture

import (
	"io/fs"
	"testing"
)

// Fixture tests for the response guards (XG-01a). Each case is a small
// in-memory module checked by the same functions as the real one; they show
// that every rule rejects its violations, in their different spellings, and
// accepts the allowed cases. No production file is changed.

func TestSharedPackageImportGuard(t *testing.T) {
	check := func(fsys fs.FS) ([]string, error) {
		return checkImports(fsys, sharedPackageTree, isInternalImport,
			"packages under pkg are shared code and must not depend on gostock/internal")
	}
	runGuardCases(t, []guardCase{
		// Allowed.
		{name: "standard library only", files: map[string]string{
			"pkg/response/r.go": "package response\nimport (\n\"encoding/json\"\n\"net/http\"\n)\nvar _ = json.Marshal\nvar _ http.Handler\n",
		}},
		{name: "external module and a non-internal module package", files: map[string]string{
			"pkg/x/x.go": "package x\nimport (\n\"github.com/google/uuid\"\n\"gostock/config\"\n)\nvar _ uuid.UUID\nvar _ config.X\n",
		}},
		{name: "test files are not production imports", files: map[string]string{
			"pkg/response/r_test.go": "package response\nimport \"gostock/internal/dto\"\n",
		}},
		{name: "internal packages may import pkg", files: map[string]string{
			"internal/handle/http/h.go": "package http\nimport \"gostock/pkg/response\"\n",
		}},

		// Rejected.
		{name: "pkg imports internal/dto", files: map[string]string{
			"pkg/response/r.go": "package response\nimport \"gostock/internal/dto\"\nvar _ dto.APIResponse\n",
		}, want: []string{`pkg/response/r.go:2:8: imports "gostock/internal/dto"; packages under pkg are shared code`}},
		{name: "aliased internal import", files: map[string]string{
			"pkg/response/r.go": "package response\nimport uc \"gostock/internal/usecase\"\nvar _ uc.X\n",
		}, want: []string{`imports "gostock/internal/usecase"`}},
		{name: "internal import inside a group", files: map[string]string{
			"pkg/response/r.go": "package response\nimport (\n\"net/http\"\n\"gostock/internal/domain\"\n)\nvar _ http.Handler\nvar _ domain.User\n",
		}, want: []string{`pkg/response/r.go:4:1: imports "gostock/internal/domain"`}},
		{name: "subpackage of pkg", files: map[string]string{
			"pkg/money/vnd/v.go": "package vnd\nimport \"gostock/internal/repository\"\n",
		}, want: []string{`pkg/money/vnd/v.go:2:8: imports "gostock/internal/repository"`}},
		{name: "blank import still counts", files: map[string]string{
			"pkg/response/r.go": "package response\nimport _ \"gostock/internal/platform/database\"\n",
		}, want: []string{`imports "gostock/internal/platform/database"`}},
	}, check)
}

func TestResponseHelperImporterGuard(t *testing.T) {
	check := func(fsys fs.FS) ([]string, error) {
		return checkResponseHelperImporters(fsys, responseHelperImporters)
	}
	const imp = "import \"gostock/pkg/response\"\n"
	runGuardCases(t, []guardCase{
		// Allowed.
		{name: "handler package", files: map[string]string{
			"internal/handle/http/h.go": "package http\n" + imp,
		}},
		{name: "subpackage of the handler package", files: map[string]string{
			"internal/handle/http/middleware/m.go": "package middleware\n" + imp,
		}},
		{name: "app package", files: map[string]string{
			"internal/app/router.go": "package app\n" + imp,
		}},
		{name: "the helper itself and its subpackages", files: map[string]string{
			"pkg/response/r.go":         "package response\nimport \"net/http\"\n",
			"pkg/response/codes/c.go":   "package codes\n" + imp,
			"pkg/response/r_helper.go":  "package response\n",
			"internal/usecase/u.go":     "package usecase\nimport \"gostock/internal/repository\"\n",
			"internal/usecase/u_doc.go": "package usecase\n",
		}},
		{name: "test files are not checked", files: map[string]string{
			"internal/usecase/u_test.go": "package usecase\n" + imp,
		}},
		{name: "hidden and testdata directories are skipped", files: map[string]string{
			".cache/x.go":                    "package x\n" + imp,
			"internal/usecase/testdata/x.go": "package x\n" + imp,
		}},

		// Rejected.
		{name: "use case imports the helper", files: map[string]string{
			"internal/usecase/u.go": "package usecase\n" + imp,
		}, want: []string{`internal/usecase/u.go:2:8: imports "gostock/pkg/response"; only internal/handle/http and internal/app write HTTP responses`}},
		{name: "aliased import in the mapper", files: map[string]string{
			"internal/mapper/m.go": "package mapper\nimport r \"gostock/pkg/response\"\nvar _ r.APIResponse\n",
		}, want: []string{`internal/mapper/m.go:2:8: imports "gostock/pkg/response"`}},
		{name: "dto imports the helper", files: map[string]string{
			"internal/dto/d.go": "package dto\nimport (\n\"time\"\n\"gostock/pkg/response\"\n)\nvar _ time.Time\n",
		}, want: []string{`internal/dto/d.go:4:1: imports "gostock/pkg/response"`}},
		{name: "main package", files: map[string]string{
			"main.go": "package main\n" + imp,
		}, want: []string{`main.go:2:8: imports "gostock/pkg/response"`}},
		{name: "a package whose path only starts like the handler package", files: map[string]string{
			"internal/handle/httpx/h.go": "package httpx\n" + imp,
		}, want: []string{`internal/handle/httpx/h.go:2:8`}},
		{name: "another shared package", files: map[string]string{
			"pkg/other/o.go": "package other\n" + imp,
		}, want: []string{`pkg/other/o.go:2:8`}},
		{name: "a subpackage of the helper", files: map[string]string{
			"internal/repository/r.go": "package repository\nimport \"gostock/pkg/response/codes\"\n",
		}, want: []string{`imports "gostock/pkg/response/codes"`}},
	}, check)
}

func TestHandlerBodyWriteGuard(t *testing.T) {
	handler := func(src string) map[string]string {
		return map[string]string{"internal/handle/http/h.go": "package http\n" + src}
	}
	const (
		jsonImp = "import \"encoding/json\"\n"
		httpImp = "import \"net/http\"\n"
	)
	runGuardCases(t, []guardCase{
		// Allowed.
		{name: "decoding a request", files: handler("import (\n\"encoding/json\"\n\"net/http\"\n)\n" +
			"func f(r *http.Request, v any) error { return json.NewDecoder(r.Body).Decode(v) }\n")},
		{name: "status and headers without a body", files: handler(httpImp +
			"func f(w http.ResponseWriter) { w.Header().Set(\"Allow\", \"GET\"); w.WriteHeader(http.StatusNoContent) }\n")},
		{name: "writing through the response helper", files: handler("import (\n\"net/http\"\n\"gostock/pkg/response\"\n)\n" +
			"func f(w http.ResponseWriter) { response.Error(w, http.StatusTeapot, \"\", \"x\") }\n")},
		{name: "Write on a value that is not a ResponseWriter", files: handler("import \"bytes\"\n" +
			"func f(w *bytes.Buffer) { w.Write(nil) }\n")},
		{name: "a closure parameter shadows the ResponseWriter", files: handler("import (\n\"bytes\"\n\"net/http\"\n)\n" +
			"func f(w http.ResponseWriter) { g := func(w *bytes.Buffer) { w.Write(nil) }; _ = g }\n")},
		{name: "test files are not checked", files: map[string]string{
			"internal/handle/http/h_test.go": "package http\nimport (\n\"encoding/json\"\n\"net/http\"\n)\n" +
				"func f(w http.ResponseWriter) { b, _ := json.Marshal(1); w.Write(b) }\n",
		}},
		{name: "packages outside the handler tree are not checked", files: map[string]string{
			"pkg/response/r.go": "package response\nimport (\n\"encoding/json\"\n\"net/http\"\n)\n" +
				"func f(w http.ResponseWriter) { b, _ := json.Marshal(1); w.Write(b) }\n",
		}},

		// Rejected: JSON encoding.
		{name: "json.Marshal", files: handler(jsonImp +
			"func f() { _, _ = json.Marshal(1) }\n"), want: []string{"h.go:3:19: uses json.Marshal; write the response with gostock/pkg/response"}},
		{name: "json.MarshalIndent", files: handler(jsonImp +
			"func f() { _, _ = json.MarshalIndent(1, \"\", \" \") }\n"), want: []string{"uses json.MarshalIndent"}},
		{name: "json.NewEncoder on the writer", files: handler("import (\n\"encoding/json\"\n\"net/http\"\n)\n" +
			"func f(w http.ResponseWriter) { _ = json.NewEncoder(w).Encode(1) }\n"), want: []string{"uses json.NewEncoder"}},
		{name: "aliased encoding/json", files: handler("import j \"encoding/json\"\n" +
			"func f() { _, _ = j.Marshal(1) }\n"), want: []string{"uses json.Marshal"}},
		{name: "dot-imported encoding/json", files: handler("import . \"encoding/json\"\n" +
			"func f() { _, _ = Marshal(1) }\n"), want: []string{"uses json.Marshal"}},
		{name: "json.Marshal taken as a value", files: handler(jsonImp +
			"var enc = json.Marshal\n"), want: []string{"h.go:3:11: uses json.Marshal"}},
		{name: "subpackage of the handler tree", files: map[string]string{
			"internal/handle/http/sub/s.go": "package sub\n" + jsonImp + "func f() { _, _ = json.Marshal(1) }\n",
		}, want: []string{"internal/handle/http/sub/s.go:3:19: uses json.Marshal"}},

		// Rejected: writes to the ResponseWriter.
		{name: "w.Write in a handler function", files: handler(httpImp +
			"func f(w http.ResponseWriter) { w.Write(nil) }\n"), want: []string{"h.go:3:33: writes to the http.ResponseWriter directly"}},
		{name: "w.Write in a HandlerFunc closure", files: handler(httpImp +
			"func f() http.Handler { return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) { _, _ = w.Write([]byte(\"x\")) }) }\n"),
			want: []string{"writes to the http.ResponseWriter directly"}},
		{name: "closure capturing the writer", files: handler(httpImp +
			"func f(w http.ResponseWriter) { func() { w.Write(nil) }() }\n"), want: []string{"writes to the http.ResponseWriter directly"}},
		{name: "local variable of type ResponseWriter", files: handler(httpImp +
			"func f(w http.ResponseWriter) { var out http.ResponseWriter = w; out.Write(nil) }\n"), want: []string{"writes to the http.ResponseWriter directly"}},
		{name: "method with a ResponseWriter parameter", files: handler(httpImp +
			"type h struct{}\nfunc (h) serve(rw http.ResponseWriter) { rw.Write(nil) }\n"), want: []string{"h.go:4:42: writes to the http.ResponseWriter directly"}},
		{name: "aliased net/http", files: handler("import nh \"net/http\"\n" +
			"func f(w nh.ResponseWriter) { w.Write(nil) }\n"), want: []string{"writes to the http.ResponseWriter directly"}},
		{name: "embedded ResponseWriter", files: handler(httpImp +
			"type rec struct{ http.ResponseWriter }\nfunc (r rec) f() { r.ResponseWriter.Write(nil) }\n"),
			want: []string{"writes to the embedded http.ResponseWriter directly"}},
		{name: "fmt.Fprintf to the writer", files: handler("import (\n\"fmt\"\n\"net/http\"\n)\n" +
			"func f(w http.ResponseWriter) { fmt.Fprintf(w, \"%d\", 1) }\n"), want: []string{"writes to the http.ResponseWriter with fmt.Fprintf"}},
		{name: "io.WriteString to the writer", files: handler("import (\n\"io\"\n\"net/http\"\n)\n" +
			"func f(w http.ResponseWriter) { io.WriteString(w, \"x\") }\n"), want: []string{"writes to the http.ResponseWriter with io.WriteString"}},
		{name: "marshal then write: two violations", files: handler("import (\n\"encoding/json\"\n\"net/http\"\n)\n" +
			"func f(w http.ResponseWriter) { b, _ := json.Marshal(1); w.Write(b) }\n"),
			want: []string{"uses json.Marshal", "writes to the http.ResponseWriter directly"}},
	}, checkHandlerBodyWrites)
}
