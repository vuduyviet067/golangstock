package app

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"gostock/internal/domain"
	"gostock/internal/dto"
	productHTTP "gostock/internal/handle/http"
	"gostock/internal/repository/postgres"
	"gostock/internal/testdb"
	"gostock/internal/usecase"
)

// TestCatalogAPI_PostgreSQL drives /api/v1/wood-types and /api/v1/suppliers
// through the real router, handlers, use cases and repositories on the
// integration test database: an admin creates, lists, edits and deletes; a
// user gets 403 and a request without a token 401. Only JWT validation is
// replaced (tokenAuthenticator).
func TestCatalogAPI_PostgreSQL(t *testing.T) {
	db := testdb.Open(t)
	ctx, cancel := context.WithTimeout(context.Background(), 20*time.Second)
	defer cancel()

	tag := fmt.Sprintf("catalog-api-%d", time.Now().UnixNano())
	// Registered before anything is written: catalog rows, then the users.
	t.Cleanup(func() {
		for _, table := range []string{"wood_types", "suppliers"} {
			if _, err := db.Exec(`DELETE FROM `+table+` WHERE name LIKE $1`, tag+"%"); err != nil {
				t.Errorf("cleanup %s: %v", table, err)
			}
		}
		if _, err := db.Exec(`DELETE FROM users WHERE email LIKE $1`, tag+"-%"); err != nil {
			t.Errorf("cleanup users: %v", err)
		}
	})
	createUser := func(role string) *domain.User {
		var id int64
		email := tag + "-" + role + "@example.test"
		if err := db.QueryRowContext(ctx,
			`INSERT INTO users (email, password_hash, role) VALUES ($1, 'x', $2) RETURNING id`, email, role,
		).Scan(&id); err != nil {
			t.Fatalf("create %s: %v", role, err)
		}
		return &domain.User{ID: id, Email: email, Role: role}
	}
	admin := createUser(usecase.RoleAdmin)
	user := createUser(usecase.RoleUser)

	userRepo := postgres.NewUserRepository(db)
	authUC := usecase.NewAuthUseCase(userRepo, postgres.NewRefreshTokenRepository(db), &fakeTokenManager{},
		15*time.Minute, 7*24*time.Hour, 15*time.Second)
	router := NewRouter(db,
		productHTTP.NewProductHandler(usecase.NewProductUseCase(postgres.NewProductRepository(db))),
		productHTTP.NewAuthHandler(authUC, nil),
		tokenAuthenticator{"admin": admin, "user": user},
		productHTTP.NewUserHandler(usecase.NewUserUseCase(userRepo)),
		nil, nil,
		productHTTP.NewWoodTypeHandler(usecase.NewWoodTypeUseCase(postgres.NewWoodTypeRepository(db))),
		productHTTP.NewSupplierHandler(usecase.NewSupplierUseCase(postgres.NewSupplierRepository(db))),
	)

	call := func(method, path, token, body string) *httptest.ResponseRecorder {
		req := httptest.NewRequest(method, path, strings.NewReader(body))
		if body != "" {
			req.Header.Set("Content-Type", "application/json")
		}
		if token != "" {
			req.Header.Set("Authorization", "Bearer "+token)
		}
		rec := httptest.NewRecorder()
		router.ServeHTTP(rec, req)
		return rec
	}
	expect := func(rec *httptest.ResponseRecorder, status int, step string) {
		t.Helper()
		if rec.Code != status {
			t.Fatalf("%s: status = %d, want %d; body = %s", step, rec.Code, status, rec.Body.String())
		}
	}
	expectCode := func(rec *httptest.ResponseRecorder, status int, code, step string) {
		t.Helper()
		expect(rec, status, step)
		if !strings.Contains(rec.Body.String(), `"code":"`+code+`"`) {
			t.Fatalf("%s: body = %s, want code %s", step, rec.Body.String(), code)
		}
	}
	decode := func(rec *httptest.ResponseRecorder, data any) {
		t.Helper()
		var env struct {
			Data json.RawMessage `json:"data"`
		}
		if err := json.Unmarshal(rec.Body.Bytes(), &env); err != nil {
			t.Fatalf("decode %s: %v", rec.Body.String(), err)
		}
		if err := json.Unmarshal(env.Data, data); err != nil {
			t.Fatalf("decode data %s: %v", env.Data, err)
		}
	}

	// Only an admin may use either catalog.
	for _, path := range []string{"/api/v1/wood-types", "/api/v1/suppliers"} {
		expectCode(call(http.MethodGet, path, "", ""), http.StatusUnauthorized, "UNAUTHENTICATED", "GET "+path+" without a token")
		expectCode(call(http.MethodGet, path, "user", ""), http.StatusForbidden, "FORBIDDEN", "GET "+path+" as user")
		expectCode(call(http.MethodPost, path, "user", `{"name":"`+tag+` x"}`), http.StatusForbidden, "FORBIDDEN", "POST "+path+" as user")
		expectCode(call(http.MethodDelete, path+"/1", "user", ""), http.StatusForbidden, "FORBIDDEN", "DELETE "+path+" as user")
	}

	t.Run("wood types", func(t *testing.T) {
		var first, second dto.WoodTypeResponse
		rec := call(http.MethodPost, "/api/v1/wood-types", "admin", `{"name":"  `+tag+` Gỗ lim  "}`)
		expect(rec, http.StatusCreated, "create")
		decode(rec, &first)
		if first.Name != tag+" Gỗ lim" || first.ID == 0 {
			t.Fatalf("created %+v, want the trimmed name and an ID", first)
		}
		// The same name again is allowed.
		rec = call(http.MethodPost, "/api/v1/wood-types", "admin", `{"name":"`+tag+` Gỗ lim"}`)
		expect(rec, http.StatusCreated, "create a duplicate name")
		decode(rec, &second)

		var list []dto.WoodTypeResponse
		rec = call(http.MethodGet, "/api/v1/wood-types", "admin", "")
		expect(rec, http.StatusOK, "list")
		decode(rec, &list)
		found := 0
		for _, w := range list {
			if w.ID == first.ID || w.ID == second.ID {
				found++
			}
		}
		if found != 2 {
			t.Errorf("list contains %d of the 2 wood types", found)
		}

		var renamed dto.WoodTypeResponse
		rec = call(http.MethodPatch, fmt.Sprintf("/api/v1/wood-types/%d", second.ID), "admin", `{"name":"`+tag+` Gỗ sến"}`)
		expect(rec, http.StatusOK, "rename")
		decode(rec, &renamed)
		if renamed.Name != tag+" Gỗ sến" || !renamed.CreatedAt.Equal(second.CreatedAt) || renamed.UpdatedAt.Before(second.UpdatedAt) {
			t.Errorf("renamed %+v from %+v", renamed, second)
		}

		expectCode(call(http.MethodPost, "/api/v1/wood-types", "admin", `{"name":"   "}`),
			http.StatusBadRequest, "INVALID_WOOD_TYPE", "blank name")
		expectCode(call(http.MethodPatch, fmt.Sprintf("/api/v1/wood-types/%d", second.ID), "admin", `{}`),
			http.StatusBadRequest, "EMPTY_UPDATE", "empty update")

		expect(call(http.MethodDelete, fmt.Sprintf("/api/v1/wood-types/%d", second.ID), "admin", ""), http.StatusNoContent, "delete")
		expectCode(call(http.MethodDelete, fmt.Sprintf("/api/v1/wood-types/%d", second.ID), "admin", ""),
			http.StatusNotFound, "WOOD_TYPE_NOT_FOUND", "delete again")
		expectCode(call(http.MethodPatch, fmt.Sprintf("/api/v1/wood-types/%d", second.ID), "admin", `{"name":"x"}`),
			http.StatusNotFound, "WOOD_TYPE_NOT_FOUND", "rename deleted")
		expectCode(call(http.MethodPatch, "/api/v1/wood-types/abc", "admin", `{"name":"x"}`),
			http.StatusBadRequest, "INVALID_WOOD_TYPE_ID", "invalid ID")
	})

	t.Run("suppliers", func(t *testing.T) {
		var created dto.SupplierResponse
		rec := call(http.MethodPost, "/api/v1/suppliers", "admin", `{"name":"`+tag+` An Phát","phone":"+84 368.122.137"}`)
		expect(rec, http.StatusCreated, "create")
		decode(rec, &created)
		if created.Phone == nil || *created.Phone != "0368122137" {
			t.Fatalf("created %+v, want the normalized phone 0368122137", created)
		}
		path := fmt.Sprintf("/api/v1/suppliers/%d", created.ID)

		expectCode(call(http.MethodPost, "/api/v1/suppliers", "admin", `{"name":"`+tag+` Bad","phone":"12345"}`),
			http.StatusBadRequest, "INVALID_PHONE", "invalid phone")

		var updated dto.SupplierResponse
		rec = call(http.MethodPatch, path, "admin", `{"name":"`+tag+` Bình Minh"}`)
		expect(rec, http.StatusOK, "rename")
		decode(rec, &updated)
		if updated.Phone == nil || *updated.Phone != "0368122137" {
			t.Errorf("rename changed the phone: %+v", updated)
		}

		rec = call(http.MethodPatch, path, "admin", `{"phone":""}`)
		expect(rec, http.StatusOK, "remove the phone")
		if !strings.Contains(rec.Body.String(), `"phone":null`) {
			t.Errorf("remove the phone: body = %s", rec.Body.String())
		}
		var stored *string
		if err := db.QueryRowContext(ctx, `SELECT phone FROM suppliers WHERE id = $1`, created.ID).Scan(&stored); err != nil || stored != nil {
			t.Errorf("stored phone = %v, %v; want NULL", stored, err)
		}

		var list []dto.SupplierResponse
		rec = call(http.MethodGet, "/api/v1/suppliers", "admin", "")
		expect(rec, http.StatusOK, "list")
		decode(rec, &list)
		listed := false
		for _, s := range list {
			if s.ID == created.ID {
				listed = s.Name == tag+" Bình Minh" && s.Phone == nil
			}
		}
		if !listed {
			t.Errorf("list does not show the updated supplier %d", created.ID)
		}

		expect(call(http.MethodDelete, path, "admin", ""), http.StatusNoContent, "delete")
		expectCode(call(http.MethodPatch, path, "admin", `{"name":"x"}`), http.StatusNotFound, "SUPPLIER_NOT_FOUND", "rename deleted")
	})
}
