package http

import (
	"context"
	"errors"
	"log/slog"
	"net/http"
	"strings"
	"unicode"

	"gostock/internal/domain"
	"gostock/internal/dto"
	"gostock/internal/handle/http/response"
	"gostock/internal/usecase"
)

// NewValidationError constructs an HTTPError for 400 Bad Request with field-level details.
func NewValidationError(message string, fields map[string]string, cause error) *dto.HTTPError {
	return &dto.HTTPError{
		StatusCode: http.StatusBadRequest,
		Message:    message,
		Fields:     fields,
		Cause:      cause,
	}
}

// MapError translates a Domain or UseCase error into an appropriate HTTPError.
// It uses errors.Is to inspect root sentinel errors and ensures that internal details,
// SQL queries, and sensitive data are never exposed in public messages.
func MapError(err error) *dto.HTTPError {
	if err == nil {
		return nil
	}

	// If the error is already a *dto.HTTPError, return it as-is
	var httpErr *dto.HTTPError
	if errors.As(err, &httpErr) {
		return httpErr
	}

	switch {
	// 400 Bad Request — Client input / validation errors
	case errors.Is(err, usecase.ErrInvalidID):
		return &dto.HTTPError{StatusCode: http.StatusBadRequest, Message: "invalid product ID", Cause: err}
	case errors.Is(err, usecase.ErrInvalidUserID):
		return &dto.HTTPError{StatusCode: http.StatusBadRequest, Message: "invalid user ID", Cause: err}
	case errors.Is(err, usecase.ErrInvalidUserRole):
		return &dto.HTTPError{StatusCode: http.StatusBadRequest, Message: "invalid user role", Cause: err}
	case errors.Is(err, usecase.ErrEmptyUpdate):
		return &dto.HTTPError{StatusCode: http.StatusBadRequest, Message: "at least one field must be provided for update", Cause: err}
	case errors.Is(err, usecase.ErrInvalidUser):
		return &dto.HTTPError{StatusCode: http.StatusBadRequest, Message: "invalid user", Cause: err}
	case errors.Is(err, usecase.ErrInvalidPage):
		return &dto.HTTPError{StatusCode: http.StatusBadRequest, Message: "invalid page", Cause: err}
	case errors.Is(err, usecase.ErrInvalidLimit):
		return &dto.HTTPError{StatusCode: http.StatusBadRequest, Message: "invalid limit", Cause: err}
	case errors.Is(err, usecase.ErrInvalidProduct):
		return &dto.HTTPError{StatusCode: http.StatusBadRequest, Message: "Invalid product", Cause: err}
	case errors.Is(err, usecase.ErrInvalidQuantity):
		return &dto.HTTPError{StatusCode: http.StatusBadRequest, Message: "quantity must be greater than 0", Cause: err}
	case errors.Is(err, usecase.ErrInvalidEmail):
		return &dto.HTTPError{StatusCode: http.StatusBadRequest, Message: "invalid email", Cause: err}
	case errors.Is(err, usecase.ErrInvalidPassword):
		return &dto.HTTPError{StatusCode: http.StatusBadRequest, Message: "invalid password", Cause: err}

	// 401 Unauthorized — Authentication credentials / Session / Refresh token errors
	case errors.Is(err, usecase.ErrInvalidCredentials):
		return &dto.HTTPError{StatusCode: http.StatusUnauthorized, Message: "invalid email or password", Cause: err}
	case errors.Is(err, usecase.ErrInvalidSession):
		return &dto.HTTPError{StatusCode: http.StatusUnauthorized, Message: "invalid or expired session", Cause: err}
	case errors.Is(err, domain.ErrInvalidRefreshToken),
		errors.Is(err, domain.ErrRefreshTokenNotFound),
		errors.Is(err, domain.ErrRefreshTokenRevoked),
		errors.Is(err, domain.ErrRefreshTokenExpired),
		errors.Is(err, domain.ErrRefreshTokenReused):
		return &dto.HTTPError{StatusCode: http.StatusUnauthorized, Message: "invalid or expired refresh token", Cause: err}

	// 404 Not Found — Resource not found
	case errors.Is(err, domain.ErrProductNotFound):
		return &dto.HTTPError{StatusCode: http.StatusNotFound, Message: "Product not found", Cause: err}
	case errors.Is(err, domain.ErrUserNotFound):
		// Note: In auth flows (Login/Refresh), domain.ErrUserNotFound is already translated by
		// AuthUseCase into ErrInvalidCredentials / ErrInvalidSession (401). If ErrUserNotFound
		// reaches the HTTP layer in user-management/resource endpoints, it indicates 404 Not Found.
		return &dto.HTTPError{StatusCode: http.StatusNotFound, Message: "user not found", Cause: err}

	// 409 Conflict — Business conflict & concurrent operations
	case errors.Is(err, domain.ErrConcurrentRefresh):
		return &dto.HTTPError{StatusCode: http.StatusConflict, Message: "concurrent refresh in progress", Cause: err}
	case errors.Is(err, domain.ErrProductSKUExists):
		return &dto.HTTPError{StatusCode: http.StatusConflict, Message: "SKU already exists", Cause: err}
	case errors.Is(err, domain.ErrEmailExists):
		return &dto.HTTPError{StatusCode: http.StatusConflict, Message: "email already exists", Cause: err}
	case errors.Is(err, domain.ErrInsufficientStock):
		return &dto.HTTPError{StatusCode: http.StatusConflict, Message: "Insufficient stock", Cause: err}

	// 500 Internal Server Error — Server misconfiguration (e.g. non-positive TTL) or infrastructure failure
	case errors.Is(err, usecase.ErrInvalidTokenTTL):
		return &dto.HTTPError{StatusCode: http.StatusInternalServerError, Message: response.InternalServerErrorMessage, Cause: err}

	default:
		return &dto.HTTPError{StatusCode: http.StatusInternalServerError, Message: response.InternalServerErrorMessage, Cause: err}
	}
}

const maxRequestIDLength = 128

// extractRequestID extracts and sanitizes an externally supplied correlation ID
// from the incoming request's X-Request-ID header.
// It rejects empty values, values exceeding maxRequestIDLength, or values containing control characters.
// This value is treated as an untrusted external correlation ID, not a trusted server-generated identifier.
func extractRequestID(r *http.Request) string {
	if r == nil {
		return ""
	}
	reqID := strings.TrimSpace(r.Header.Get("X-Request-ID"))
	if reqID == "" || len(reqID) > maxRequestIDLength {
		return ""
	}
	for _, ch := range reqID {
		if unicode.IsControl(ch) {
			return ""
		}
	}
	return reqID
}

// WriteError translates err using MapError and writes the corresponding standard response envelope.
// If the error maps to an internal server error (500), it safely logs the underlying cause internally
// using structured logging (slog.ErrorContext) with request context and request ID (if available),
// without leaking sensitive details or stack traces to the client.
func WriteError(w http.ResponseWriter, r *http.Request, err error) {
	httpErr := MapError(err)
	if httpErr == nil {
		return
	}

	if httpErr.StatusCode == http.StatusInternalServerError && httpErr.Cause != nil {
		ctx := context.Background()
		var attrs []any
		if r != nil {
			if r.Context() != nil {
				ctx = r.Context()
			}
			if reqID := extractRequestID(r); reqID != "" {
				attrs = append(attrs, slog.String("request_id", reqID))
			}
		}
		attrs = append(attrs, slog.String("error", httpErr.Cause.Error()))
		slog.ErrorContext(ctx, "internal server error", attrs...)
	}

	if len(httpErr.Fields) > 0 {
		response.ValidationError(w, httpErr.Message, httpErr.Fields)
		return
	}

	response.Error(w, httpErr.StatusCode, httpErr.Message)
}
