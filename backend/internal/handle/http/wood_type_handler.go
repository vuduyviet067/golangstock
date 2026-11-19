package http

import (
	"net/http"

	"gostock/internal/dto"
	"gostock/internal/usecase"
	"gostock/pkg/response"
)

// WoodTypeHandler serves /api/v1/wood-types (admin only, see the router).
type WoodTypeHandler struct {
	useCase *usecase.WoodTypeUseCase
}

func NewWoodTypeHandler(uc *usecase.WoodTypeUseCase) *WoodTypeHandler {
	return &WoodTypeHandler{useCase: uc}
}

func (h *WoodTypeHandler) List(w http.ResponseWriter, r *http.Request) {
	woodTypes, err := h.useCase.List(r.Context())
	if err != nil {
		WriteError(w, r, err)
		return
	}
	response.Success(w, http.StatusOK, "Wood types retrieved successfully", woodTypes)
}

func (h *WoodTypeHandler) Create(w http.ResponseWriter, r *http.Request) {
	var req dto.CreateWoodTypeRequest
	if !decodeJSONObject(w, r, &req) {
		return
	}

	created, err := h.useCase.Create(r.Context(), dto.CreateWoodTypeInput{Name: req.Name})
	if err != nil {
		WriteError(w, r, err)
		return
	}
	response.Success(w, http.StatusCreated, "Wood type created successfully", created)
}

func (h *WoodTypeHandler) Update(w http.ResponseWriter, r *http.Request) {
	id, ok := catalogID(w, r, codeInvalidWoodTypeID, "invalid wood type ID")
	if !ok {
		return
	}
	var req dto.UpdateWoodTypeRequest
	if !decodeJSONObject(w, r, &req) {
		return
	}

	updated, err := h.useCase.Update(r.Context(), dto.UpdateWoodTypeInput{ID: id, Name: req.Name})
	if err != nil {
		WriteError(w, r, err)
		return
	}
	response.Success(w, http.StatusOK, "Wood type updated successfully", updated)
}

// Delete answers 204 No Content, like DELETE /api/v1/products/{id}.
func (h *WoodTypeHandler) Delete(w http.ResponseWriter, r *http.Request) {
	id, ok := catalogID(w, r, codeInvalidWoodTypeID, "invalid wood type ID")
	if !ok {
		return
	}
	if err := h.useCase.Delete(r.Context(), id); err != nil {
		WriteError(w, r, err)
		return
	}
	response.NoContent(w)
}
