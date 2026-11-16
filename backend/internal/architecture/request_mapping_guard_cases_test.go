package architecture

import (
	"io/fs"
	"reflect"
	"testing"
	"testing/fstest"
)

// Fixture tests for the request-side guards (Step 5.5): each case is a small
// in-memory module checked by the same functions as the real one. No
// production file is changed.

// ---------------------------------------------------------------------------
// Guard 1: the HTTP layer does not construct a domain Product.
// ---------------------------------------------------------------------------

const (
	reqGuardDomain = "package domain\n" +
		"type Product struct{ ID int64; SKU string }\n" +
		"type User struct{ ID int64; Role string }\n"
	domainImportLine = "import \"gostock/internal/domain\"\n"
)

// domainHandlerFiles returns a fixture module with the guard domain and one
// handler file with the given body (after the package clause).
func domainHandlerFiles(body string) map[string]string {
	return map[string]string{
		"internal/domain/domain.go": reqGuardDomain,
		"internal/handle/http/h.go": "package http\n" + body,
	}
}

func TestHandlerDomainConstructionGuard(t *testing.T) {
	check := func(fsys fs.FS) ([]string, error) {
		return checkHandlerDomainConstruction(fsys, handlerUnbuildableDomainTypes)
	}
	runGuardCases(t, []guardCase{
		// Allowed.
		{name: "domain.User variable and parameter (principal)", files: domainHandlerFiles(domainImportLine +
			"var user domain.User\n" +
			"func authenticate(user domain.User) {}\n")},
		{name: "domain.User literals are not guarded", files: domainHandlerFiles(domainImportLine +
			"func f() { _ = domain.User{ID: 1}; _ = &domain.User{}; _ = new(domain.User); _ = []domain.User{{ID: 2}} }\n")},
		{name: "auth middleware shape: principal in the request context", files: domainHandlerFiles(
			"import (\n\"context\"\n\"gostock/internal/domain\"\n)\n" +
				"type ctxKey struct{}\n" +
				"func withUser(ctx context.Context, u *domain.User) context.Context { return context.WithValue(ctx, ctxKey{}, u) }\n" +
				"func userFrom(ctx context.Context) (*domain.User, bool) { u, ok := ctx.Value(ctxKey{}).(*domain.User); return u, ok }\n")},
		{name: "Product received, not constructed", files: domainHandlerFiles(domainImportLine +
			"func sku(p domain.Product) string { return p.SKU }\n" +
			"func count(ps []*domain.Product) int { return len(ps) }\n")},
		{name: "another package named domain", files: domainHandlerFiles("import \"example.com/catalog/domain\"\n" +
			"func f() { _ = domain.Product{}; _ = &domain.Product{} }\n")},
		{name: "a handler-local type named Product", files: domainHandlerFiles(
			"type Product struct{ ID int64 }\nfunc f() { _ = Product{ID: 1} }\n")},
		{name: "input DTO built by an adapter", files: domainHandlerFiles(
			"import \"gostock/internal/dto\"\nfunc f() { _ = dto.CreateProductInput{SKU: \"x\"} }\n")},
		{name: "handler tests may build products", files: map[string]string{
			"internal/domain/domain.go":      reqGuardDomain,
			"internal/handle/http/h_test.go": "package http\n" + domainImportLine + "func f() { _ = domain.Product{} }\n",
		}},

		// Rejected.
		{name: "empty value literal", files: domainHandlerFiles(domainImportLine +
			"func f() { product := domain.Product{}; _ = product }\n"),
			want: []string{"h.go:3:23: handler builds domain.Product"}},
		{name: "pointer literal", files: domainHandlerFiles(domainImportLine +
			"func f() { product := &domain.Product{}; _ = product }\n"),
			want: []string{"h.go:3:24: handler builds domain.Product"}},
		{name: "literal with fields", files: domainHandlerFiles(domainImportLine +
			"func f() {\n\tproduct := domain.Product{\n\t\tSKU: \"SKU-001\",\n\t}\n\t_ = product\n}\n"),
			want: []string{"h.go:4:13: handler builds domain.Product"}},
		{name: "aliased domain import", files: domainHandlerFiles("import dm \"gostock/internal/domain\"\n" +
			"func f() { _ = dm.Product{SKU: \"x\"} }\n"),
			want: []string{"handler builds domain.Product"}},
		{name: "dot-imported domain", files: domainHandlerFiles("import . \"gostock/internal/domain\"\n" +
			"func f() { _ = Product{}; _ = User{} }\n"),
			want: []string{"handler builds domain.Product"}},
		{name: "slice literal with elided element types", files: domainHandlerFiles(domainImportLine +
			"func f() { _ = []domain.Product{{SKU: \"a\"}, {SKU: \"b\"}} }\n"),
			want: []string{"handler builds domain.Product"}},
		{name: "map literal", files: domainHandlerFiles(domainImportLine +
			"func f() { _ = map[int64]*domain.Product{} }\n"),
			want: []string{"handler builds domain.Product"}},
		{name: "new", files: domainHandlerFiles(domainImportLine +
			"func f() { _ = new(domain.Product) }\n"),
			want: []string{"handler allocates domain.Product"}},
		{name: "variable then field assignment", files: domainHandlerFiles(domainImportLine +
			"func f() { var p domain.Product; p.SKU = \"x\"; _ = p }\n"),
			want: []string{"handler declares a variable of domain.Product"}},
		{name: "conversion", files: domainHandlerFiles(domainImportLine +
			"type form struct{ ID int64; SKU string }\nfunc f(v form) { _ = domain.Product(v) }\n"),
			want: []string{"handler converts to domain.Product"}},
		{name: "local alias of domain.Product", files: domainHandlerFiles(domainImportLine +
			"type product = domain.Product\nfunc f() { _ = product{SKU: \"x\"} }\n"),
			want: []string{"handler builds local type product (defined from a guarded domain type)"}},
		{name: "inside a function literal", files: domainHandlerFiles(domainImportLine +
			"var build = func() any { return domain.Product{} }\n"),
			want: []string{"handler builds domain.Product"}},
		{name: "response helper subpackage", files: map[string]string{
			"internal/domain/domain.go":          reqGuardDomain,
			"internal/handle/http/response/r.go": "package response\n" + domainImportLine + "func f() { _ = domain.Product{} }\n",
		}, want: []string{"internal/handle/http/response/r.go:3:16: handler builds domain.Product"}},
		{name: "every site is reported", files: domainHandlerFiles(domainImportLine +
			"func create() { _ = domain.Product{} }\nfunc update() { _ = &domain.Product{ID: 7} }\n"),
			want: []string{"h.go:3:21: handler builds", "h.go:4:22: handler builds"}},
	}, check)

	t.Run("stale entry", func(t *testing.T) {
		fsys := fstest.MapFS{
			"internal/domain/domain.go": {Data: []byte(reqGuardDomain)},
		}
		got, err := checkHandlerDomainConstruction(fsys, map[string]string{"Product": "p", "Order": "o"})
		if err != nil {
			t.Fatal(err)
		}
		want := []string{`handlerUnbuildableDomainTypes entry "Order" matches no type declared in internal/domain; remove it`}
		if !reflect.DeepEqual(got, want) {
			t.Fatalf("got %q, want %q", got, want)
		}
	})
}

// ---------------------------------------------------------------------------
// Guard 2: public use case methods do not take HTTP request DTOs.
// ---------------------------------------------------------------------------

// reqGuardDTO has JSON-tagged request and response types and untagged input
// and query types. LoginBody breaks the naming convention: it is a request DTO
// only because reqGuardHandler decodes a request into it.
const (
	reqGuardDTO = "package dto\n" +
		"type LoginRequest struct{ Email string `json:\"email\"` }\n" +
		"type CreateProductRequest struct{ SKU string `json:\"sku\"` }\n" +
		"type LoginBody struct{ Email string `json:\"email\"` }\n" +
		"type CreateProductInput struct{ SKU string }\n" +
		"type UpdateProductInput struct{ ID int64; MinimumStock *int }\n" +
		"type ChartQuery struct{ From string }\n" +
		"type ProductResponse struct{ ID int64 `json:\"id\"` }\n"
	reqGuardHandler = "package http\n" +
		"import (\n\"encoding/json\"\n\"net/http\"\n\"gostock/internal/dto\"\n)\n" +
		"func login(r *http.Request) { var body dto.LoginBody; _ = json.NewDecoder(r.Body).Decode(&body) }\n" +
		"func adapt(in *dto.CreateProductInput) {}\n" +
		"func create() { var in dto.CreateProductInput; adapt(&in) }\n"
)

// usecaseFiles returns a fixture module with the guard dto and handler and one
// use case file with the given body.
func usecaseFiles(body string) map[string]string {
	return map[string]string{
		"internal/dto/dto.go":       reqGuardDTO,
		"internal/handle/http/h.go": reqGuardHandler,
		"internal/usecase/uc.go": "package usecase\nimport (\n\"context\"\n\"gostock/internal/dto\"\n)\n" +
			"type ProductUseCase struct{ repo any }\ntype AuthUseCase struct{ repo any }\n" +
			"var _ context.Context\nvar _ dto.ProductResponse\n" + body,
	}
}

func TestRequestDTODerivation(t *testing.T) {
	mapFS := func(files map[string]string) fstest.MapFS {
		fsys := fstest.MapFS{}
		for name, src := range files {
			fsys[name] = &fstest.MapFile{Data: []byte(src)}
		}
		return fsys
	}
	derive := func(t *testing.T, files map[string]string) (all, decoded []string) {
		t.Helper()
		fsys := mapFS(files)
		set, err := requestDTOTypes(fsys)
		if err != nil {
			t.Fatal(err)
		}
		dec, err := decodedDTOTypes(fsys)
		if err != nil {
			t.Fatal(err)
		}
		return sortedKeys(set), sortedKeys(dec)
	}

	t.Run("naming convention and decode targets; input, query and response DTOs excluded", func(t *testing.T) {
		all, decoded := derive(t, usecaseFiles(""))
		if want := []string{"CreateProductRequest", "LoginBody", "LoginRequest"}; !reflect.DeepEqual(all, want) {
			t.Errorf("request DTO set = %v, want %v", all, want)
		}
		if want := []string{"LoginBody"}; !reflect.DeepEqual(decoded, want) {
			t.Errorf("decode targets = %v, want %v", decoded, want)
		}
	})

	decodeForms := map[string]string{
		"var then Decode(&v)":          "func f(r *http.Request) { var req dto.ChartQuery; _ = json.NewDecoder(r.Body).Decode(&req) }\n",
		"short declaration of &T{}":    "func f(r *http.Request) { req := &dto.ChartQuery{}; _ = json.NewDecoder(r.Body).Decode(req) }\n",
		"new(T)":                       "func f(r *http.Request) { req := new(dto.ChartQuery); _ = json.NewDecoder(r.Body).Decode(req) }\n",
		"json.Unmarshal":               "func f(b []byte) { var req dto.ChartQuery; _ = json.Unmarshal(b, &req) }\n",
		"literal passed directly":      "func f(r *http.Request) { _ = json.NewDecoder(r.Body).Decode(&dto.ChartQuery{}) }\n",
		"pointer parameter":            "func f(r *http.Request, req *dto.ChartQuery) { _ = json.NewDecoder(r.Body).Decode(req) }\n",
		"inside a function literal":    "func f(r *http.Request) { g := func(req *dto.ChartQuery) { _ = json.NewDecoder(r.Body).Decode(req) }; _ = g }\n",
		"aliased dto import (d.T)":     "func f(r *http.Request) { var req d.ChartQuery; _ = json.NewDecoder(r.Body).Decode(&req) }\n",
		"subpackage of the HTTP layer": "",
	}
	for name, body := range decodeForms {
		t.Run("decode target: "+name, func(t *testing.T) {
			files := usecaseFiles("")
			imports := "import (\n\"encoding/json\"\n\"net/http\"\n\"gostock/internal/dto\"\n)\nvar _ = dto.ChartQuery{}\nvar _ http.Header\n"
			switch {
			case name == "aliased dto import (d.T)":
				imports = "import (\n\"encoding/json\"\n\"net/http\"\nd \"gostock/internal/dto\"\n)\nvar _ http.Header\n"
			case body == "":
				files["internal/handle/http/sub/s.go"] = "package sub\n" + imports +
					"func f(b []byte) { var req dto.ChartQuery; _ = json.Unmarshal(b, &req) }\n"
			}
			if body != "" {
				files["internal/handle/http/q.go"] = "package http\n" + imports + body
			}
			_, decoded := derive(t, files)
			if want := []string{"ChartQuery", "LoginBody"}; !reflect.DeepEqual(decoded, want) {
				t.Errorf("decode targets = %v, want %v", decoded, want)
			}
		})
	}

	t.Run("test files are not decode sites", func(t *testing.T) {
		files := usecaseFiles("")
		files["internal/handle/http/q_test.go"] = "package http\nimport (\n\"encoding/json\"\n\"gostock/internal/dto\"\n)\n" +
			"func f(b []byte) { var q dto.ChartQuery; _ = json.Unmarshal(b, &q) }\n"
		if _, decoded := derive(t, files); !reflect.DeepEqual(decoded, []string{"LoginBody"}) {
			t.Errorf("decode targets = %v, want [LoginBody]", decoded)
		}
	})
}

func TestUsecaseRequestParamGuard(t *testing.T) {
	check := func(fsys fs.FS) ([]string, error) {
		types, err := requestDTOTypes(fsys)
		if err != nil {
			return nil, err
		}
		return checkUsecaseParams(fsys, types)
	}
	runGuardCases(t, []guardCase{
		// Allowed.
		{name: "CreateProductInput", files: usecaseFiles(
			"func (uc *ProductUseCase) Create(ctx context.Context, in dto.CreateProductInput, actorID int64) (dto.ProductResponse, error) { return dto.ProductResponse{}, nil }\n")},
		{name: "UpdateProductInput", files: usecaseFiles(
			"func (uc *ProductUseCase) Update(ctx context.Context, in dto.UpdateProductInput) (dto.ProductResponse, error) { return dto.ProductResponse{}, nil }\n")},
		{name: "pointer to an input and a query DTO", files: usecaseFiles(
			"func (uc *ProductUseCase) Chart(in *dto.CreateProductInput, q dto.ChartQuery) error { return nil }\n")},
		{name: "JSON-tagged response DTO is not a request DTO", files: usecaseFiles(
			"func (uc *ProductUseCase) Echo(p dto.ProductResponse) dto.ProductResponse { return p }\n")},
		{name: "plain values", files: usecaseFiles(
			"func (uc *AuthUseCase) Login(ctx context.Context, email, password string) error { return nil }\n")},
		{name: "unexported method", files: usecaseFiles(
			"func (uc *ProductUseCase) create(req dto.CreateProductRequest) error { return nil }\n")},
		{name: "method of an unexported type", files: usecaseFiles(
			"type helper struct{ n int }\nfunc (h helper) Parse(req dto.LoginRequest) error { return nil }\n")},
		{name: "request DTO in a result is out of scope", files: usecaseFiles(
			"func (uc *ProductUseCase) Template() dto.CreateProductRequest { return dto.CreateProductRequest{} }\n")},

		// Rejected.
		{name: "dto.LoginRequest", files: usecaseFiles(
			"func (uc *AuthUseCase) Login(ctx context.Context, req dto.LoginRequest) error { return nil }\n"),
			want: []string{"uc.go:10:51: AuthUseCase.Login takes dto.LoginRequest, an HTTP request DTO"}},
		{name: "*dto.CreateProductRequest", files: usecaseFiles(
			"func (uc *ProductUseCase) Create(ctx context.Context, req *dto.CreateProductRequest, actorID int64) error { return nil }\n"),
			want: []string{"ProductUseCase.Create takes dto.CreateProductRequest"}},
		{name: "value receiver", files: usecaseFiles(
			"func (uc ProductUseCase) Create(req dto.CreateProductRequest) error { return nil }\n"),
			want: []string{"ProductUseCase.Create takes dto.CreateProductRequest"}},
		{name: "decode-derived type outside the naming convention", files: usecaseFiles(
			"func (uc *AuthUseCase) Login(body dto.LoginBody) error { return nil }\n"),
			want: []string{"AuthUseCase.Login takes dto.LoginBody"}},
		{name: "slice", files: usecaseFiles(
			"func (uc *ProductUseCase) Import(reqs []dto.CreateProductRequest) error { return nil }\n"),
			want: []string{"ProductUseCase.Import"}},
		{name: "map", files: usecaseFiles(
			"func (uc *ProductUseCase) Import(reqs map[string]*dto.CreateProductRequest) error { return nil }\n"),
			want: []string{"ProductUseCase.Import"}},
		{name: "variadic", files: usecaseFiles(
			"func (uc *ProductUseCase) Import(reqs ...dto.CreateProductRequest) error { return nil }\n"),
			want: []string{"ProductUseCase.Import"}},
		{name: "function type", files: usecaseFiles(
			"func (uc *AuthUseCase) With(f func(dto.LoginRequest) error) error { return nil }\n"),
			want: []string{"AuthUseCase.With"}},
		{name: "channel", files: usecaseFiles(
			"func (uc *AuthUseCase) Watch(c <-chan dto.LoginRequest) {}\n"),
			want: []string{"AuthUseCase.Watch"}},
		{name: "generic type argument", files: usecaseFiles(
			"type Batch[T any] []T\nfunc (uc *ProductUseCase) Import(b Batch[dto.CreateProductRequest]) {}\n"),
			want: []string{"ProductUseCase.Import"}},
		{name: "local alias of a request DTO", files: usecaseFiles(
			"type Credentials = dto.LoginRequest\nfunc (uc *AuthUseCase) Login(c Credentials) error { return nil }\n"),
			want: []string{"AuthUseCase.Login takes local type Credentials (defined from a request DTO)"}},
		{name: "local type built from a request DTO", files: usecaseFiles(
			"type Requests []dto.CreateProductRequest\nfunc (uc *ProductUseCase) Import(r Requests) error { return nil }\n"),
			want: []string{"ProductUseCase.Import takes local type Requests"}},
		{name: "aliased dto import", files: map[string]string{
			"internal/dto/dto.go":       reqGuardDTO,
			"internal/handle/http/h.go": reqGuardHandler,
			"internal/usecase/uc.go": "package usecase\nimport d \"gostock/internal/dto\"\n" +
				"type AuthUseCase struct{ r any }\nfunc (uc *AuthUseCase) Login(req d.LoginRequest) error { return nil }\n",
		}, want: []string{"AuthUseCase.Login takes dto.LoginRequest"}},
		{name: "dot-imported dto", files: map[string]string{
			"internal/dto/dto.go":       reqGuardDTO,
			"internal/handle/http/h.go": reqGuardHandler,
			"internal/usecase/uc.go": "package usecase\nimport . \"gostock/internal/dto\"\n" +
				"type AuthUseCase struct{ r any }\nfunc (uc *AuthUseCase) Login(req *LoginRequest, in CreateProductInput) error { return nil }\n",
		}, want: []string{"AuthUseCase.Login takes dto.LoginRequest"}},
		{name: "exported package function", files: usecaseFiles(
			"func ValidateLogin(req dto.LoginRequest) error { return nil }\n"),
			want: []string{"ValidateLogin takes dto.LoginRequest"}},
		{name: "one violation per method, each method reported", files: usecaseFiles(
			"func (uc *AuthUseCase) Login(a dto.LoginRequest, b dto.LoginRequest) error { return nil }\n" +
				"func (uc *ProductUseCase) Create(req dto.CreateProductRequest) error { return nil }\n"),
			want: []string{"AuthUseCase.Login", "ProductUseCase.Create"}},
	}, check)
}
