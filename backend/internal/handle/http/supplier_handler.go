package http

import (
	"net/http"

	"gostock/internal/dto"
	"gostock/internal/usecase"
	"gostock/pkg/response"
)

// SupplierHandler serves /api/v1/suppliers (admin only, see the router).
type SupplierHandler struct {
	useCase *usecase.SupplierUseCase
}

func NewSupplierHandler(uc *usecase.SupplierUseCase) *SupplierHandler {
	return &SupplierHandler{useCase: uc}
}

func (h *SupplierHandler) List(w http.ResponseWriter, r *http.Request) {
	suppliers, err := h.useCase.List(r.Context())
	if err != nil {
		WriteError(w, r, err)
		return
	}
	response.Success(w, http.StatusOK, "Suppliers retrieved successfully", suppliers)
}

func (h *SupplierHandler) Create(w http.ResponseWriter, r *http.Request) {
	var req dto.CreateSupplierRequest
	if !decodeJSONObject(w, r, &req) {
		return
	}

	created, err := h.useCase.Create(r.Context(), dto.CreateSupplierInput{Name: req.Name, Phone: req.Phone})
	if err != nil {
		WriteError(w, r, err)
		return
	}
	response.Success(w, http.StatusCreated, "Supplier created successfully", created)
}

func (h *SupplierHandler) Update(w http.ResponseWriter, r *http.Request) {
	id, ok := catalogID(w, r, codeInvalidSupplierID, "invalid supplier ID")
	if !ok {
		return
	}
	var req dto.UpdateSupplierRequest
	if !decodeJSONObject(w, r, &req) {
		return
	}

	updated, err := h.useCase.Update(r.Context(), dto.UpdateSupplierInput{ID: id, Name: req.Name, Phone: req.Phone})
	if err != nil {
		WriteError(w, r, err)
		return
	}
	response.Success(w, http.StatusOK, "Supplier updated successfully", updated)
}

// Delete answers 204 No Content, like DELETE /api/v1/products/{id}.
func (h *SupplierHandler) Delete(w http.ResponseWriter, r *http.Request) {
	id, ok := catalogID(w, r, codeInvalidSupplierID, "invalid supplier ID")
	if !ok {
		return
	}
	if err := h.useCase.Delete(r.Context(), id); err != nil {
		WriteError(w, r, err)
		return
	}
	response.NoContent(w)
}
