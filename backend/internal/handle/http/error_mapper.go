package http

import (
	"context"
	"errors"
	"log/slog"
	"net/http"
	"strings"
	"unicode"

	"gostock/internal/domain"
	"gostock/internal/handle/http/response"
	"gostock/internal/usecase"
)

// HTTPError encapsulates an HTTP status code, a safe public client message,
// optional field-level validation errors, and the underlying root cause error.
type HTTPError struct {
	StatusCode int
	Message    string
	Fields     map[string]string // nil if not a field-level validation error
	Cause      error
}

func (e *HTTPError) Error() string {
	if e == nil {
		return ""
	}
	if e.Cause != nil {
		return e.Message + ": " + e.Cause.Error()
	}
	return e.Message
}

func (e *HTTPError) Unwrap() error {
	if e == nil {
		return nil
	}
	return e.Cause
}

// NewValidationError constructs an HTTPError for 400 Bad Request with field-level details.
func NewValidationError(message string, fields map[string]string, cause error) *HTTPError {
	return &HTTPError{
		StatusCode: http.StatusBadRequest,
		Message:    message,
		Fields:     fields,
		Cause:      cause,
	}
}

// MapError translates a Domain or UseCase error into an appropriate HTTPError.
// It uses errors.Is to inspect root sentinel errors and ensures that internal details,
// SQL queries, and sensitive data are never exposed in public messages.
func MapError(err error) *HTTPError {
	if err == nil {
		return nil
	}

	// If the error is already an *HTTPError, return it as-is
	var httpErr *HTTPError
	if errors.As(err, &httpErr) {
		return httpErr
	}

	switch {
	// 400 Bad Request — Client input / validation errors
	case errors.Is(err, usecase.ErrInvalidID):
		return &HTTPError{StatusCode: http.StatusBadRequest, Message: "invalid product ID", Cause: err}
	case errors.Is(err, usecase.ErrInvalidUserID):
		return &HTTPError{StatusCode: http.StatusBadRequest, Message: "invalid user ID", Cause: err}
	case errors.Is(err, usecase.ErrInvalidUserRole):
		return &HTTPError{StatusCode: http.StatusBadRequest, Message: "invalid user role", Cause: err}
	case errors.Is(err, usecase.ErrEmptyUpdate):
		return &HTTPError{StatusCode: http.StatusBadRequest, Message: "at least one field must be provided for update", Cause: err}
	case errors.Is(err, usecase.ErrInvalidUser):
		return &HTTPError{StatusCode: http.StatusBadRequest, Message: "invalid user", Cause: err}
	case errors.Is(err, usecase.ErrInvalidPage):
		return &HTTPError{StatusCode: http.StatusBadRequest, Message: "invalid page", Cause: err}
	case errors.Is(err, usecase.ErrInvalidLimit):
		return &HTTPError{StatusCode: http.StatusBadRequest, Message: "invalid limit", Cause: err}
	case errors.Is(err, usecase.ErrInvalidProduct):
		return &HTTPError{StatusCode: http.StatusBadRequest, Message: "Invalid product", Cause: err}
	case errors.Is(err, usecase.ErrInvalidQuantity):
		return &HTTPError{StatusCode: http.StatusBadRequest, Message: "quantity must be greater than 0", Cause: err}
	case errors.Is(err, usecase.ErrInvalidEmail):
		return &HTTPError{StatusCode: http.StatusBadRequest, Message: "invalid email", Cause: err}
	case errors.Is(err, usecase.ErrInvalidPassword):
		return &HTTPError{StatusCode: http.StatusBadRequest, Message: "invalid password", Cause: err}

	// 401 Unauthorized — Authentication credentials / Session / Refresh token errors
	case errors.Is(err, usecase.ErrInvalidCredentials):
		return &HTTPError{StatusCode: http.StatusUnauthorized, Message: "invalid email or password", Cause: err}
	case errors.Is(err, usecase.ErrInvalidSession):
		return &HTTPError{StatusCode: http.StatusUnauthorized, Message: "invalid or expired session", Cause: err}
	case errors.Is(err, domain.ErrInvalidRefreshToken),
		errors.Is(err, domain.ErrRefreshTokenNotFound),
		errors.Is(err, domain.ErrRefreshTokenRevoked),
		errors.Is(err, domain.ErrRefreshTokenExpired),
		errors.Is(err, domain.ErrRefreshTokenReused):
		return &HTTPError{StatusCode: http.StatusUnauthorized, Message: "invalid or expired refresh token", Cause: err}

	// 404 Not Found — Resource not found
	case errors.Is(err, domain.ErrProductNotFound):
		return &HTTPError{StatusCode: http.StatusNotFound, Message: "Product not found", Cause: err}
	case errors.Is(err, domain.ErrUserNotFound):
		// Note: In auth flows (Login/Refresh), domain.ErrUserNotFound is already translated by
		// AuthUseCase into ErrInvalidCredentials / ErrInvalidSession (401). If ErrUserNotFound
		// reaches the HTTP layer in user-management/resource endpoints, it indicates 404 Not Found.
		return &HTTPError{StatusCode: http.StatusNotFound, Message: "user not found", Cause: err}

	// 409 Conflict — Business conflict & concurrent operations
	case errors.Is(err, domain.ErrConcurrentRefresh):
		return &HTTPError{StatusCode: http.StatusConflict, Message: "concurrent refresh in progress", Cause: err}
	case errors.Is(err, domain.ErrProductSKUExists):
		return &HTTPError{StatusCode: http.StatusConflict, Message: "SKU already exists", Cause: err}
	case errors.Is(err, domain.ErrEmailExists):
		return &HTTPError{StatusCode: http.StatusConflict, Message: "email already exists", Cause: err}
	case errors.Is(err, domain.ErrInsufficientStock):
		return &HTTPError{StatusCode: http.StatusConflict, Message: "Insufficient stock", Cause: err}

	// 500 Internal Server Error — Server misconfiguration (e.g. non-positive TTL) or infrastructure failure
	case errors.Is(err, usecase.ErrInvalidTokenTTL):
		return &HTTPError{StatusCode: http.StatusInternalServerError, Message: response.InternalServerErrorMessage, Cause: err}

	default:
		return &HTTPError{StatusCode: http.StatusInternalServerError, Message: response.InternalServerErrorMessage, Cause: err}
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
