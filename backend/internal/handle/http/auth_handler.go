package http

import (
	"encoding/json"
	"errors"
	"io"
	"net/http"
	"time"

	"gostock/internal/domain"
	"gostock/internal/dto"
	"gostock/internal/handle/http/response"
	"gostock/internal/usecase"
)

type AuthHandler struct {
	useCase *usecase.AuthUseCase
	cookies *CookieManager
}

func NewAuthHandler(uc *usecase.AuthUseCase, cookies *CookieManager) *AuthHandler {
	if cookies == nil {
		cookies = NewCookieManager(false, http.SameSiteLaxMode, 15*time.Minute, 7*24*time.Hour)
	}
	return &AuthHandler{
		useCase: uc,
		cookies: cookies,
	}
}

func (h *AuthHandler) Register(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		w.Header().Set("Allow", http.MethodPost)
		response.Error(w, http.StatusMethodNotAllowed, "method not allowed")
		return
	}

	if !EnforceJSONContentType(w, r) {
		return
	}

	r.Body = http.MaxBytesReader(w, r.Body, 1<<20)

	var req dto.RegisterRequest
	decoder := json.NewDecoder(r.Body)
	decoder.DisallowUnknownFields()

	if err := decoder.Decode(&req); err != nil {
		response.Error(w, http.StatusBadRequest, "invalid JSON request")
		return
	}

	var extra any
	if err := decoder.Decode(&extra); !errors.Is(err, io.EOF) {
		response.Error(w, http.StatusBadRequest, "request must contain exactly one JSON object")
		return
	}

	user, err := h.useCase.Register(r.Context(), req.Email, req.Password)
	if err != nil {
		WriteError(w, r, err)
		return
	}

	createdAtStr := ""
	if !user.CreatedAt.IsZero() {
		createdAtStr = user.CreatedAt.Format(time.RFC3339)
	}

	response.Success(w, http.StatusCreated, "User registered successfully", dto.AuthUserResponse{
		ID:        user.ID,
		Email:     user.Email,
		Role:      user.Role,
		CreatedAt: createdAtStr,
	})
}

func (h *AuthHandler) Login(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		w.Header().Set("Allow", http.MethodPost)
		response.Error(w, http.StatusMethodNotAllowed, "method not allowed")
		return
	}

	if !EnforceJSONContentType(w, r) {
		return
	}

	r.Body = http.MaxBytesReader(w, r.Body, 1<<20)

	var req dto.LoginRequest
	decoder := json.NewDecoder(r.Body)
	decoder.DisallowUnknownFields()

	if err := decoder.Decode(&req); err != nil {
		response.Error(w, http.StatusBadRequest, "invalid JSON request")
		return
	}

	var extra any
	if err := decoder.Decode(&extra); !errors.Is(err, io.EOF) {
		response.Error(w, http.StatusBadRequest, "request must contain exactly one JSON object")
		return
	}

	result, err := h.useCase.Login(r.Context(), req.Email, req.Password)
	if err != nil {
		WriteError(w, r, err)
		return
	}

	h.cookies.SetAuthCookies(w, result.AccessToken, result.RefreshToken)

	createdAtStr := ""
	if !result.User.CreatedAt.IsZero() {
		createdAtStr = result.User.CreatedAt.Format(time.RFC3339)
	}

	response.Success(w, http.StatusOK, "Login successful", dto.AuthUserResponse{
		ID:        result.User.ID,
		Email:     result.User.Email,
		Role:      result.User.Role,
		CreatedAt: createdAtStr,
	})
}

func (h *AuthHandler) Refresh(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		w.Header().Set("Allow", http.MethodPost)
		response.Error(w, http.StatusMethodNotAllowed, "method not allowed")
		return
	}

	cookie, err := r.Cookie(RefreshTokenCookieName)
	if err != nil || cookie.Value == "" {
		response.Error(w, http.StatusUnauthorized, "refresh token required")
		return
	}

	result, err := h.useCase.Refresh(r.Context(), cookie.Value)
	if err != nil {
		httpErr := MapError(err)
		if httpErr.StatusCode == http.StatusUnauthorized {

			h.cookies.ClearAuthCookies(w)
		}

		WriteError(w, r, httpErr)
		return
	}

	h.cookies.SetAuthCookies(w, result.AccessToken, result.RefreshToken)

	createdAtStr := ""
	if !result.User.CreatedAt.IsZero() {
		createdAtStr = result.User.CreatedAt.Format(time.RFC3339)
	}

	response.Success(w, http.StatusOK, "Token refreshed successfully", dto.AuthUserResponse{
		ID:        result.User.ID,
		Email:     result.User.Email,
		Role:      result.User.Role,
		CreatedAt: createdAtStr,
	})
}

func (h *AuthHandler) Me(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		w.Header().Set("Allow", http.MethodGet)
		response.Error(w, http.StatusMethodNotAllowed, "method not allowed")
		return
	}

	user, ok := CurrentUser(r.Context())
	if !ok || user == nil {
		response.Error(w, http.StatusUnauthorized, "authentication required")
		return
	}

	createdAtStr := ""
	if !user.CreatedAt.IsZero() {
		createdAtStr = user.CreatedAt.Format(time.RFC3339)
	}

	response.Success(w, http.StatusOK, "User profile retrieved successfully", dto.AuthUserResponse{
		ID:        user.ID,
		Email:     user.Email,
		Role:      user.Role,
		CreatedAt: createdAtStr,
	})
}

func (h *AuthHandler) Logout(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		w.Header().Set("Allow", http.MethodPost)
		response.Error(w, http.StatusMethodNotAllowed, "method not allowed")
		return
	}

	h.cookies.ClearAuthCookies(w)

	cookie, err := r.Cookie(RefreshTokenCookieName)
	if err == nil && cookie.Value != "" {
		logoutErr := h.useCase.Logout(r.Context(), cookie.Value)
		if logoutErr != nil {
			switch {
			case errors.Is(logoutErr, usecase.ErrInvalidSession),
				errors.Is(logoutErr, domain.ErrRefreshTokenNotFound),
				errors.Is(logoutErr, domain.ErrRefreshTokenRevoked),
				errors.Is(logoutErr, domain.ErrRefreshTokenExpired),
				errors.Is(logoutErr, domain.ErrInvalidRefreshToken),
				errors.Is(logoutErr, domain.ErrRefreshTokenReused):
				// Idempotent: token already invalid, revoked, expired, reused or missing is considered successful logout
			default:
				// Infrastructure or database failure: report 500 error, but cookies were already cleared
				WriteError(w, r, logoutErr)
				return
			}
		}
	}

	response.Success(w, http.StatusOK, "logged out", map[string]string{
		"message": "logged out",
	})
}
