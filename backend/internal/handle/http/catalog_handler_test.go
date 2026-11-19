package http

import (
	"context"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"gostock/internal/domain"
	"gostock/internal/repository"
	"gostock/internal/usecase"
)

var catalogTime = time.Date(2026, 11, 20, 8, 0, 0, 0, time.UTC)

const catalogTimeJSON = `"2026-11-20T08:00:00Z"`

// catalogRepo implements both catalog repositories: ID 1 exists, a created
// row gets ID 10, and deleteErr is returned by Delete.
type catalogRepo struct {
	deleteErr error
	writes    int
}

func (c *catalogRepo) wood() repository.WoodTypeRepository  { return woodTypeRepoAdapter{c} }
func (c *catalogRepo) suppl() repository.SupplierRepository { return supplierRepoAdapter{c} }

type woodTypeRepoAdapter struct{ c *catalogRepo }

func (a woodTypeRepoAdapter) List(context.Context) ([]domain.WoodType, error) {
	return []domain.WoodType{{ID: 1, Name: "Gỗ lim", CreatedAt: catalogTime, UpdatedAt: catalogTime}}, nil
}
func (a woodTypeRepoAdapter) GetByID(_ context.Context, id int64) (*domain.WoodType, error) {
	if id != 1 {
		return nil, repository.ErrWoodTypeNotFound
	}
	return &domain.WoodType{ID: 1, Name: "Gỗ lim", CreatedAt: catalogTime, UpdatedAt: catalogTime}, nil
}
func (a woodTypeRepoAdapter) Create(_ context.Context, w *domain.WoodType) error {
	a.c.writes++
	w.ID, w.CreatedAt, w.UpdatedAt = 10, catalogTime, catalogTime
	return nil
}
func (a woodTypeRepoAdapter) Update(_ context.Context, w *domain.WoodType) error {
	a.c.writes++
	if w.ID != 1 {
		return repository.ErrWoodTypeNotFound
	}
	w.CreatedAt, w.UpdatedAt = catalogTime, catalogTime
	return nil
}
func (a woodTypeRepoAdapter) Delete(_ context.Context, id int64) error {
	a.c.writes++
	if id != 1 {
		return repository.ErrWoodTypeNotFound
	}
	return a.c.deleteErr
}

type supplierRepoAdapter struct{ c *catalogRepo }

func (a supplierRepoAdapter) List(context.Context) ([]domain.Supplier, error) {
	return []domain.Supplier{}, nil
}
func (a supplierRepoAdapter) GetByID(_ context.Context, id int64) (*domain.Supplier, error) {
	if id != 1 {
		return nil, repository.ErrSupplierNotFound
	}
	phone := "0368122137"
	return &domain.Supplier{ID: 1, Name: "An Phát", Phone: &phone, CreatedAt: catalogTime, UpdatedAt: catalogTime}, nil
}
func (a supplierRepoAdapter) Create(_ context.Context, s *domain.Supplier) error {
	a.c.writes++
	s.ID, s.CreatedAt, s.UpdatedAt = 10, catalogTime, catalogTime
	return nil
}
func (a supplierRepoAdapter) Update(_ context.Context, s *domain.Supplier) error {
	a.c.writes++
	return nil
}
func (a supplierRepoAdapter) Delete(_ context.Context, id int64) error {
	a.c.writes++
	if id != 1 {
		return repository.ErrSupplierNotFound
	}
	return a.c.deleteErr
}

// serveCatalog sends one request to the handler registered for method on
// pattern; contentType "" sends none.
func serveCatalog(h http.HandlerFunc, method, pattern, target, body, contentType string) *httptest.ResponseRecorder {
	req := httptest.NewRequest(method, target, strings.NewReader(body))
	if contentType != "" {
		req.Header.Set("Content-Type", contentType)
	}
	rec := httptest.NewRecorder()
	serveRoute(rec, req, method, pattern, h)
	return rec
}

func TestWoodTypeHandler(t *testing.T) {
	const json = "application/json"
	tests := []struct {
		name                       string
		method, target, body, ctyp string
		deleteErr                  error
		wantStatus                 int
		wantBody                   string
	}{
		{"list", http.MethodGet, "/api/v1/wood-types", "", "", nil, http.StatusOK,
			`{"success":true,"message":"Wood types retrieved successfully","data":[{"id":1,"name":"Gỗ lim","created_at":` + catalogTimeJSON + `,"updated_at":` + catalogTimeJSON + `}]}`},
		{"create", http.MethodPost, "/api/v1/wood-types", `{"name":"  Gỗ lim "}`, json, nil, http.StatusCreated,
			`{"success":true,"message":"Wood type created successfully","data":{"id":10,"name":"Gỗ lim","created_at":` + catalogTimeJSON + `,"updated_at":` + catalogTimeJSON + `}}`},
		{"create blank name", http.MethodPost, "/api/v1/wood-types", `{"name":" "}`, json, nil, http.StatusBadRequest,
			`{"success":false,"code":"INVALID_WOOD_TYPE","message":"wood type name must have 1 to 100 characters","data":null}`},
		{"create without name", http.MethodPost, "/api/v1/wood-types", `{}`, json, nil, http.StatusBadRequest,
			`{"success":false,"code":"INVALID_WOOD_TYPE","message":"wood type name must have 1 to 100 characters","data":null}`},
		{"create unknown field", http.MethodPost, "/api/v1/wood-types", `{"name":"x","id":5}`, json, nil, http.StatusBadRequest,
			`{"success":false,"code":"VALIDATION_ERROR","message":"invalid JSON request","data":null}`},
		{"create two objects", http.MethodPost, "/api/v1/wood-types", `{"name":"x"}{}`, json, nil, http.StatusBadRequest,
			`{"success":false,"code":"VALIDATION_ERROR","message":"request must contain exactly one JSON object","data":null}`},
		{"create without Content-Type", http.MethodPost, "/api/v1/wood-types", `{"name":"x"}`, "", nil, http.StatusUnsupportedMediaType,
			`{"success":false,"code":"UNSUPPORTED_MEDIA_TYPE","message":"Content-Type must be application/json","data":null}`},
		{"update", http.MethodPatch, "/api/v1/wood-types/1", `{"name":"Gỗ sến"}`, json, nil, http.StatusOK,
			`{"success":true,"message":"Wood type updated successfully","data":{"id":1,"name":"Gỗ sến","created_at":` + catalogTimeJSON + `,"updated_at":` + catalogTimeJSON + `}}`},
		{"update empty body", http.MethodPatch, "/api/v1/wood-types/1", `{}`, json, nil, http.StatusBadRequest,
			`{"success":false,"code":"EMPTY_UPDATE","message":"at least one field must be provided for update","data":null}`},
		{"update null name", http.MethodPatch, "/api/v1/wood-types/1", `{"name":null}`, json, nil, http.StatusBadRequest,
			`{"success":false,"code":"EMPTY_UPDATE","message":"at least one field must be provided for update","data":null}`},
		{"update not found", http.MethodPatch, "/api/v1/wood-types/2", `{"name":"x"}`, json, nil, http.StatusNotFound,
			`{"success":false,"code":"WOOD_TYPE_NOT_FOUND","message":"wood type not found","data":null}`},
		{"update ID abc", http.MethodPatch, "/api/v1/wood-types/abc", `{"name":"x"}`, json, nil, http.StatusBadRequest,
			`{"success":false,"code":"INVALID_WOOD_TYPE_ID","message":"invalid wood type ID","data":null}`},
		{"update ID 0", http.MethodPatch, "/api/v1/wood-types/0", `{"name":"x"}`, json, nil, http.StatusBadRequest,
			`{"success":false,"code":"INVALID_WOOD_TYPE_ID","message":"invalid wood type ID","data":null}`},
		{"delete", http.MethodDelete, "/api/v1/wood-types/1", "", "", nil, http.StatusNoContent, ``},
		{"delete not found", http.MethodDelete, "/api/v1/wood-types/2", "", "", nil, http.StatusNotFound,
			`{"success":false,"code":"WOOD_TYPE_NOT_FOUND","message":"wood type not found","data":null}`},
		{"delete in use", http.MethodDelete, "/api/v1/wood-types/1", "", "", repository.ErrInUse, http.StatusConflict,
			`{"success":false,"code":"IN_USE","message":"the record is in use and cannot be deleted","data":null}`},
		{"delete ID -1", http.MethodDelete, "/api/v1/wood-types/-1", "", "", nil, http.StatusBadRequest,
			`{"success":false,"code":"INVALID_WOOD_TYPE_ID","message":"invalid wood type ID","data":null}`},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			repo := &catalogRepo{deleteErr: tt.deleteErr}
			h := NewWoodTypeHandler(usecase.NewWoodTypeUseCase(repo.wood()))
			handlers := map[string]http.HandlerFunc{
				http.MethodGet: h.List, http.MethodPost: h.Create, http.MethodPatch: h.Update, http.MethodDelete: h.Delete,
			}
			pattern := "/api/v1/wood-types"
			if tt.method == http.MethodPatch || tt.method == http.MethodDelete {
				pattern += "/{id}"
			}
			rec := serveCatalog(handlers[tt.method], tt.method, pattern, tt.target, tt.body, tt.ctyp)
			if tt.wantStatus == http.StatusNoContent {
				if rec.Code != http.StatusNoContent || rec.Body.Len() != 0 {
					t.Fatalf("got %d %q, want 204 with no body", rec.Code, rec.Body.String())
				}
				return
			}
			contractAssertResponse(t, rec, tt.wantStatus, tt.wantBody)
		})
	}
}

func TestSupplierHandler(t *testing.T) {
	const json = "application/json"
	tests := []struct {
		name                       string
		method, target, body, ctyp string
		deleteErr                  error
		wantStatus                 int
		wantBody                   string
		wantWrites                 int
	}{
		{"list empty is []", http.MethodGet, "/api/v1/suppliers", "", "", nil, http.StatusOK,
			`{"success":true,"message":"Suppliers retrieved successfully","data":[]}`, 0},
		{"create with phone normalized", http.MethodPost, "/api/v1/suppliers", `{"name":"An Phát","phone":"+84 368 122 137"}`, json, nil, http.StatusCreated,
			`{"success":true,"message":"Supplier created successfully","data":{"id":10,"name":"An Phát","phone":"0368122137","created_at":` + catalogTimeJSON + `,"updated_at":` + catalogTimeJSON + `}}`, 1},
		{"create without phone", http.MethodPost, "/api/v1/suppliers", `{"name":"An Phát"}`, json, nil, http.StatusCreated,
			`{"success":true,"message":"Supplier created successfully","data":{"id":10,"name":"An Phát","phone":null,"created_at":` + catalogTimeJSON + `,"updated_at":` + catalogTimeJSON + `}}`, 1},
		{"create invalid phone", http.MethodPost, "/api/v1/suppliers", `{"name":"An Phát","phone":"12345"}`, json, nil, http.StatusBadRequest,
			`{"success":false,"code":"INVALID_PHONE","message":"invalid Vietnamese phone number","data":null}`, 0},
		{"create blank name", http.MethodPost, "/api/v1/suppliers", `{"name":""}`, json, nil, http.StatusBadRequest,
			`{"success":false,"code":"INVALID_SUPPLIER","message":"supplier name must have 1 to 100 characters","data":null}`, 0},
		{"update removes the phone", http.MethodPatch, "/api/v1/suppliers/1", `{"phone":""}`, json, nil, http.StatusOK,
			`{"success":true,"message":"Supplier updated successfully","data":{"id":1,"name":"An Phát","phone":null,"created_at":` + catalogTimeJSON + `,"updated_at":` + catalogTimeJSON + `}}`, 1},
		{"update name keeps the phone", http.MethodPatch, "/api/v1/suppliers/1", `{"name":"Bình Minh"}`, json, nil, http.StatusOK,
			`{"success":true,"message":"Supplier updated successfully","data":{"id":1,"name":"Bình Minh","phone":"0368122137","created_at":` + catalogTimeJSON + `,"updated_at":` + catalogTimeJSON + `}}`, 1},
		{"update invalid phone", http.MethodPatch, "/api/v1/suppliers/1", `{"phone":"abc"}`, json, nil, http.StatusBadRequest,
			`{"success":false,"code":"INVALID_PHONE","message":"invalid Vietnamese phone number","data":null}`, 0},
		{"update empty body", http.MethodPatch, "/api/v1/suppliers/1", `{}`, json, nil, http.StatusBadRequest,
			`{"success":false,"code":"EMPTY_UPDATE","message":"at least one field must be provided for update","data":null}`, 0},
		{"update not found", http.MethodPatch, "/api/v1/suppliers/2", `{"name":"x"}`, json, nil, http.StatusNotFound,
			`{"success":false,"code":"SUPPLIER_NOT_FOUND","message":"supplier not found","data":null}`, 0},
		{"update ID abc", http.MethodPatch, "/api/v1/suppliers/abc", `{"name":"x"}`, json, nil, http.StatusBadRequest,
			`{"success":false,"code":"INVALID_SUPPLIER_ID","message":"invalid supplier ID","data":null}`, 0},
		{"update wrong Content-Type", http.MethodPatch, "/api/v1/suppliers/1", `{"name":"x"}`, "text/plain", nil, http.StatusUnsupportedMediaType,
			`{"success":false,"code":"UNSUPPORTED_MEDIA_TYPE","message":"Content-Type must be application/json","data":null}`, 0},
		{"delete not found", http.MethodDelete, "/api/v1/suppliers/2", "", "", nil, http.StatusNotFound,
			`{"success":false,"code":"SUPPLIER_NOT_FOUND","message":"supplier not found","data":null}`, 1},
		{"delete in use", http.MethodDelete, "/api/v1/suppliers/1", "", "", repository.ErrInUse, http.StatusConflict,
			`{"success":false,"code":"IN_USE","message":"the record is in use and cannot be deleted","data":null}`, 1},
		{"delete ID 0", http.MethodDelete, "/api/v1/suppliers/0", "", "", nil, http.StatusBadRequest,
			`{"success":false,"code":"INVALID_SUPPLIER_ID","message":"invalid supplier ID","data":null}`, 0},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			repo := &catalogRepo{deleteErr: tt.deleteErr}
			h := NewSupplierHandler(usecase.NewSupplierUseCase(repo.suppl()))
			handlers := map[string]http.HandlerFunc{
				http.MethodGet: h.List, http.MethodPost: h.Create, http.MethodPatch: h.Update, http.MethodDelete: h.Delete,
			}
			pattern := "/api/v1/suppliers"
			if tt.method == http.MethodPatch || tt.method == http.MethodDelete {
				pattern += "/{id}"
			}
			rec := serveCatalog(handlers[tt.method], tt.method, pattern, tt.target, tt.body, tt.ctyp)
			contractAssertResponse(t, rec, tt.wantStatus, tt.wantBody)
			if repo.writes != tt.wantWrites {
				t.Errorf("repository writes = %d, want %d", repo.writes, tt.wantWrites)
			}
		})
	}

	repo := &catalogRepo{}
	rec := serveCatalog(NewSupplierHandler(usecase.NewSupplierUseCase(repo.suppl())).Delete,
		http.MethodDelete, "/api/v1/suppliers/{id}", "/api/v1/suppliers/1", "", "")
	if rec.Code != http.StatusNoContent || rec.Body.Len() != 0 {
		t.Errorf("delete: got %d %q, want 204 with no body", rec.Code, rec.Body.String())
	}
}

// Behind the router's admin group: no user is 401, a user who is not an admin
// is 403, and the handler is not reached in either case.
func TestCatalogHandlers_AdminOnly(t *testing.T) {
	repo := &catalogRepo{}
	create := NewWoodTypeHandler(usecase.NewWoodTypeUseCase(repo.wood())).Create
	protected := AuthMiddleware(denyAllAuthenticator{})(RequireAdmin(http.HandlerFunc(create)))

	rec := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodPost, "/api/v1/wood-types", strings.NewReader(`{"name":"x"}`))
	req.Header.Set("Content-Type", "application/json")
	protected.ServeHTTP(rec, req)
	contractAssertResponse(t, rec, http.StatusUnauthorized,
		`{"success":false,"code":"UNAUTHENTICATED","message":"authentication required","data":null}`)

	rec = httptest.NewRecorder()
	req = withTestUser(httptest.NewRequest(http.MethodPost, "/api/v1/wood-types", strings.NewReader(`{"name":"x"}`)))
	req.Header.Set("Content-Type", "application/json")
	RequireAdmin(http.HandlerFunc(create)).ServeHTTP(rec, req)
	if rec.Code != http.StatusForbidden || !strings.Contains(rec.Body.String(), `"code":"FORBIDDEN"`) {
		t.Errorf("user: got %d %s, want 403 FORBIDDEN", rec.Code, rec.Body.String())
	}
	if repo.writes != 0 {
		t.Errorf("the handler was reached %d time(s)", repo.writes)
	}
}

// denyAllAuthenticator rejects every token.
type denyAllAuthenticator struct{}

func (denyAllAuthenticator) Authenticate(context.Context, string) (*domain.User, error) {
	return nil, repository.ErrUserNotFound
}
