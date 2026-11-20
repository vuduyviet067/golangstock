package architecture

import "testing"

// Fixture tests for the router guard (XG-01b): small in-memory modules
// checked by checkRouterImporters, showing that it rejects chi imports
// outside the HTTP layer, in their different spellings, and accepts the
// allowed ones. No production file is changed.

func TestRouterImporterGuard(t *testing.T) {
	const chi = "import \"github.com/go-chi/chi/v5\"\n"
	runGuardCases(t, []guardCase{
		// Allowed.
		{name: "app builds the router", files: map[string]string{
			"internal/app/router.go": "package app\n" + chi,
		}},
		{name: "handler reads path parameters", files: map[string]string{
			"internal/handle/http/routing.go": "package http\n" + chi,
		}},
		{name: "subpackage of the handler package with chi middleware", files: map[string]string{
			"internal/handle/http/mw/m.go": "package mw\nimport \"github.com/go-chi/chi/v5/middleware\"\n",
		}},
		{name: "other go-chi modules are not the router", files: map[string]string{
			"internal/usecase/u.go": "package usecase\nimport (\n\"github.com/go-chi/cors\"\n\"github.com/go-chi/chimiddleware\"\n)\n",
		}},
		{name: "test files are not checked", files: map[string]string{
			"internal/usecase/u_test.go": "package usecase\n" + chi,
		}},

		// Rejected.
		{name: "use case imports chi", files: map[string]string{
			"internal/usecase/u.go": "package usecase\n" + chi,
		}, want: []string{`internal/usecase/u.go:2:8: imports "github.com/go-chi/chi/v5"; only internal/handle/http and internal/app use the chi router`}},
		{name: "repository imports chi middleware", files: map[string]string{
			"internal/repository/postgres/r.go": "package postgres\nimport \"github.com/go-chi/chi/v5/middleware\"\n",
		}, want: []string{`imports "github.com/go-chi/chi/v5/middleware"`}},
		{name: "aliased import in the shared response package", files: map[string]string{
			"pkg/response/r.go": "package response\nimport router \"github.com/go-chi/chi/v5\"\nvar _ router.Router\n",
		}, want: []string{`pkg/response/r.go:2:8: imports "github.com/go-chi/chi/v5"`}},
		{name: "chi v1 module path", files: map[string]string{
			"internal/mapper/m.go": "package mapper\nimport \"github.com/go-chi/chi\"\n",
		}, want: []string{`imports "github.com/go-chi/chi"`}},
		{name: "chi inside an import group", files: map[string]string{
			"internal/dto/d.go": "package dto\nimport (\n\"time\"\n\"github.com/go-chi/chi/v5\"\n)\nvar _ time.Time\n",
		}, want: []string{`internal/dto/d.go:4:1: imports "github.com/go-chi/chi/v5"`}},
		{name: "main package", files: map[string]string{
			"main.go": "package main\n" + chi,
		}, want: []string{`main.go:2:8`}},
		{name: "a package whose path only starts like the handler package", files: map[string]string{
			"internal/handle/httpx/h.go": "package httpx\n" + chi,
		}, want: []string{`internal/handle/httpx/h.go:2:8`}},
	}, checkRouterImporters)
}
