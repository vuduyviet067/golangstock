package http

import (
	"encoding/json"
	"io"
	"net/http"
	"strconv"
	"strings"

	"gostock/internal/domain"
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

	response.Success(w, http.StatusOK, "Product retrieved successfully", product)
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

	response.Success(w, http.StatusOK, "Products retrieved successfully", products)
}
func (h *ProductHandler) Create(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		w.Header().Set("Allow", http.MethodPost)
		response.Error(w, http.StatusMethodNotAllowed, "method not allowed")
		return
	}

	r.Body = http.MaxBytesReader(w, r.Body, 1<<20)

	var product domain.Product

	decoder := json.NewDecoder(r.Body)
	decoder.DisallowUnknownFields()

	if err := decoder.Decode(&product); err != nil {
		response.Error(w, http.StatusBadRequest, "Invalid JSON request")
		return
	}

	var extra any
	if err := decoder.Decode(&extra); err != io.EOF {
		response.Error(w, http.StatusBadRequest, "Request must contain one JSON object")
		return
	}

	if err := h.useCase.Create(r.Context(), &product); err != nil {
		WriteError(w, r, err)
		return
	}

	response.Success(w, http.StatusCreated, "Product created successfully", product)
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

	r.Body = http.MaxBytesReader(w, r.Body, 1<<20)

	var product domain.Product

	decoder := json.NewDecoder(r.Body)
	decoder.DisallowUnknownFields()

	if err := decoder.Decode(&product); err != nil {
		response.Error(w, http.StatusBadRequest, "Invalid JSON request")
		return
	}

	var extra any
	if err := decoder.Decode(&extra); err != io.EOF {
		response.Error(w, http.StatusBadRequest, "Request must contain one JSON object")
		return
	}

	product.ID = id

	if err := h.useCase.Update(r.Context(), &product); err != nil {
		WriteError(w, r, err)
		return
	}

	response.Success(w, http.StatusOK, "Product updated successfully", product)
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

	r.Body = http.MaxBytesReader(w, r.Body, 1<<20)

	var req struct {
		Quantity int `json:"quantity"`
	}

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

	response.Success(w, http.StatusOK, "Stock added successfully", product)
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

	r.Body = http.MaxBytesReader(w, r.Body, 1<<20)

	var req struct {
		Quantity int `json:"quantity"`
	}

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

	response.Success(w, http.StatusOK, "Stock reduced successfully", product)
}
