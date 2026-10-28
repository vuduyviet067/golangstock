package http

import (
	"encoding/json"
	"errors"
	"io"
	"net/http"
	"strconv"
	"strings"

	"gostock/internal/domain"
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

func writeJSON(w http.ResponseWriter, status int, data any) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)

	if err := json.NewEncoder(w).Encode(data); err != nil {
		return
	}
}

func writeError(w http.ResponseWriter, status int, message string) {
	writeJSON(w, status, map[string]string{
		"error": message,
	})
}

func (h *ProductHandler) GetByID(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		w.Header().Set("Allow", http.MethodGet)
		writeError(w, http.StatusMethodNotAllowed, "method not allowed")
		return
	}

	rawID := strings.TrimPrefix(r.URL.Path, "/api/v1/products/")
	if rawID == "" || strings.Contains(rawID, "/") {
		writeError(w, http.StatusBadRequest, "invalid product ID")
		return
	}

	id, err := strconv.ParseInt(rawID, 10, 64)
	if err != nil {
		writeError(w, http.StatusBadRequest, "invalid product ID")
		return
	}

	product, err := h.useCase.GetByID(r.Context(), id)
	if err != nil {
		switch {
		case errors.Is(err, usecase.ErrInvalidID):
			writeError(w, http.StatusBadRequest, "invalid product ID")
		case errors.Is(err, domain.ErrProductNotFound):
			writeError(w, http.StatusNotFound, "Product not found")
		default:
			writeError(w, http.StatusInternalServerError, "Internal server error")
		}
		return
	}

	writeJSON(w, http.StatusOK, product)
}
func (h *ProductHandler) List(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		w.Header().Set("Allow", http.MethodGet)
		writeError(w, http.StatusMethodNotAllowed, "method not allowed")
		return
	}

	query := r.URL.Query()
	page := 1
	limit := 10

	if rawPage := query.Get("page"); rawPage != "" {
		parsed, err := strconv.Atoi(rawPage)
		if err != nil {
			writeError(w, http.StatusBadRequest, "invalid page")
			return
		}
		page = parsed
	}

	if rawLimit := query.Get("limit"); rawLimit != "" {
		parsed, err := strconv.Atoi(rawLimit)
		if err != nil {
			writeError(w, http.StatusBadRequest, "invalid limit")
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
		if errors.Is(err, usecase.ErrInvalidProduct) {
			writeError(w, http.StatusBadRequest, "invalid list parameters")
			return
		}

		writeError(w, http.StatusInternalServerError, "Internal server error")
		return
	}

	writeJSON(w, http.StatusOK, products)
}
func (h *ProductHandler) Create(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		w.Header().Set("Allow", http.MethodPost)
		writeError(w, http.StatusMethodNotAllowed, "method not allowed")
		return
	}

	r.Body = http.MaxBytesReader(w, r.Body, 1<<20)

	var product domain.Product

	decoder := json.NewDecoder(r.Body)
	decoder.DisallowUnknownFields()

	if err := decoder.Decode(&product); err != nil {
		writeError(w, http.StatusBadRequest, "Invalid JSON request")
		return
	}

	var extra any
	if err := decoder.Decode(&extra); err != io.EOF {
		writeError(w, http.StatusBadRequest, "Request must contain one JSON object")
		return
	}

	if err := h.useCase.Create(r.Context(), &product); err != nil {
		switch {
		case errors.Is(err, usecase.ErrInvalidProduct):
			writeError(w, http.StatusBadRequest, "Invalid product")
		case errors.Is(err, domain.ErrProductSKUExists):
			writeError(w, http.StatusConflict, "SKU already exists")
		default:
			writeError(w, http.StatusInternalServerError, "Internal server error")
		}
		return
	}

	writeJSON(w, http.StatusCreated, product)
}

func (h *ProductHandler) Update(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPut {
		w.Header().Set("Allow", http.MethodPut)
		writeError(w, http.StatusMethodNotAllowed, "method not allowed")
		return
	}

	rawID := strings.TrimPrefix(r.URL.Path, "/api/v1/products/")
	if rawID == "" || strings.Contains(rawID, "/") {
		writeError(w, http.StatusBadRequest, "invalid product ID")
		return
	}

	id, err := strconv.ParseInt(rawID, 10, 64)
	if err != nil || id <= 0 {
		writeError(w, http.StatusBadRequest, "invalid product ID")
		return
	}

	r.Body = http.MaxBytesReader(w, r.Body, 1<<20)

	var product domain.Product

	decoder := json.NewDecoder(r.Body)
	decoder.DisallowUnknownFields()

	if err := decoder.Decode(&product); err != nil {
		writeError(w, http.StatusBadRequest, "Invalid JSON request")
		return
	}

	var extra any
	if err := decoder.Decode(&extra); err != io.EOF {
		writeError(w, http.StatusBadRequest, "Request must contain one JSON object")
		return
	}

	product.ID = id

	if err := h.useCase.Update(r.Context(), &product); err != nil {
		switch {
		case errors.Is(err, usecase.ErrInvalidID):
			writeError(w, http.StatusBadRequest, "invalid product ID")
		case errors.Is(err, usecase.ErrInvalidProduct):
			writeError(w, http.StatusBadRequest, "Invalid product")
		case errors.Is(err, domain.ErrProductNotFound):
			writeError(w, http.StatusNotFound, "Product not found")
		case errors.Is(err, domain.ErrProductSKUExists):
			writeError(w, http.StatusConflict, "SKU already exists")
		default:
			writeError(w, http.StatusInternalServerError, "Internal server error")
		}
		return
	}

	writeJSON(w, http.StatusOK, product)
}

func (h *ProductHandler) Delete(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodDelete {
		w.Header().Set("Allow", http.MethodDelete)
		writeError(w, http.StatusMethodNotAllowed, "method not allowed")
		return
	}

	rawID := strings.TrimPrefix(r.URL.Path, "/api/v1/products/")
	if rawID == "" || strings.Contains(rawID, "/") {
		writeError(w, http.StatusBadRequest, "invalid product ID")
		return
	}

	id, err := strconv.ParseInt(rawID, 10, 64)
	if err != nil || id <= 0 {
		writeError(w, http.StatusBadRequest, "invalid product ID")
		return
	}

	if err := h.useCase.Delete(r.Context(), id); err != nil {
		switch {
		case errors.Is(err, usecase.ErrInvalidID):
			writeError(w, http.StatusBadRequest, "invalid product ID")
		case errors.Is(err, domain.ErrProductNotFound):
			writeError(w, http.StatusNotFound, "Product not found")
		default:
			writeError(w, http.StatusInternalServerError, "Internal server error")
		}
		return
	}

	w.WriteHeader(http.StatusNoContent)
}

func (h *ProductHandler) StockIn(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		w.Header().Set("Allow", http.MethodPost)
		writeError(w, http.StatusMethodNotAllowed, "method not allowed")
		return
	}

	rawID := strings.TrimPrefix(r.URL.Path, "/api/v1/products/")
	rawID = strings.TrimSuffix(rawID, "/stock-in")

	if rawID == "" || strings.Contains(rawID, "/") {
		writeError(w, http.StatusBadRequest, "invalid product ID")
		return
	}

	id, err := strconv.ParseInt(rawID, 10, 64)
	if err != nil || id <= 0 {
		writeError(w, http.StatusBadRequest, "invalid product ID")
		return
	}

	r.Body = http.MaxBytesReader(w, r.Body, 1<<20)

	var req struct {
		Quantity int `json:"quantity"`
	}

	decoder := json.NewDecoder(r.Body)
	decoder.DisallowUnknownFields()

	if err := decoder.Decode(&req); err != nil {
		writeError(w, http.StatusBadRequest, "Invalid JSON body")
		return
	}

	var extra any
	if err := decoder.Decode(&extra); err != io.EOF {
		writeError(w, http.StatusBadRequest, "Request must contain one JSON object")
		return
	}

	product, err := h.useCase.StockIn(r.Context(), id, req.Quantity)
	if err != nil {
		switch {
		case errors.Is(err, usecase.ErrInvalidID):
			writeError(w, http.StatusBadRequest, "invalid product ID")
		case errors.Is(err, usecase.ErrInvalidQuantity):
			writeError(w, http.StatusBadRequest, "quantity must be greater than 0")
		case errors.Is(err, domain.ErrProductNotFound):
			writeError(w, http.StatusNotFound, "Product not found")
		default:
			writeError(w, http.StatusInternalServerError, "Internal server error")
		}
		return
	}

	writeJSON(w, http.StatusOK, product)
}

func (h *ProductHandler) StockOut(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		w.Header().Set("Allow", http.MethodPost)
		writeError(w, http.StatusMethodNotAllowed, "method not allowed")
		return
	}

	rawID := strings.TrimPrefix(r.URL.Path, "/api/v1/products/")
	rawID = strings.TrimSuffix(rawID, "/stock-out")

	if rawID == "" || strings.Contains(rawID, "/") {
		writeError(w, http.StatusBadRequest, "invalid product ID")
		return
	}

	id, err := strconv.ParseInt(rawID, 10, 64)
	if err != nil || id <= 0 {
		writeError(w, http.StatusBadRequest, "invalid product ID")
		return
	}

	r.Body = http.MaxBytesReader(w, r.Body, 1<<20)

	var req struct {
		Quantity int `json:"quantity"`
	}

	decoder := json.NewDecoder(r.Body)
	decoder.DisallowUnknownFields()

	if err := decoder.Decode(&req); err != nil {
		writeError(w, http.StatusBadRequest, "Invalid JSON body")
		return
	}

	var extra any
	if err := decoder.Decode(&extra); err != io.EOF {
		writeError(w, http.StatusBadRequest, "Request must contain one JSON object")
		return
	}

	product, err := h.useCase.StockOut(r.Context(), id, req.Quantity)
	if err != nil {
		switch {
		case errors.Is(err, usecase.ErrInvalidID):
			writeError(w, http.StatusBadRequest, "invalid product ID")
		case errors.Is(err, usecase.ErrInvalidQuantity):
			writeError(w, http.StatusBadRequest, "quantity must be greater than 0")
		case errors.Is(err, domain.ErrProductNotFound):
			writeError(w, http.StatusNotFound, "Product not found")
		case errors.Is(err, domain.ErrInsufficientStock):
			writeError(w, http.StatusConflict, "Insufficient stock")
		default:
			writeError(w, http.StatusInternalServerError, "Internal server error")
		}
		return
	}

	writeJSON(w, http.StatusOK, product)
}
