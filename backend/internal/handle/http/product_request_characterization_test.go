package http

import (
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"gostock/internal/domain"
)

// Characterization of the product request path (Step 5.2), before the
// Request DTO -> Input DTO -> Mapper -> Domain refactor. These tests pin what
// the repository receives for a Create or Update request (after the handler
// and the use case), what the response reports, and which requests are
// rejected before the repository is reached. They describe the current
// behavior; they are not a target design.
//
// Already covered elsewhere and not repeated here: minimum_stock values on
// Create (TestCreateMinimumStock) and Update (TestUpdateMinimumStock), the
// quantity rejection on Update (TestUpdateRejectsQuantity), the path ID over
// the body id on Update (TestUpdateReportsStoredQuantity), and decode errors
// before authentication on Create (TestStockChangesKeepInputErrorsBeforeAuth).

func TestCreateRequest_RepositoryReceivesNormalizedProduct(t *testing.T) {
	repo := &fakeProductRepository{createID: 123}
	req := httptest.NewRequest(http.MethodPost, "/api/v1/products", strings.NewReader(
		`{"id":99,"sku":"  SKU010  ","name":" Mouse ","price":25.5,"quantity":3,"minimum_stock":2}`))
	req.Header.Set("Content-Type", "application/json")
	rec := httptest.NewRecorder()

	newTestHandler(repo).Create(rec, withTestUser(req))

	// The body id is ignored (the repository assigns it), SKU and name are
	// trimmed, the other fields are passed as sent.
	want := domain.Product{ID: 0, SKU: "SKU010", Name: "Mouse", Price: 25.5, Quantity: 3, MinimumStock: 2}
	if repo.createdProduct == nil {
		t.Fatal("repository Create was not called")
	}
	if *repo.createdProduct != want {
		t.Errorf("repository received %+v, want %+v", *repo.createdProduct, want)
	}
	if repo.actorID != testActorID {
		t.Errorf("repository received actor %d, want %d (the authenticated user)", repo.actorID, testActorID)
	}
	// The response carries the ID the repository assigned.
	contractAssertResponse(t, rec, http.StatusCreated,
		`{"success":true,"message":"Product created successfully","data":{"id":123,"sku":"SKU010","name":"Mouse","price":25.5,"quantity":3,"minimum_stock":2}}`)
}

func TestUpdateRequest_RepositoryReceivesNormalizedProduct(t *testing.T) {
	repo := &fakeProductRepository{storedQuantity: 9, storedMinimumStock: 1}
	req := httptest.NewRequest(http.MethodPut, "/api/v1/products/7", strings.NewReader(
		`{"id":99,"sku":" SKU007 ","name":" Keyboard Pro ","price":120.5,"minimum_stock":3}`))
	req.Header.Set("Content-Type", "application/json")
	rec := httptest.NewRecorder()

	newTestHandler(repo).Update(rec, req)

	// The ID comes from the path, SKU and name are trimmed, quantity is never
	// sent (0), and minimum_stock travels as the separate argument while the
	// product's own MinimumStock field is 0.
	want := domain.Product{ID: 7, SKU: "SKU007", Name: "Keyboard Pro", Price: 120.5, Quantity: 0, MinimumStock: 0}
	if repo.updatedProduct == nil {
		t.Fatal("repository Update was not called")
	}
	if *repo.updatedProduct != want {
		t.Errorf("repository received %+v, want %+v", *repo.updatedProduct, want)
	}
	if repo.updateMinimumStock == nil || *repo.updateMinimumStock != 3 {
		t.Errorf("repository received minimumStock %v, want a pointer to 3", repo.updateMinimumStock)
	}
	// The response reports the stored quantity and the threshold the
	// repository returns, not request values.
	contractAssertResponse(t, rec, http.StatusOK,
		`{"success":true,"message":"Product updated successfully","data":{"id":7,"sku":"SKU007","name":"Keyboard Pro","price":120.5,"quantity":9,"minimum_stock":3}}`)
}

func TestCreateRequest_RejectedBeforeRepository(t *testing.T) {
	const valid = `{"sku":"SKU001","name":"Keyboard","price":100,"quantity":5}`
	tests := []struct {
		name        string
		method      string
		contentType string
		body        string
		withUser    bool
		wantStatus  int
		wantMessage string
	}{
		{"invalid JSON", http.MethodPost, "application/json", `{"sku":`, true, http.StatusBadRequest, "Invalid JSON request"},
		{"unknown field", http.MethodPost, "application/json", `{"sku":"SKU001","name":"Keyboard","colour":"red"}`, true, http.StatusBadRequest, "Invalid JSON request"},
		{"trailing JSON value", http.MethodPost, "application/json", valid + `{}`, true, http.StatusBadRequest, "Request must contain one JSON object"},
		{"missing content type", http.MethodPost, "", valid, true, http.StatusUnsupportedMediaType, "Content-Type must be application/json"},
		{"wrong content type", http.MethodPost, "text/plain", valid, true, http.StatusUnsupportedMediaType, "Content-Type must be application/json"},
		{"wrong method", http.MethodPut, "application/json", valid, true, http.StatusMethodNotAllowed, "method not allowed"},
		{"no authenticated user", http.MethodPost, "application/json", valid, false, http.StatusUnauthorized, "authentication required"},
		// The handler checks the user before the use case validates the
		// product, so an invalid product without a user is a 401.
		{"invalid product and no authenticated user", http.MethodPost, "application/json", `{"sku":"  ","name":"Keyboard","price":100}`, false, http.StatusUnauthorized, "authentication required"},
		{"blank SKU after trimming", http.MethodPost, "application/json", `{"sku":"   ","name":"Keyboard","price":100}`, true, http.StatusBadRequest, "Invalid product"},
		{"negative quantity", http.MethodPost, "application/json", `{"sku":"SKU001","name":"Keyboard","price":100,"quantity":-1}`, true, http.StatusBadRequest, "Invalid product"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			repo := &fakeProductRepository{createID: 1}
			req := httptest.NewRequest(tt.method, "/api/v1/products", strings.NewReader(tt.body))
			if tt.contentType != "" {
				req.Header.Set("Content-Type", tt.contentType)
			}
			if tt.withUser {
				req = withTestUser(req)
			}
			rec := httptest.NewRecorder()

			newTestHandler(repo).Create(rec, req)

			contractAssertResponse(t, rec, tt.wantStatus, `{"success":false,"message":"`+tt.wantMessage+`","data":null}`)
			if repo.stockCalls != 0 || repo.createdProduct != nil {
				t.Errorf("repository Create was called (%d call(s)) for a rejected request", repo.stockCalls)
			}
		})
	}
}

func TestUpdateRequest_RejectedBeforeRepository(t *testing.T) {
	const valid = `{"sku":"SKU001","name":"Keyboard","price":100}`
	tests := []struct {
		name        string
		path        string
		contentType string
		body        string
		wantStatus  int
		wantMessage string
	}{
		// The path ID is checked before the Content-Type.
		{"invalid ID and no content type", "/api/v1/products/abc", "", valid, http.StatusBadRequest, "invalid product ID"},
		{"zero ID and wrong content type", "/api/v1/products/0", "text/plain", valid, http.StatusBadRequest, "invalid product ID"},
		{"missing content type", "/api/v1/products/1", "", valid, http.StatusUnsupportedMediaType, "Content-Type must be application/json"},
		{"invalid JSON", "/api/v1/products/1", "application/json", `{"sku":`, http.StatusBadRequest, "Invalid JSON request"},
		{"unknown field", "/api/v1/products/1", "application/json", `{"sku":"SKU001","colour":"red"}`, http.StatusBadRequest, "Invalid JSON request"},
		{"trailing JSON value", "/api/v1/products/1", "application/json", valid + `{}`, http.StatusBadRequest, "Request must contain one JSON object"},
		{"blank name after trimming", "/api/v1/products/1", "application/json", `{"sku":"SKU001","name":"   ","price":100}`, http.StatusBadRequest, "Invalid product"},
		{"negative price", "/api/v1/products/1", "application/json", `{"sku":"SKU001","name":"Keyboard","price":-0.5}`, http.StatusBadRequest, "Invalid product"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			repo := &fakeProductRepository{storedQuantity: 4}
			req := httptest.NewRequest(http.MethodPut, tt.path, strings.NewReader(tt.body))
			if tt.contentType != "" {
				req.Header.Set("Content-Type", tt.contentType)
			}
			rec := httptest.NewRecorder()

			newTestHandler(repo).Update(rec, req)

			contractAssertResponse(t, rec, tt.wantStatus, `{"success":false,"message":"`+tt.wantMessage+`","data":null}`)
			if repo.updatedProduct != nil || repo.updateMinimumStock != nil {
				t.Errorf("repository Update was called for a rejected request: %+v", repo.updatedProduct)
			}
		})
	}
}
