package architecture

import (
	"io/fs"
	"strings"
	"testing"
	"testing/fstest"
)

// Fixture tests for the layer guards (Step 4.1). Each case is a small
// in-memory module checked by the same functions as the real one; together
// they show that every rule rejects its violations, in their different
// spellings, and accepts the allowed cases. No production file is changed.

// guardDTO and guardMapper are the dto and mapper of the handler fixtures.
// ProductView is a response DTO only because the mapper returns it; ChartPoint
// only because ChartResponse contains it.
const (
	guardDTO = "package dto\n" +
		"type ProductResponse struct{ ID int64 }\n" +
		"type ChartResponse struct{ Points []ChartPoint }\n" +
		"type ChartPoint struct{ Period string }\n" +
		"type ProductView struct{ ID int64 }\n" +
		"type CreateProductRequest struct{ SKU string }\n" +
		"type ChartQuery struct{ From string }\n"
	guardMapper = "package mapper\nimport \"gostock/internal/dto\"\n" +
		"func ToProductView() dto.ProductView { return dto.ProductView{} }\n"
	guardDomain = "package domain\ntype User struct{ ID int64 }\n"
)

type guardCase struct {
	name  string
	files map[string]string
	// want holds one substring per expected violation (any order).
	want []string
}

func runGuardCases(t *testing.T, cases []guardCase, check func(fsys fs.FS) ([]string, error)) {
	t.Helper()
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			fsys := fstest.MapFS{}
			for name, src := range tc.files {
				fsys[name] = &fstest.MapFile{Data: []byte(src)}
			}
			got, err := check(fsys)
			if err != nil {
				t.Fatalf("check: %v", err)
			}
			if len(got) != len(tc.want) {
				t.Fatalf("got %d violation(s), want %d:\n%s", len(got), len(tc.want), strings.Join(got, "\n"))
			}
			used := make([]bool, len(got))
			for _, w := range tc.want {
				found := false
				for i, v := range got {
					if !used[i] && strings.Contains(v, w) {
						used[i], found = true, true
						break
					}
				}
				if !found {
					t.Errorf("no violation contains %q in:\n%s", w, strings.Join(got, "\n"))
				}
			}
		})
	}
}

func withFiles(base map[string]string, extra map[string]string) map[string]string {
	files := map[string]string{}
	for k, v := range base {
		files[k] = v
	}
	for k, v := range extra {
		files[k] = v
	}
	return files
}

// handlerFiles returns a fixture module with the guard dto and mapper and one
// handler file with the given body (after the package clause).
func handlerFiles(body string) map[string]string {
	return withFiles(map[string]string{
		"internal/dto/dto.go":       guardDTO,
		"internal/mapper/mapper.go": guardMapper,
	}, map[string]string{"internal/handle/http/h.go": "package http\n" + body})
}

const dtoImportLine = "import \"gostock/internal/dto\"\n"

func TestHandlerImportGuard(t *testing.T) {
	check := func(fsys fs.FS) ([]string, error) {
		return checkImports(fsys, handlePackageTree, isMapperImport, "only use cases call the mapper")
	}
	runGuardCases(t, []guardCase{
		{name: "handler without mapper", files: map[string]string{
			"internal/handle/http/h.go": "package http\nimport (\n\"net/http\"\n\"gostock/internal/dto\"\n\"gostock/internal/usecase\"\n)\n",
		}},
		{name: "test files may import the mapper", files: map[string]string{
			"internal/handle/http/h_test.go": "package http\nimport \"gostock/internal/mapper\"\n",
		}},
		{name: "handler imports the mapper", files: map[string]string{
			"internal/handle/http/h.go": "package http\nimport \"gostock/internal/mapper\"\n",
		}, want: []string{`h.go:2:8: imports "gostock/internal/mapper"`}},
		{name: "handler imports the mapper under an alias", files: map[string]string{
			"internal/handle/http/h.go": "package http\nimport m \"gostock/internal/mapper\"\n",
		}, want: []string{`imports "gostock/internal/mapper"`}},
		{name: "response helper imports the mapper", files: map[string]string{
			"internal/handle/http/response/r.go": "package response\nimport \"gostock/internal/mapper\"\n",
		}, want: []string{`response/r.go:2:8: imports "gostock/internal/mapper"`}},
		{name: "a mapper subpackage is the mapper too", files: map[string]string{
			"internal/handle/http/h.go": "package http\nimport \"gostock/internal/mapper/auth\"\n",
		}, want: []string{`imports "gostock/internal/mapper/auth"`}},
	}, check)
}

func TestHandlerResponseConstructionGuard(t *testing.T) {
	check := func(fsys fs.FS) ([]string, error) {
		types, err := responseDTOTypes(fsys)
		if err != nil {
			return nil, err
		}
		return checkHandlerResponseConstruction(fsys, types)
	}
	runGuardCases(t, []guardCase{
		// Allowed.
		{name: "request and input DTOs", files: handlerFiles(dtoImportLine +
			"func f() { var req dto.CreateProductRequest; _ = req; _ = dto.ChartQuery{From: \"x\"} }\n")},
		{name: "passing a use case DTO through", files: handlerFiles(dtoImportLine +
			"func f(p dto.ProductResponse) any { return p }\n")},
		{name: "envelope types of pkg/response are not response DTOs", files: handlerFiles("import \"gostock/pkg/response\"\n" +
			"func f() { _ = response.APIResponse{Success: true}; _ = response.PaginatedData{} }\n")},
		{name: "response helper builds envelopes", files: withFiles(handlerFiles(""), map[string]string{
			"pkg/response/r.go": "package response\ntype APIResponse struct{ Success bool }\nfunc f() { _ = APIResponse{} }\n",
		})},
		{name: "handler tests may build DTOs", files: withFiles(handlerFiles(""), map[string]string{
			"internal/handle/http/h_test.go": "package http\n" + dtoImportLine + "func f() { _ = dto.ProductResponse{} }\n",
		})},

		// Rejected.
		{name: "composite literal", files: handlerFiles(dtoImportLine +
			"func f() { _ = dto.ProductResponse{ID: 1} }\n"), want: []string{"h.go:3:16: handler builds a response DTO"}},
		{name: "pointer literal", files: handlerFiles(dtoImportLine +
			"func f() { _ = &dto.ProductResponse{} }\n"), want: []string{"handler builds"}},
		{name: "slice literal with elided element types", files: handlerFiles(dtoImportLine +
			"func f() { _ = []dto.ProductResponse{{ID: 1}, {ID: 2}} }\n"), want: []string{"handler builds"}},
		{name: "map literal", files: handlerFiles(dtoImportLine +
			"func f() { _ = map[string]dto.ProductResponse{} }\n"), want: []string{"handler builds"}},
		{name: "nested response type", files: handlerFiles(dtoImportLine +
			"func f() { _ = dto.ChartPoint{Period: \"2026-01\"} }\n"), want: []string{"handler builds"}},
		{name: "type returned by the mapper", files: handlerFiles(dtoImportLine +
			"func f() { _ = dto.ProductView{} }\n"), want: []string{"handler builds"}},
		{name: "aliased dto import", files: handlerFiles("import d \"gostock/internal/dto\"\n" +
			"func f() { _ = d.ProductResponse{} }\n"), want: []string{"handler builds"}},
		{name: "dot-imported dto", files: handlerFiles("import . \"gostock/internal/dto\"\n" +
			"func f() { _ = ProductResponse{} }\n"), want: []string{"handler builds"}},
		{name: "new", files: handlerFiles(dtoImportLine +
			"func f() { _ = new(dto.ProductResponse) }\n"), want: []string{"handler allocates a response DTO"}},
		{name: "variable then field assignment", files: handlerFiles(dtoImportLine +
			"func f() { var r dto.ProductResponse; r.ID = 1; _ = r }\n"), want: []string{"handler declares a variable of a response DTO"}},
		{name: "conversion", files: handlerFiles(dtoImportLine +
			"type view struct{ ID int64 }\nfunc f(v view) { _ = dto.ProductResponse(v) }\n"), want: []string{"handler converts to a response DTO"}},
		// The envelope exception is gone: an envelope put back in dto is a
		// response DTO like any other *Response type.
		{name: "envelope declared in dto again", files: withFiles(handlerFiles(dtoImportLine+
			"func f() { _ = dto.APIResponse{Success: true} }\n"), map[string]string{
			"internal/dto/envelope.go": "package dto\ntype APIResponse struct{ Success bool }\n",
		}), want: []string{"h.go:3:16: handler builds a response DTO"}},
	}, check)
}

func TestUsecaseResultGuard(t *testing.T) {
	allow := map[string]string{"AuthUseCase.Authenticate": "principal"}
	check := func(fsys fs.FS) ([]string, error) { return checkUsecaseResults(fsys, allow) }

	uc := func(body string) map[string]string {
		return map[string]string{
			"internal/domain/user.go": guardDomain,
			"internal/usecase/uc.go": "package usecase\nimport (\n\"gostock/internal/domain\"\n\"gostock/internal/dto\"\n)\n" +
				"type UserUseCase struct{ repo any }\ntype AuthUseCase struct{ repo any }\n" +
				"func (uc *AuthUseCase) Authenticate() (*domain.User, error) { return nil, nil }\n" +
				"var _ dto.X\n" + body,
		}
	}
	runGuardCases(t, []guardCase{
		// Allowed.
		{name: "DTO results", files: uc("func (uc *UserUseCase) Get() (dto.UserResponse, error) { return dto.UserResponse{}, nil }\n")},
		{name: "domain only in parameters", files: uc("func (uc *UserUseCase) Save(u *domain.User, us []domain.User) error { return nil }\n")},
		{name: "unexported method", files: uc("func (uc *UserUseCase) load() *domain.User { return nil }\n")},
		{name: "method of an unexported type", files: uc("type helper struct{ n int }\nfunc (h helper) User() domain.User { return domain.User{} }\n")},
		{name: "exported function without domain result", files: uc("func IsAdmin(u *domain.User) bool { return false }\n")},

		// Rejected.
		{name: "value", files: uc("func (uc *UserUseCase) Get() (domain.User, error) { return domain.User{}, nil }\n"),
			want: []string{"UserUseCase.Get returns a domain type"}},
		{name: "pointer", files: uc("func (uc *UserUseCase) Get() *domain.User { return nil }\n"),
			want: []string{"UserUseCase.Get returns a domain type"}},
		{name: "slice", files: uc("func (uc *UserUseCase) List() ([]domain.User, int64, error) { return nil, 0, nil }\n"),
			want: []string{"UserUseCase.List returns a domain type"}},
		{name: "map", files: uc("func (uc *UserUseCase) Index() map[int64]*domain.User { return nil }\n"),
			want: []string{"UserUseCase.Index"}},
		{name: "channel", files: uc("func (uc *UserUseCase) Watch() <-chan domain.User { return nil }\n"),
			want: []string{"UserUseCase.Watch"}},
		{name: "function type", files: uc("func (uc *UserUseCase) Loader() func() domain.User { return nil }\n"),
			want: []string{"UserUseCase.Loader"}},
		{name: "generic type argument", files: uc("type Page[T any] []T\nfunc (uc *UserUseCase) Page() Page[domain.User] { return nil }\n"),
			want: []string{"UserUseCase.Page"}},
		{name: "value receiver, middle result", files: uc("func (uc UserUseCase) Pair() (int, *domain.User, error) { return 0, nil, nil }\n"),
			want: []string{"UserUseCase.Pair"}},
		{name: "local alias of a domain type", files: uc("type Principal = domain.User\nfunc (uc *UserUseCase) Me() Principal { return Principal{} }\n"),
			want: []string{"UserUseCase.Me"}},
		{name: "local type built from a domain type", files: uc("type Users []domain.User\ntype Page struct{ n int }\nfunc (uc *UserUseCase) All() Users { return nil }\n"),
			want: []string{"UserUseCase.All"}},
		{name: "exported package function", files: uc("func Lookup() *domain.User { return nil }\n"),
			want: []string{"Lookup returns a domain type"}},
		{name: "aliased domain import", files: map[string]string{
			"internal/domain/user.go": guardDomain,
			"internal/usecase/uc.go": "package usecase\nimport dm \"gostock/internal/domain\"\n" +
				"type AuthUseCase struct{ r any }\nfunc (uc *AuthUseCase) Authenticate() *dm.User { return nil }\n" +
				"type UserUseCase struct{ r any }\nfunc (uc *UserUseCase) Get() *dm.User { return nil }\n",
		}, want: []string{"UserUseCase.Get"}},
		{name: "dot-imported domain", files: map[string]string{
			"internal/domain/user.go": guardDomain,
			"internal/usecase/uc.go": "package usecase\nimport . \"gostock/internal/domain\"\n" +
				"type AuthUseCase struct{ r any }\nfunc (uc *AuthUseCase) Authenticate() *User { return nil }\n" +
				"type UserUseCase struct{ r any }\nfunc (uc *UserUseCase) Get() []User { return nil }\n",
		}, want: []string{"UserUseCase.Get"}},
		{name: "allowlist is per method, not per receiver", files: uc("func (uc *AuthUseCase) Principal() *domain.User { return nil }\n"),
			want: []string{"AuthUseCase.Principal returns a domain type"}},
	}, check)

	t.Run("stale allowlist entries", func(t *testing.T) {
		fsys := fstest.MapFS{
			"internal/domain/user.go": {Data: []byte(guardDomain)},
			"internal/usecase/uc.go": {Data: []byte("package usecase\nimport \"gostock/internal/dto\"\n" +
				"type AuthUseCase struct{ r any }\nfunc (uc *AuthUseCase) Authenticate() dto.AuthUserResponse { return dto.AuthUserResponse{} }\n")},
		}
		got, err := checkUsecaseResults(fsys, map[string]string{
			"AuthUseCase.Authenticate": "principal",
			"AdminUseCase.CreateAdmin": "cli",
		})
		if err != nil {
			t.Fatal(err)
		}
		want := []string{
			`"AdminUseCase.CreateAdmin" matches no exported use case method`,
			`"AuthUseCase.Authenticate" is not needed`,
		}
		if len(got) != len(want) {
			t.Fatalf("got %q, want %d violations", got, len(want))
		}
		for i := range want {
			if !strings.Contains(got[i], want[i]) {
				t.Errorf("violation %d = %q, want it to contain %q", i, got[i], want[i])
			}
		}
	})
}

func TestDomainAndDTOImportGuard(t *testing.T) {
	check := func(fsys fs.FS) ([]string, error) {
		var all []string
		for _, dir := range []string{domainPackage, dtoPackage} {
			v, err := checkImports(fsys, dir, isInternalImport, dir+" must not depend on other internal packages")
			if err != nil {
				return nil, err
			}
			all = append(all, v...)
		}
		return all, nil
	}
	runGuardCases(t, []guardCase{
		{name: "standard library and external modules", files: map[string]string{
			"internal/domain/user.go": "package domain\nimport \"time\"\ntype User struct{ At time.Time }\n",
			"internal/dto/token.go":   "package dto\nimport (\n\"time\"\n\"github.com/google/uuid\"\n)\ntype T struct{ At time.Time; ID uuid.UUID }\n",
			"internal/dto/cfg.go":     "package dto\nimport \"gostock/config\"\nvar _ config.X\n",
		}},
		{name: "dto imports domain", files: map[string]string{
			"internal/dto/auth.go": "package dto\nimport \"gostock/internal/domain\"\ntype R struct{ U *domain.User }\n",
		}, want: []string{`internal/dto/auth.go:2:8: imports "gostock/internal/domain"; internal/dto must not depend`}},
		{name: "domain imports dto", files: map[string]string{
			"internal/domain/user.go": "package domain\nimport \"gostock/internal/dto\"\ntype U struct{ R dto.R }\n",
		}, want: []string{`imports "gostock/internal/dto"; internal/domain must not depend`}},
		{name: "violation in a second file of the package", files: map[string]string{
			"internal/dto/a.go": "package dto\ntype A struct{ N int }\n",
			"internal/dto/b.go": "package dto\nimport u \"gostock/internal/usecase\"\nvar _ u.X\n",
		}, want: []string{`internal/dto/b.go:2:8: imports "gostock/internal/usecase"`}},
		{name: "subpackage of domain", files: map[string]string{
			"internal/domain/stock/m.go": "package stock\nimport \"gostock/internal/repository\"\n",
		}, want: []string{`internal/domain/stock/m.go:2:8: imports "gostock/internal/repository"`}},
		{name: "test files are not production imports", files: map[string]string{
			"internal/dto/x_test.go": "package dto\nimport \"gostock/internal/mapper\"\n",
		}},
	}, check)
}

func TestDomainDeclarationGuard(t *testing.T) {
	domainFile := func(src string) map[string]string {
		return map[string]string{"internal/domain/d.go": "package domain\n" + src}
	}
	runGuardCases(t, []guardCase{
		// Allowed: the real inventory_status.go form, other literals, imports.
		{name: "struct and vocabulary constants", files: domainFile("import \"time\"\n" +
			"type Product struct{ ID int64; At time.Time }\n" +
			"const (\n\tInventoryStatusNormal = \"normal\"\n\tInventoryStatusLowStock = \"low_stock\"\n)\n" +
			"const MaxNameLength int = 200\nconst Offset = -7\nconst Ratio = 0.5\nconst Enabled = true\n")},
		{name: "generic struct", files: domainFile("type Page[T any] struct{ Items []T }\n")},
		{name: "test files may declare anything", files: map[string]string{
			"internal/domain/d_test.go": "package domain\nfunc helper() {}\nvar x = 1\n",
		}},
		// Allowed (XG-04): named basic types and literal constants of them.
		{name: "named basic types and their constants", files: domainFile(
			"type Money int64\ntype Status string\ntype Role string\n" +
				"const A Status = \"a\"\nconst B Money = 100\nconst Refund Money = -5\n" +
				"const (\n\tRoleUser Role = \"user\"\n\tRoleAdmin Role = \"admin\"\n)\n")},
		{name: "named basic type declared in another file of the package", files: map[string]string{
			"internal/domain/money.go":  "package domain\ntype Money int64\n",
			"internal/domain/limits.go": "package domain\nconst MaxLine Money = 1000000\n",
		}},

		// Rejected.
		{name: "function", files: domainFile("func IsLow(q, m int) bool { return q <= m }\n"),
			want: []string{"d.go:2:1: function IsLow"}},
		{name: "method", files: domainFile("type T struct{ N int }\nfunc (t *T) Double() int { return t.N * 2 }\n"),
			want: []string{"method T.Double"}},
		{name: "package variable", files: domainFile("var Statuses = []string{\"normal\"}\n"),
			want: []string{"package variable Statuses"}},
		{name: "interface", files: domainFile("type Repo interface{ Get() }\n"),
			want: []string{"interface Repo"}},
		{name: "method on a named basic type", files: domainFile("type Money int64\nfunc (m Money) Add(n Money) Money { return m + n }\n"),
			want: []string{"method Money.Add"}},
		{name: "function on a named basic type", files: domainFile("type Money int64\nfunc Sum(a, b Money) Money { return a + b }\n"),
			want: []string{"function Sum"}},
		{name: "package variable of a named basic type", files: domainFile("type Money int64\nvar Zero Money = 0\n"),
			want: []string{"package variable Zero"}},
		{name: "named non-basic types", files: domainFile("type Tags []string\ntype Ptr *int64\ntype Index map[string]int\n" +
			"type Fn func()\ntype Ch chan int\ntype Arr [2]int\n"),
			want: []string{"non-struct type Tags", "non-struct type Ptr", "non-struct type Index",
				"non-struct type Fn", "non-struct type Ch", "non-struct type Arr"}},
		{name: "alias of a basic type", files: domainFile("type Name = string\n"),
			want: []string{"non-struct type Name"}},
		{name: "type based on another package's type", files: domainFile("import \"time\"\ntype Wait time.Duration\n"),
			want: []string{"non-struct type Wait"}},
		{name: "type based on a named basic type", files: domainFile("type Money int64\ntype Price Money\n"),
			want: []string{"non-struct type Price"}},
		{name: "computed constant of a named basic type", files: domainFile("type Money int64\nconst C Money = 1 << 3\n"),
			want: []string{"constant C is computed"}},
		{name: "iota expression of a named basic type", files: domainFile("type Status int\nconst (\n\tD Status = iota + 1\n\tE\n)\n"),
			want: []string{"constant D is computed", "constant E has no literal value of its own"}},
		{name: "constant of another package's type", files: domainFile("import \"time\"\nconst Wait time.Duration = 5\n"),
			want: []string{"constant Wait has a non-basic type"}},
		{name: "constant of a named basic type from another domain package", files: map[string]string{
			"internal/domain/money.go":   "package domain\ntype Money int64\n",
			"internal/domain/stock/s.go": "package stock\nimport \"gostock/internal/domain\"\nconst A domain.Money = 1\nconst B Money = 2\n",
		}, want: []string{"constant A has a non-basic type", "constant B has a non-basic type"}},
		{name: "constant of an alias of a basic type", files: domainFile("type Name = string\nconst N Name = \"n\"\n"),
			want: []string{"non-struct type Name", "constant N has a non-basic type"}},
		{name: "alias type", files: domainFile("type A struct{ N int }\ntype B = A\n"),
			want: []string{"non-struct type B"}},
		{name: "alias to a struct literal", files: domainFile("type B = struct{ N int }\n"),
			want: []string{"alias type B"}},
		{name: "iota and implicit repetition", files: domainFile("const (\n\tA = iota\n\tB\n)\n"),
			want: []string{"constant A is computed", "constant B has no literal value of its own"}},
		{name: "computed constant", files: domainFile("const Max = 10 * 20\n"),
			want: []string{"constant Max is computed"}},
		{name: "constant referencing another", files: domainFile("const A = \"a\"\nconst B = A\n"),
			want: []string{"constant B is computed"}},
		{name: "constant of a non-basic type", files: domainFile("type S struct{ N int }\nconst X S = 1\n"),
			want: []string{"constant X has a non-basic type"}},
		{name: "violation in a domain subpackage", files: map[string]string{
			"internal/domain/stock/s.go": "package stock\nfunc F() {}\n",
		}, want: []string{"internal/domain/stock/s.go:2:1: function F"}},
	}, func(fsys fs.FS) ([]string, error) { return checkDomainDeclarations(fsys) })
}
