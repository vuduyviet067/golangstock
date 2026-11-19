package http

import (
	"encoding/json"
	"io"
	"net/http"
	"strconv"
	"strings"

	"gostock/internal/dto"
	"gostock/internal/usecase"
	"gostock/pkg/response"
)

type ProductHandler struct {
	useCase *usecase.ProductUseCase
}

func NewProductHandler(uc *usecase.ProductUseCase) *ProductHandler {
	return &ProductHandler{
		useCase: uc,
	}
}

// createProductInput adapts a create request to the use case input. It copies
// the fields as sent: the use case trims, validates and applies the default
// minimum stock. The request ID is accepted for backward compatibility but
// dropped: the database assigns it.
func createProductInput(req dto.CreateProductRequest) dto.CreateProductInput {
	return dto.CreateProductInput{
		SKU:          req.SKU,
		Name:         req.Name,
		Price:        req.Price,
		Quantity:     req.Quantity,
		MinimumStock: req.MinimumStock,
	}
}

// updateProductInput adapts an update request to the use case input. The ID
// is the one parsed from the URL path; the request body's id is accepted for
// backward compatibility but ignored. The other fields are copied as sent: the
// use case trims and validates them, and a nil minimum stock stays nil (keep
// the stored threshold). The request's quantity has no counterpart: Update
// rejects it before calling this.
func updateProductInput(id int64, req dto.UpdateProductRequest) dto.UpdateProductInput {
	return dto.UpdateProductInput{
		ID:           id,
		SKU:          req.SKU,
		Name:         req.Name,
		Price:        req.Price,
		MinimumStock: req.MinimumStock,
	}
}

// currentActorID returns the ID of the user set by AuthMiddleware. Stock
// changes are recorded against this user. If no user is present it writes
// 401 and returns false; behind AuthMiddleware this does not happen.
func currentActorID(w http.ResponseWriter, r *http.Request) (int64, bool) {
	user, ok := CurrentUser(r.Context())
	if !ok {
		response.Error(w, http.StatusUnauthorized, response.CodeUnauthenticated, "authentication required")
		return 0, false
	}
	return user.ID, true
}

func (h *ProductHandler) GetByID(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		w.Header().Set("Allow", http.MethodGet)
		response.Error(w, http.StatusMethodNotAllowed, response.CodeMethodNotAllowed, "method not allowed")
		return
	}

	rawID := strings.TrimPrefix(r.URL.Path, "/api/v1/products/")
	if rawID == "" || strings.Contains(rawID, "/") {
		response.Error(w, http.StatusBadRequest, codeInvalidProductID, "invalid product ID")
		return
	}

	id, err := strconv.ParseInt(rawID, 10, 64)
	if err != nil {
		response.Error(w, http.StatusBadRequest, codeInvalidProductID, "invalid product ID")
		return
	}

	product, err := h.useCase.GetByID(r.Context(), id)
	if err != nil {
		WriteError(w, r, err)
		return
	}

	response.Success(w, http.StatusOK, "Product retrieved successfully", product)
}
func (h *ProductHandler) List(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		w.Header().Set("Allow", http.MethodGet)
		response.Error(w, http.StatusMethodNotAllowed, response.CodeMethodNotAllowed, "method not allowed")
		return
	}

	query := r.URL.Query()
	page := 1
	limit := 10

	if rawPage := query.Get("page"); rawPage != "" {
		parsed, err := strconv.Atoi(rawPage)
		if err != nil {
			response.Error(w, http.StatusBadRequest, codeInvalidPage, "invalid page")
			return
		}
		page = parsed
	}

	if rawLimit := query.Get("limit"); rawLimit != "" {
		parsed, err := strconv.Atoi(rawLimit)
		if err != nil {
			response.Error(w, http.StatusBadRequest, codeInvalidLimit, "invalid limit")
			return
		}
		limit = parsed
	}

	products, _, err := h.useCase.List(
		r.Context(),
		query.Get("q"),
		page,
		limit,
		query.Get("sortBy"),
		query.Get("order"),
	)
	if err != nil {
		WriteError(w, r, err)
		return
	}

	response.Success(w, http.StatusOK, "Products retrieved successfully", products)
}
func (h *ProductHandler) Create(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		w.Header().Set("Allow", http.MethodPost)
		response.Error(w, http.StatusMethodNotAllowed, response.CodeMethodNotAllowed, "method not allowed")
		return
	}

	if !EnforceJSONContentType(w, r) {
		return
	}

	r.Body = http.MaxBytesReader(w, r.Body, 1<<20)

	var req dto.CreateProductRequest

	decoder := json.NewDecoder(r.Body)
	decoder.DisallowUnknownFields()

	if err := decoder.Decode(&req); err != nil {
		response.Error(w, http.StatusBadRequest, response.CodeValidationError, "Invalid JSON request")
		return
	}

	var extra any
	if err := decoder.Decode(&extra); err != io.EOF {
		response.Error(w, http.StatusBadRequest, response.CodeValidationError, "Request must contain one JSON object")
		return
	}

	actorID, ok := currentActorID(w, r)
	if !ok {
		return
	}

	created, err := h.useCase.Create(r.Context(), createProductInput(req), actorID)
	if err != nil {
		WriteError(w, r, err)
		return
	}

	response.Success(w, http.StatusCreated, "Product created successfully", created)
}

func (h *ProductHandler) Update(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPut {
		w.Header().Set("Allow", http.MethodPut)
		response.Error(w, http.StatusMethodNotAllowed, response.CodeMethodNotAllowed, "method not allowed")
		return
	}

	rawID := strings.TrimPrefix(r.URL.Path, "/api/v1/products/")
	if rawID == "" || strings.Contains(rawID, "/") {
		response.Error(w, http.StatusBadRequest, codeInvalidProductID, "invalid product ID")
		return
	}

	id, err := strconv.ParseInt(rawID, 10, 64)
	if err != nil || id <= 0 {
		response.Error(w, http.StatusBadRequest, codeInvalidProductID, "invalid product ID")
		return
	}

	if !EnforceJSONContentType(w, r) {
		return
	}

	r.Body = http.MaxBytesReader(w, r.Body, 1<<20)

	var req dto.UpdateProductRequest

	decoder := json.NewDecoder(r.Body)
	decoder.DisallowUnknownFields()

	if err := decoder.Decode(&req); err != nil {
		response.Error(w, http.StatusBadRequest, response.CodeValidationError, "Invalid JSON request")
		return
	}

	var extra any
	if err := decoder.Decode(&extra); err != io.EOF {
		response.Error(w, http.StatusBadRequest, response.CodeValidationError, "Request must contain one JSON object")
		return
	}

	// Stock may change only through stock-in/stock-out. Reject the field
	// whatever its value (including 0 and null) so a stale form value can
	// never overwrite the current quantity.
	if req.Quantity != nil {
		response.Error(w, http.StatusBadRequest, response.CodeValidationError, "quantity cannot be updated; use stock-in or stock-out")
		return
	}

	// An omitted or null minimum_stock keeps the stored threshold.
	updated, err := h.useCase.Update(r.Context(), updateProductInput(id, req))
	if err != nil {
		WriteError(w, r, err)
		return
	}

	response.Success(w, http.StatusOK, "Product updated successfully", updated)
}

func (h *ProductHandler) Delete(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodDelete {
		w.Header().Set("Allow", http.MethodDelete)
		response.Error(w, http.StatusMethodNotAllowed, response.CodeMethodNotAllowed, "method not allowed")
		return
	}

	rawID := strings.TrimPrefix(r.URL.Path, "/api/v1/products/")
	if rawID == "" || strings.Contains(rawID, "/") {
		response.Error(w, http.StatusBadRequest, codeInvalidProductID, "invalid product ID")
		return
	}

	id, err := strconv.ParseInt(rawID, 10, 64)
	if err != nil || id <= 0 {
		response.Error(w, http.StatusBadRequest, codeInvalidProductID, "invalid product ID")
		return
	}

	if err := h.useCase.Delete(r.Context(), id); err != nil {
		WriteError(w, r, err)
		return
	}

	response.NoContent(w)
}

func (h *ProductHandler) StockIn(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		w.Header().Set("Allow", http.MethodPost)
		response.Error(w, http.StatusMethodNotAllowed, response.CodeMethodNotAllowed, "method not allowed")
		return
	}

	rawID := strings.TrimPrefix(r.URL.Path, "/api/v1/products/")
	rawID = strings.TrimSuffix(rawID, "/stock-in")

	if rawID == "" || strings.Contains(rawID, "/") {
		response.Error(w, http.StatusBadRequest, codeInvalidProductID, "invalid product ID")
		return
	}

	id, err := strconv.ParseInt(rawID, 10, 64)
	if err != nil || id <= 0 {
		response.Error(w, http.StatusBadRequest, codeInvalidProductID, "invalid product ID")
		return
	}

	if !EnforceJSONContentType(w, r) {
		return
	}

	r.Body = http.MaxBytesReader(w, r.Body, 1<<20)

	var req dto.StockQuantityRequest

	decoder := json.NewDecoder(r.Body)
	decoder.DisallowUnknownFields()

	if err := decoder.Decode(&req); err != nil {
		response.Error(w, http.StatusBadRequest, response.CodeValidationError, "Invalid JSON body")
		return
	}

	var extra any
	if err := decoder.Decode(&extra); err != io.EOF {
		response.Error(w, http.StatusBadRequest, response.CodeValidationError, "Request must contain one JSON object")
		return
	}

	actorID, ok := currentActorID(w, r)
	if !ok {
		return
	}

	product, err := h.useCase.StockIn(r.Context(), id, req.Quantity, actorID)
	if err != nil {
		WriteError(w, r, err)
		return
	}

	response.Success(w, http.StatusOK, "Stock added successfully", product)
}

func (h *ProductHandler) StockOut(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		w.Header().Set("Allow", http.MethodPost)
		response.Error(w, http.StatusMethodNotAllowed, response.CodeMethodNotAllowed, "method not allowed")
		return
	}

	rawID := strings.TrimPrefix(r.URL.Path, "/api/v1/products/")
	rawID = strings.TrimSuffix(rawID, "/stock-out")

	if rawID == "" || strings.Contains(rawID, "/") {
		response.Error(w, http.StatusBadRequest, codeInvalidProductID, "invalid product ID")
		return
	}

	id, err := strconv.ParseInt(rawID, 10, 64)
	if err != nil || id <= 0 {
		response.Error(w, http.StatusBadRequest, codeInvalidProductID, "invalid product ID")
		return
	}

	if !EnforceJSONContentType(w, r) {
		return
	}

	r.Body = http.MaxBytesReader(w, r.Body, 1<<20)

	var req dto.StockQuantityRequest

	decoder := json.NewDecoder(r.Body)
	decoder.DisallowUnknownFields()

	if err := decoder.Decode(&req); err != nil {
		response.Error(w, http.StatusBadRequest, response.CodeValidationError, "Invalid JSON body")
		return
	}

	var extra any
	if err := decoder.Decode(&extra); err != io.EOF {
		response.Error(w, http.StatusBadRequest, response.CodeValidationError, "Request must contain one JSON object")
		return
	}

	actorID, ok := currentActorID(w, r)
	if !ok {
		return
	}

	product, err := h.useCase.StockOut(r.Context(), id, req.Quantity, actorID)
	if err != nil {
		WriteError(w, r, err)
		return
	}

	response.Success(w, http.StatusOK, "Stock reduced successfully", product)
}
