package architecture

import (
	"go/token"
	"slices"
	"strings"
	"testing"
	"testing/fstest"
)

const customClaimsKey = "internal/platform/jwt.customClaims"

func TestCheckConvention(t *testing.T) {
	tests := []struct {
		name       string
		files      map[string]string
		exceptions map[string]string
		// want holds one substring per expected violation, in sorted order.
		want []string
	}{
		// Valid layouts.
		{
			name: "data structs in domain and dto",
			files: map[string]string{
				"internal/domain/user.go": "package domain\ntype User struct{ ID int64 }",
				"internal/dto/auth.go":    "package dto\ntype LoginRequest struct{ Email string `json:\"email\"` }",
			},
		},
		{
			name: "domain entity with methods",
			files: map[string]string{
				"internal/domain/token.go": "package domain\ntype RefreshToken struct{ Revoked bool }\nfunc (t *RefreshToken) IsRevoked() bool { return t.Revoked }",
			},
		},
		{
			name: "behavior struct with methods in another file",
			files: map[string]string{
				"internal/usecase/product.go":         "package usecase\ntype Repo interface{}\ntype ProductUseCase struct{ repo Repo }",
				"internal/usecase/product_methods.go": "package usecase\nfunc (uc *ProductUseCase) Get() {}",
			},
		},
		{
			name: "value and generic receivers",
			files: map[string]string{
				"internal/platform/cache/cache.go": "package cache\n" +
					"type Cache[K comparable, V any] struct{ items map[K]V }\n" +
					"func (c *Cache[K, V]) Get(k K) V { return c.items[k] }\n" +
					"type clock struct{ offset int }\n" +
					"func (c clock) Now() int { return c.offset }",
			},
		},
		{
			name: "behavior struct embedding an unexported type",
			files: map[string]string{
				"internal/app/app.go": "package app\ntype base struct{ id int }\nfunc (b *base) ID() int { return b.id }\n" +
					"type App struct{ *base; addr string }\nfunc (a *App) Run() {}",
			},
		},
		{
			name: "encapsulated value object",
			files: map[string]string{
				"config/origin.go": "package config\ntype origin struct{ scheme, host string }\nfunc (o origin) String() string { return o.scheme + o.host }",
			},
		},
		{
			name: "structs without fields are ignored",
			files: map[string]string{
				"internal/handle/http/set.go": "package http\ntype marker struct{}\nvar ready = make(chan struct{})\nvar set = map[string]struct{}{\"a\": struct{}{}}",
			},
		},
		{
			name: "documented exception",
			files: map[string]string{
				"internal/platform/jwt/jwt.go": "package jwt\ntype customClaims struct{ Email string `json:\"email\"` }",
			},
			exceptions: map[string]string{customClaimsKey: "reason"},
		},
		{
			name: "test files are exempt",
			files: map[string]string{
				"internal/handle/http/fixture_test.go": "package http\ntype fixture struct{ Name string }\nfunc f() { var x struct{ A int }; _ = x }",
			},
		},
		{
			name: "skipped directories and nested modules",
			files: map[string]string{
				"testdata/sample.go":    "package sample\ntype Sample struct{ ID int }",
				".cache/sample.go":      "package sample\ntype Sample struct{ ID int }",
				"tools/go.mod":          "module tools\n",
				"tools/generate/gen.go": "package generate\ntype Options struct{ Out string }",
			},
		},

		// Violations.
		{
			name: "exported data struct outside domain and dto",
			files: map[string]string{
				"internal/handle/http/auth.go": "package http\ntype loginRequest struct{ Email string `json:\"email\"` }",
			},
			want: []string{"data struct internal/handle/http.loginRequest is declared outside"},
		},
		{
			name: "methods do not make a struct with exported fields a behavior struct",
			files: map[string]string{
				"internal/usecase/filter.go": "package usecase\ntype Filter struct{ Page int }\nfunc (f Filter) Offset() int { return f.Page }",
			},
			want: []string{"data struct internal/usecase.Filter is declared outside"},
		},
		{
			name: "unexported fields without methods",
			files: map[string]string{
				"internal/app/app.go": "package app\ntype startOptions struct{ ready chan struct{} }",
			},
			want: []string{"data struct internal/app.startOptions is declared outside"},
		},
		{
			name: "embedded exported field",
			files: map[string]string{
				"internal/app/app.go": "package app\nimport \"sync\"\ntype App struct{ sync.Mutex; addr string }\nfunc (a *App) Run() {}",
			},
			want: []string{"data struct internal/app.App is declared outside"},
		},
		{
			name: "anonymous struct in a function",
			files: map[string]string{
				"internal/handle/http/stock.go": "package http\nfunc f() { var req struct{ Quantity int }; _ = req }",
			},
			want: []string{"internal/handle/http/stock.go:2:20: anonymous data struct outside"},
		},
		{
			name: "anonymous struct nested in a behavior struct",
			files: map[string]string{
				"internal/app/app.go": "package app\ntype App struct{ opts struct{ Addr string } }\nfunc (a *App) Run() {}",
			},
			want: []string{"anonymous data struct outside"},
		},
		{
			name: "dto subpackage is not a data package",
			files: map[string]string{
				"internal/dto/handler/request.go": "package handler\ntype Request struct{ ID int }",
			},
			want: []string{"data struct internal/dto/handler.Request is declared outside"},
		},
		{
			name: "methods declared only in test files do not count",
			files: map[string]string{
				"internal/usecase/uc.go":      "package usecase\ntype UseCase struct{ repo int }",
				"internal/usecase/uc_test.go": "package usecase\nfunc (u *UseCase) helper() {}",
			},
			want: []string{"data struct internal/usecase.UseCase is declared outside"},
		},
		{
			name: "behavior struct in dto",
			files: map[string]string{
				"internal/dto/product.go": "package dto\ntype ProductUseCase struct{ repo int }\nfunc (uc *ProductUseCase) Get() {}",
			},
			want: []string{"behavior struct internal/dto.ProductUseCase is declared in internal/dto"},
		},
		{
			name: "exception that matches no struct",
			files: map[string]string{
				"internal/dto/auth.go": "package dto\ntype LoginRequest struct{ Email string }",
			},
			exceptions: map[string]string{customClaimsKey: "reason"},
			want:       []string{`entry "internal/platform/jwt.customClaims" matches no struct declaration`},
		},
		{
			name: "exception for a behavior struct is not needed",
			files: map[string]string{
				"internal/usecase/uc.go": "package usecase\ntype UseCase struct{ repo int }\nfunc (u *UseCase) Run() {}",
			},
			exceptions: map[string]string{"internal/usecase.UseCase": "reason"},
			want:       []string{`entry "internal/usecase.UseCase" is not needed`},
		},
		{
			name: "exception for a dto struct is not needed",
			files: map[string]string{
				"internal/dto/auth.go": "package dto\ntype LoginRequest struct{ Email string }",
			},
			exceptions: map[string]string{"internal/dto.LoginRequest": "reason"},
			want:       []string{`entry "internal/dto.LoginRequest" is not needed`},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			fsys := fstest.MapFS{}
			for name, src := range tt.files {
				fsys[name] = &fstest.MapFile{Data: []byte(src)}
			}
			pkgs, err := loadPackages(fsys)
			if err != nil {
				t.Fatalf("loadPackages: %v", err)
			}

			got := checkConvention(pkgs, tt.exceptions)
			if len(got) != len(tt.want) {
				t.Fatalf("got %d violations, want %d:\n%s", len(got), len(tt.want), strings.Join(got, "\n"))
			}
			for i, want := range tt.want {
				if !strings.Contains(got[i], want) {
					t.Errorf("violation %d = %q, want it to contain %q", i, got[i], want)
				}
			}
		})
	}
}

func TestParseFile(t *testing.T) {
	tests := []struct {
		name string
		src  string
		// structs lists each struct as "<name or _>:<fields|empty>:<unexported|exported>".
		structs   []string
		receivers []string
	}{
		{
			name:    "named struct",
			src:     "package p\ntype User struct{ ID int }",
			structs: []string{"User:fields:exported"},
		},
		{
			name:    "grouped and generic declarations",
			src:     "package p\ntype (\n\tA struct{ x int }\n\tB[T any] struct{ V T }\n)",
			structs: []string{"A:fields:unexported", "B:fields:exported"},
		},
		{
			name:    "named empty struct",
			src:     "package p\ntype marker struct{}",
			structs: []string{"marker:empty:unexported"},
		},
		{
			name:    "alias to a struct literal",
			src:     "package p\ntype A = struct{ X int }",
			structs: []string{"A:fields:exported"},
		},
		{
			name:    "function-local named struct",
			src:     "package p\nfunc f() { type row struct{ id int }; _ = row{} }",
			structs: []string{"row:fields:unexported"},
		},
		{
			name:    "anonymous struct variable",
			src:     "package p\nfunc f() { var req struct{ Quantity int }; _ = req }",
			structs: []string{"_:fields:exported"},
		},
		{
			name:    "anonymous struct nested in a named struct",
			src:     "package p\ntype A struct{ inner struct{ X int } }",
			structs: []string{"A:fields:unexported", "_:fields:exported"},
		},
		{
			name:    "embedded fields",
			src:     "package p\nimport \"sync\"\ntype A struct{ *base }\ntype B struct{ sync.Mutex }\ntype C struct{ Base[int] }",
			structs: []string{"A:fields:unexported", "B:fields:exported", "C:fields:exported"},
		},
		{
			name:    "empty struct idioms",
			src:     "package p\nvar ch chan struct{}\nvar set = map[string]struct{}{\"a\": struct{}{}}",
			structs: []string{"_:empty:unexported", "_:empty:unexported", "_:empty:unexported"},
		},
		{
			name: "non-struct types",
			src:  "package p\ntype I interface{ M() }\ntype F func()\ntype K string",
		},
		{
			name:      "method receivers",
			src:       "package p\nfunc (a A) M() {}\nfunc (b *B) M() {}\nfunc (c *C[K, V]) M() {}\nfunc (d D[T]) M() {}\nfunc F() {}",
			receivers: []string{"A", "B", "C", "D"},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			structs, receivers, err := parseFile(token.NewFileSet(), "src.go", []byte(tt.src))
			if err != nil {
				t.Fatalf("parseFile: %v", err)
			}

			var got []string
			for _, s := range structs {
				name, fields, visibility := s.name, "empty", "exported"
				if name == "" {
					name = "_"
				}
				if s.hasFields {
					fields = "fields"
				}
				if s.allFieldsUnexported {
					visibility = "unexported"
				}
				got = append(got, name+":"+fields+":"+visibility)
			}
			if !slices.Equal(got, tt.structs) {
				t.Errorf("structs = %q, want %q", got, tt.structs)
			}
			if !slices.Equal(receivers, tt.receivers) {
				t.Errorf("receivers = %q, want %q", receivers, tt.receivers)
			}
		})
	}
}

func TestModulePathOf(t *testing.T) {
	tests := []struct {
		name  string
		gomod string
		want  string
	}{
		{name: "plain", gomod: "module gostock\n\ngo 1.27.3\n", want: "gostock"},
		{name: "CRLF line endings", gomod: "module gostock\r\n\r\ngo 1.27.3\r\n", want: "gostock"},
		{name: "quoted", gomod: "module \"gostock\"\n", want: "gostock"},
		{name: "trailing comment", gomod: "module gostock // the API\n", want: "gostock"},
		{name: "after comments", gomod: "// header\nmodule example.com/app\n", want: "example.com/app"},
		{name: "similar directive is not module", gomod: "modulex gostock\n", want: ""},
		{name: "missing", gomod: "go 1.27.3\n", want: ""},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := modulePathOf([]byte(tt.gomod)); got != tt.want {
				t.Errorf("modulePathOf = %q, want %q", got, tt.want)
			}
		})
	}
}
