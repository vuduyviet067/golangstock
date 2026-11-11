package app

import (
	"context"
	"encoding/json"
	"errors"
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

// tokenAuthenticator maps fixed tokens to real users of the test database, so
// the integration test runs the real router, middleware, handlers, use cases
// and repositories; only JWT validation is replaced.
type tokenAuthenticator map[string]*domain.User

func (a tokenAuthenticator) Authenticate(ctx context.Context, token string) (*domain.User, error) {
	if user, ok := a[token]; ok {
		return user, nil
	}
	return nil, errors.New("invalid token")
}

// historyPage is the paginated envelope returned by the history endpoints.
type historyPage struct {
	Success bool   `json:"success"`
	Message string `json:"message"`
	Data    struct {
		Items      []dto.StockMovementResponse `json:"items"`
		Pagination dto.PaginationMeta          `json:"pagination"`
	} `json:"data"`
}

// TestStockMovementHistory_EndToEnd records stock changes through the API,
// reads them back through both history endpoints, deletes the product and
// checks that its history survives in the global history only.
func TestStockMovementHistory_EndToEnd(t *testing.T) {
	db := testdb.Open(t)

	ctx, cancel := context.WithTimeout(context.Background(), 20*time.Second)
	defer cancel()

	suffix := time.Now().UnixNano()
	createUser := func(role string) *domain.User {
		var id int64
		email := fmt.Sprintf("history-%s-%d@example.test", role, suffix)
		if err := db.QueryRowContext(ctx,
			`INSERT INTO users (email, password_hash, role) VALUES ($1, 'x', $2) RETURNING id`, email, role,
		).Scan(&id); err != nil {
			t.Fatalf("create %s: %v", role, err)
		}
		return &domain.User{ID: id, Email: email, Role: role}
	}
	admin := createUser(usecase.RoleAdmin)
	user := createUser(usecase.RoleUser)
	sku := fmt.Sprintf("HISTORY-E2E-%d", suffix)

	// Movements must go before their actors (ON DELETE RESTRICT).
	t.Cleanup(func() {
		_, _ = db.Exec(`DELETE FROM stock_movements WHERE product_sku = $1`, sku)
		_, _ = db.Exec(`DELETE FROM products WHERE sku = $1`, sku)
		_, _ = db.Exec(`DELETE FROM users WHERE id IN ($1, $2)`, admin.ID, user.ID)
	})

	productRepo := postgres.NewProductRepository(db)
	userRepo := postgres.NewUserRepository(db)
	authUC := usecase.NewAuthUseCase(userRepo, postgres.NewRefreshTokenRepository(db), &fakeTokenManager{},
		15*time.Minute, 7*24*time.Hour, 15*time.Second)
	router := NewRouter(
		db,
		productHTTP.NewProductHandler(usecase.NewProductUseCase(productRepo)),
		productHTTP.NewAuthHandler(authUC, nil),
		tokenAuthenticator{"admin": admin, "user": user},
		productHTTP.NewUserHandler(usecase.NewUserUseCase(userRepo)),
		productHTTP.NewStockMovementHandler(
			usecase.NewStockMovementUseCase(productRepo, postgres.NewStockMovementRepository(db)),
		),
		nil,
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
	decode := func(rec *httptest.ResponseRecorder) historyPage {
		t.Helper()
		var page historyPage
		if err := json.Unmarshal(rec.Body.Bytes(), &page); err != nil {
			t.Fatalf("decode %s: %v", rec.Body.String(), err)
		}
		return page
	}

	// 1-3. Create with stock (admin), stock-in and stock-out (user).
	rec := call(http.MethodPost, "/api/v1/products", "admin",
		fmt.Sprintf(`{"sku":%q,"name":"History E2E","price":10,"quantity":5}`, sku))
	expect(rec, http.StatusCreated, "create")
	var created struct {
		Data dto.ProductResponse `json:"data"`
	}
	if err := json.Unmarshal(rec.Body.Bytes(), &created); err != nil {
		t.Fatalf("decode create: %v", err)
	}
	productID := created.Data.ID
	productPath := fmt.Sprintf("/api/v1/products/%d", productID)
	historyPath := productPath + "/movements"

	expect(call(http.MethodPost, productPath+"/stock-in", "user", `{"quantity":3}`), http.StatusOK, "stock-in")
	expect(call(http.MethodPost, productPath+"/stock-out", "user", `{"quantity":2}`), http.StatusOK, "stock-out")

	// Authentication and authorization on the real router.
	expect(call(http.MethodGet, historyPath, "", ""), http.StatusUnauthorized, "anonymous product history")
	expect(call(http.MethodGet, "/api/v1/stock-movements", "", ""), http.StatusUnauthorized, "anonymous global history")
	expect(call(http.MethodGet, "/api/v1/stock-movements", "user", ""), http.StatusForbidden, "user global history")
	expect(call(http.MethodGet, historyPath+"?limit=0", "user", ""), http.StatusBadRequest, "invalid limit")

	// 4-6. Product history, newest first, with types, amounts and actors.
	type wantMovement struct {
		typ                   string
		change, before, after int
		actor                 int64
	}
	want := []wantMovement{
		{"out", -2, 8, 6, user.ID},
		{"in", 3, 5, 8, user.ID},
		{"initial", 5, 0, 5, admin.ID},
	}
	check := func(items []dto.StockMovementResponse, want []wantMovement, wantProductID *int64, step string) {
		t.Helper()
		if len(items) != len(want) {
			t.Fatalf("%s: %d items, want %d: %+v", step, len(items), len(want), items)
		}
		for i, w := range want {
			m := items[i]
			if m.MovementType != w.typ || m.QuantityChange != w.change || m.QuantityBefore != w.before || m.QuantityAfter != w.after {
				t.Errorf("%s item %d = %s %d (%d -> %d), want %s %d (%d -> %d)",
					step, i, m.MovementType, m.QuantityChange, m.QuantityBefore, m.QuantityAfter, w.typ, w.change, w.before, w.after)
			}
			if m.UserID == nil || *m.UserID != w.actor {
				t.Errorf("%s item %d actor = %v, want %d", step, i, m.UserID, w.actor)
			}
			if m.ProductSKU != sku {
				t.Errorf("%s item %d sku = %q, want %q", step, i, m.ProductSKU, sku)
			}
			switch {
			case wantProductID == nil && m.ProductID != nil:
				t.Errorf("%s item %d product_id = %d, want null", step, i, *m.ProductID)
			case wantProductID != nil && (m.ProductID == nil || *m.ProductID != *wantProductID):
				t.Errorf("%s item %d product_id = %v, want %d", step, i, m.ProductID, *wantProductID)
			}
			if i > 0 && items[i-1].ID <= m.ID {
				t.Errorf("%s: not newest first (id %d before %d)", step, items[i-1].ID, m.ID)
			}
		}
	}

	for _, token := range []string{"user", "admin"} {
		rec = call(http.MethodGet, historyPath, token, "")
		expect(rec, http.StatusOK, token+" product history")
		page := decode(rec)
		check(page.Data.Items, want, &productID, token+" product history")
		if p := page.Data.Pagination; p.Page != 1 || p.Limit != 10 || p.Total != 3 || p.TotalPages != 1 {
			t.Errorf("%s product history pagination = %+v, want page 1, limit 10, total 3, 1 page", token, p)
		}
	}
	movementIDs := make([]int64, 0, 3)
	for _, m := range decode(call(http.MethodGet, historyPath, "user", "")).Data.Items {
		movementIDs = append(movementIDs, m.ID)
	}

	// 7. Pagination and total.
	page1 := decode(call(http.MethodGet, historyPath+"?page=1&limit=2", "user", ""))
	check(page1.Data.Items, want[0:2], &productID, "page 1")
	if p := page1.Data.Pagination; p.Total != 3 || p.TotalPages != 2 || p.Limit != 2 {
		t.Errorf("page 1 pagination = %+v, want total 3, 2 pages, limit 2", p)
	}
	page2 := decode(call(http.MethodGet, historyPath+"?page=2&limit=2", "user", ""))
	check(page2.Data.Items, want[2:3], &productID, "page 2")
	page3 := decode(call(http.MethodGet, historyPath+"?page=3&limit=2", "user", ""))
	if len(page3.Data.Items) != 0 || page3.Data.Pagination.Total != 3 {
		t.Errorf("page 3 = %+v, want no items and total 3", page3.Data)
	}

	// 8. Delete the product.
	expect(call(http.MethodDelete, productPath, "admin", ""), http.StatusNoContent, "delete product")

	// 9. The movements remain in the global history with product_id null and
	// the SKU snapshot. Other tests may write concurrently, so collect this
	// product's rows by SKU across pages instead of assuming positions.
	var global []dto.StockMovementResponse
	seen := map[int64]bool{}
	for pageNo := 1; len(global) < 3; pageNo++ {
		rec = call(http.MethodGet, fmt.Sprintf("/api/v1/stock-movements?page=%d&limit=100", pageNo), "admin", "")
		expect(rec, http.StatusOK, "admin global history")
		page := decode(rec)
		for _, m := range page.Data.Items {
			if m.ProductSKU == sku && !seen[m.ID] {
				seen[m.ID] = true
				global = append(global, m)
			}
		}
		if pageNo >= page.Data.Pagination.TotalPages {
			break
		}
	}
	check(global, want, nil, "global history after delete")
	for i, m := range global {
		if m.ID != movementIDs[i] {
			t.Errorf("global history item %d id = %d, want the same movement %d as before the delete", i, m.ID, movementIDs[i])
		}
	}

	// 10. The product endpoint now reports the product as missing.
	rec = call(http.MethodGet, historyPath, "user", "")
	expect(rec, http.StatusNotFound, "product history after delete")
	if !strings.Contains(rec.Body.String(), `"message":"Product not found"`) {
		t.Errorf("body = %s, want Product not found", rec.Body.String())
	}

	// Reading history wrote nothing.
	var rows int
	if err := db.QueryRowContext(ctx, `SELECT COUNT(*) FROM stock_movements WHERE product_sku = $1`, sku).Scan(&rows); err != nil {
		t.Fatalf("count movements: %v", err)
	}
	if rows != 3 {
		t.Errorf("stock_movements has %d rows for %s, want 3", rows, sku)
	}
}
