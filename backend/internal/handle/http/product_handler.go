package http

import (
	"encoding/json"
	"io"
	"net/http"
	"strconv"
	"strings"

	"gostock/internal/domain"
	"gostock/internal/dto"
	"gostock/internal/handle/http/response"
	"gostock/internal/usecase"
)

type ProductHandler struct {
	useCase *usecase.ProductUseCase
}

func NewProductHandler(uc *usecase.ProductUseCase) *ProductHandler {
	return &ProductHandler{
		useCase: uc,
	}
}

// productFromCreateRequest maps a create request to a new product. The request
// ID is accepted for backward compatibility but ignored: the database assigns it.
func productFromCreateRequest(req dto.CreateProductRequest) domain.Product {
	return domain.Product{
		SKU:      req.SKU,
		Name:     req.Name,
		Price:    req.Price,
		Quantity: req.Quantity,
	}
}

func toProductResponse(p domain.Product) dto.ProductResponse {
	return dto.ProductResponse{
		ID:       p.ID,
		SKU:      p.SKU,
		Name:     p.Name,
		Price:    p.Price,
		Quantity: p.Quantity,
	}
}

// toProductResponsePtr keeps a nil product as a JSON null.
func toProductResponsePtr(p *domain.Product) *dto.ProductResponse {
	if p == nil {
		return nil
	}
	resp := toProductResponse(*p)
	return &resp
}

// toProductResponses keeps a nil slice as a JSON null and an empty slice as [].
func toProductResponses(products []domain.Product) []dto.ProductResponse {
	if products == nil {
		return nil
	}
	items := make([]dto.ProductResponse, len(products))
	for i, p := range products {
		items[i] = toProductResponse(p)
	}
	return items
}

func (h *ProductHandler) GetByID(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		w.Header().Set("Allow", http.MethodGet)
		response.Error(w, http.StatusMethodNotAllowed, "method not allowed")
		return
	}

	rawID := strings.TrimPrefix(r.URL.Path, "/api/v1/products/")
	if rawID == "" || strings.Contains(rawID, "/") {
		response.Error(w, http.StatusBadRequest, "invalid product ID")
		return
	}

	id, err := strconv.ParseInt(rawID, 10, 64)
	if err != nil {
		response.Error(w, http.StatusBadRequest, "invalid product ID")
		return
	}

	product, err := h.useCase.GetByID(r.Context(), id)
	if err != nil {
		WriteError(w, r, err)
		return
	}

	response.Success(w, http.StatusOK, "Product retrieved successfully", toProductResponsePtr(product))
}
func (h *ProductHandler) List(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		w.Header().Set("Allow", http.MethodGet)
		response.Error(w, http.StatusMethodNotAllowed, "method not allowed")
		return
	}

	query := r.URL.Query()
	page := 1
	limit := 10

	if rawPage := query.Get("page"); rawPage != "" {
		parsed, err := strconv.Atoi(rawPage)
		if err != nil {
			response.Error(w, http.StatusBadRequest, "invalid page")
			return
		}
		page = parsed
	}

	if rawLimit := query.Get("limit"); rawLimit != "" {
		parsed, err := strconv.Atoi(rawLimit)
		if err != nil {
			response.Error(w, http.StatusBadRequest, "invalid limit")
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

	response.Success(w, http.StatusOK, "Products retrieved successfully", toProductResponses(products))
}
func (h *ProductHandler) Create(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		w.Header().Set("Allow", http.MethodPost)
		response.Error(w, http.StatusMethodNotAllowed, "method not allowed")
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
		response.Error(w, http.StatusBadRequest, "Invalid JSON request")
		return
	}

	var extra any
	if err := decoder.Decode(&extra); err != io.EOF {
		response.Error(w, http.StatusBadRequest, "Request must contain one JSON object")
		return
	}

	product := productFromCreateRequest(req)

	if err := h.useCase.Create(r.Context(), &product); err != nil {
		WriteError(w, r, err)
		return
	}

	response.Success(w, http.StatusCreated, "Product created successfully", toProductResponse(product))
}

func (h *ProductHandler) Update(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPut {
		w.Header().Set("Allow", http.MethodPut)
		response.Error(w, http.StatusMethodNotAllowed, "method not allowed")
		return
	}

	rawID := strings.TrimPrefix(r.URL.Path, "/api/v1/products/")
	if rawID == "" || strings.Contains(rawID, "/") {
		response.Error(w, http.StatusBadRequest, "invalid product ID")
		return
	}

	id, err := strconv.ParseInt(rawID, 10, 64)
	if err != nil || id <= 0 {
		response.Error(w, http.StatusBadRequest, "invalid product ID")
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
		response.Error(w, http.StatusBadRequest, "Invalid JSON request")
		return
	}

	var extra any
	if err := decoder.Decode(&extra); err != io.EOF {
		response.Error(w, http.StatusBadRequest, "Request must contain one JSON object")
		return
	}

	// Stock may change only through stock-in/stock-out. Reject the field
	// whatever its value (including 0 and null) so a stale form value can
	// never overwrite the current quantity.
	if req.Quantity != nil {
		response.Error(w, http.StatusBadRequest, "quantity cannot be updated; use stock-in or stock-out")
		return
	}

	product := domain.Product{
		ID:    id,
		SKU:   req.SKU,
		Name:  req.Name,
		Price: req.Price,
	}

	if err := h.useCase.Update(r.Context(), &product); err != nil {
		WriteError(w, r, err)
		return
	}

	response.Success(w, http.StatusOK, "Product updated successfully", toProductResponse(product))
}

func (h *ProductHandler) Delete(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodDelete {
		w.Header().Set("Allow", http.MethodDelete)
		response.Error(w, http.StatusMethodNotAllowed, "method not allowed")
		return
	}

	rawID := strings.TrimPrefix(r.URL.Path, "/api/v1/products/")
	if rawID == "" || strings.Contains(rawID, "/") {
		response.Error(w, http.StatusBadRequest, "invalid product ID")
		return
	}

	id, err := strconv.ParseInt(rawID, 10, 64)
	if err != nil || id <= 0 {
		response.Error(w, http.StatusBadRequest, "invalid product ID")
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
		response.Error(w, http.StatusMethodNotAllowed, "method not allowed")
		return
	}

	rawID := strings.TrimPrefix(r.URL.Path, "/api/v1/products/")
	rawID = strings.TrimSuffix(rawID, "/stock-in")

	if rawID == "" || strings.Contains(rawID, "/") {
		response.Error(w, http.StatusBadRequest, "invalid product ID")
		return
	}

	id, err := strconv.ParseInt(rawID, 10, 64)
	if err != nil || id <= 0 {
		response.Error(w, http.StatusBadRequest, "invalid product ID")
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
		response.Error(w, http.StatusBadRequest, "Invalid JSON body")
		return
	}

	var extra any
	if err := decoder.Decode(&extra); err != io.EOF {
		response.Error(w, http.StatusBadRequest, "Request must contain one JSON object")
		return
	}

	product, err := h.useCase.StockIn(r.Context(), id, req.Quantity)
	if err != nil {
		WriteError(w, r, err)
		return
	}

	response.Success(w, http.StatusOK, "Stock added successfully", toProductResponsePtr(product))
}

func (h *ProductHandler) StockOut(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		w.Header().Set("Allow", http.MethodPost)
		response.Error(w, http.StatusMethodNotAllowed, "method not allowed")
		return
	}

	rawID := strings.TrimPrefix(r.URL.Path, "/api/v1/products/")
	rawID = strings.TrimSuffix(rawID, "/stock-out")

	if rawID == "" || strings.Contains(rawID, "/") {
		response.Error(w, http.StatusBadRequest, "invalid product ID")
		return
	}

	id, err := strconv.ParseInt(rawID, 10, 64)
	if err != nil || id <= 0 {
		response.Error(w, http.StatusBadRequest, "invalid product ID")
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
		response.Error(w, http.StatusBadRequest, "Invalid JSON body")
		return
	}

	var extra any
	if err := decoder.Decode(&extra); err != io.EOF {
		response.Error(w, http.StatusBadRequest, "Request must contain one JSON object")
		return
	}

	product, err := h.useCase.StockOut(r.Context(), id, req.Quantity)
	if err != nil {
		WriteError(w, r, err)
		return
	}

	response.Success(w, http.StatusOK, "Stock reduced successfully", toProductResponsePtr(product))
}
