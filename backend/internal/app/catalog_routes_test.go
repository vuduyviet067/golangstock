package app

import (
	"context"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"gostock/internal/domain"
	"gostock/internal/repository"
)

// fakeWoodTypeRepo and fakeSupplierRepo hold no state: ID 1 exists, every
// other ID does not, and a created row gets ID 100. The route tests only
// check routing, authentication and authorization.
type fakeWoodTypeRepo struct{}

var catalogFakeTime = time.Date(2026, 11, 20, 8, 0, 0, 0, time.UTC)

func (f *fakeWoodTypeRepo) List(ctx context.Context) ([]domain.WoodType, error) {
	return []domain.WoodType{{ID: 1, Name: "Gỗ lim", CreatedAt: catalogFakeTime, UpdatedAt: catalogFakeTime}}, nil
}

func (f *fakeWoodTypeRepo) GetByID(ctx context.Context, id int64) (*domain.WoodType, error) {
	if id != 1 {
		return nil, repository.ErrWoodTypeNotFound
	}
	return &domain.WoodType{ID: 1, Name: "Gỗ lim", CreatedAt: catalogFakeTime, UpdatedAt: catalogFakeTime}, nil
}

func (f *fakeWoodTypeRepo) Create(ctx context.Context, w *domain.WoodType) error {
	w.ID, w.CreatedAt, w.UpdatedAt = 100, catalogFakeTime, catalogFakeTime
	return nil
}

func (f *fakeWoodTypeRepo) Update(ctx context.Context, w *domain.WoodType) error {
	if w.ID != 1 {
		return repository.ErrWoodTypeNotFound
	}
	w.CreatedAt, w.UpdatedAt = catalogFakeTime, catalogFakeTime
	return nil
}

func (f *fakeWoodTypeRepo) Delete(ctx context.Context, id int64) error {
	if id != 1 {
		return repository.ErrWoodTypeNotFound
	}
	return nil
}

type fakeSupplierRepo struct{}

func (f *fakeSupplierRepo) List(ctx context.Context) ([]domain.Supplier, error) {
	return []domain.Supplier{{ID: 1, Name: "Xưởng An Phát", CreatedAt: catalogFakeTime, UpdatedAt: catalogFakeTime}}, nil
}

func (f *fakeSupplierRepo) GetByID(ctx context.Context, id int64) (*domain.Supplier, error) {
	if id != 1 {
		return nil, repository.ErrSupplierNotFound
	}
	return &domain.Supplier{ID: 1, Name: "Xưởng An Phát", CreatedAt: catalogFakeTime, UpdatedAt: catalogFakeTime}, nil
}

func (f *fakeSupplierRepo) Create(ctx context.Context, s *domain.Supplier) error {
	s.ID, s.CreatedAt, s.UpdatedAt = 100, catalogFakeTime, catalogFakeTime
	return nil
}

func (f *fakeSupplierRepo) Update(ctx context.Context, s *domain.Supplier) error {
	if s.ID != 1 {
		return repository.ErrSupplierNotFound
	}
	return nil
}

func (f *fakeSupplierRepo) Delete(ctx context.Context, id int64) error {
	if id != 1 {
		return repository.ErrSupplierNotFound
	}
	return nil
}

// Wood types and suppliers are admin only: no token is 401, a user token is
// 403 for every method, an admin reaches the handlers.
func TestCatalogRoutes_AuthorizationMatrix(t *testing.T) {
	router := setupTestRouter()

	type call struct{ method, path, body string }
	calls := []call{
		{http.MethodGet, "/api/v1/wood-types", ""},
		{http.MethodPost, "/api/v1/wood-types", `{"name":"Gỗ lim"}`},
		{http.MethodPatch, "/api/v1/wood-types/1", `{"name":"Gỗ sến"}`},
		{http.MethodDelete, "/api/v1/wood-types/1", ""},
		{http.MethodGet, "/api/v1/suppliers", ""},
		{http.MethodPost, "/api/v1/suppliers", `{"name":"An Phát","phone":"0368122137"}`},
		{http.MethodPatch, "/api/v1/suppliers/1", `{"phone":""}`},
		{http.MethodDelete, "/api/v1/suppliers/1", ""},
	}
	adminStatus := map[string]int{
		http.MethodGet: http.StatusOK, http.MethodPost: http.StatusCreated,
		http.MethodPatch: http.StatusOK, http.MethodDelete: http.StatusNoContent,
	}

	for _, c := range calls {
		for _, tc := range []struct {
			token      string
			wantStatus int
			wantCode   string
		}{
			{"", http.StatusUnauthorized, `"code":"UNAUTHENTICATED"`},
			{"user-token", http.StatusForbidden, `"code":"FORBIDDEN"`},
			{"admin-token", adminStatus[c.method], ""},
		} {
			t.Run(c.method+" "+c.path+" token="+tc.token, func(t *testing.T) {
				req := httptest.NewRequest(c.method, c.path, strings.NewReader(c.body))
				if c.body != "" {
					req.Header.Set("Content-Type", "application/json")
				}
				if tc.token != "" {
					req.Header.Set("Authorization", "Bearer "+tc.token)
				}
				rec := httptest.NewRecorder()
				router.ServeHTTP(rec, req)
				if rec.Code != tc.wantStatus {
					t.Fatalf("status = %d, want %d; body = %s", rec.Code, tc.wantStatus, rec.Body.String())
				}
				if tc.wantCode != "" && !strings.Contains(rec.Body.String(), tc.wantCode) {
					t.Errorf("body = %s, want %s", rec.Body.String(), tc.wantCode)
				}
			})
		}
	}
}
