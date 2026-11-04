package http

import (
	"encoding/json"
	"net/http"
	"strconv"
	"strings"
	"time"

	"gostock/internal/domain"
	"gostock/internal/handle/http/response"
	"gostock/internal/usecase"
)

type UserResponse struct {
	ID        int64     `json:"id"`
	Email     string    `json:"email"`
	Role      string    `json:"role"`
	CreatedAt time.Time `json:"created_at"`
	UpdatedAt time.Time `json:"updated_at"`
}

func toUserResponse(u domain.User) UserResponse {
	return UserResponse{
		ID:        u.ID,
		Email:     u.Email,
		Role:      u.Role,
		CreatedAt: u.CreatedAt,
		UpdatedAt: u.UpdatedAt,
	}
}

type UserHandler struct {
	useCase *usecase.UserUseCase
}

func NewUserHandler(uc *usecase.UserUseCase) *UserHandler {
	return &UserHandler{
		useCase: uc,
	}
}

func (h *UserHandler) List(w http.ResponseWriter, r *http.Request) {
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

	users, total, err := h.useCase.List(r.Context(), query.Get("q"), page, limit)
	if err != nil {
		WriteError(w, r, err)
		return
	}

	items := make([]UserResponse, len(users))
	for i, u := range users {
		items[i] = toUserResponse(u)
	}

	response.Paginated(w, "Users retrieved successfully", items, page, limit, total)
}

func (h *UserHandler) GetByID(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		w.Header().Set("Allow", http.MethodGet)
		response.Error(w, http.StatusMethodNotAllowed, "method not allowed")
		return
	}

	rawID := strings.TrimPrefix(r.URL.Path, "/api/v1/users/")
	if rawID == "" || strings.Contains(rawID, "/") {
		response.Error(w, http.StatusBadRequest, "invalid user ID")
		return
	}

	id, err := strconv.ParseInt(rawID, 10, 64)
	if err != nil || id <= 0 {
		response.Error(w, http.StatusBadRequest, "invalid user ID")
		return
	}

	user, err := h.useCase.GetByID(r.Context(), id)
	if err != nil {
		WriteError(w, r, err)
		return
	}

	response.Success(w, http.StatusOK, "User retrieved successfully", toUserResponse(*user))
}

type updateUserInputRequest struct {
	Email *string `json:"email"`
	Role  *string `json:"role"`
}

func (h *UserHandler) Update(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPatch {
		w.Header().Set("Allow", http.MethodPatch)
		response.Error(w, http.StatusMethodNotAllowed, "method not allowed")
		return
	}

	rawID := strings.TrimPrefix(r.URL.Path, "/api/v1/users/")
	if rawID == "" || strings.Contains(rawID, "/") {
		response.Error(w, http.StatusBadRequest, "invalid user ID")
		return
	}

	id, err := strconv.ParseInt(rawID, 10, 64)
	if err != nil || id <= 0 {
		response.Error(w, http.StatusBadRequest, "invalid user ID")
		return
	}

	r.Body = http.MaxBytesReader(w, r.Body, 1048576)
	dec := json.NewDecoder(r.Body)
	dec.DisallowUnknownFields()

	var req updateUserInputRequest
	if err := dec.Decode(&req); err != nil {
		response.Error(w, http.StatusBadRequest, "invalid request body")
		return
	}

	updatedUser, err := h.useCase.Update(r.Context(), id, usecase.UpdateUserInput{
		Email: req.Email,
		Role:  req.Role,
	})
	if err != nil {
		WriteError(w, r, err)
		return
	}

	response.Success(w, http.StatusOK, "User updated successfully", toUserResponse(*updatedUser))
}
