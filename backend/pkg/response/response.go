// Package response writes the standard JSON envelope of the HTTP API:
//
//	{"success": bool, "code": "UPPER_SNAKE" (errors only), "message": string, "data": any}
//
// It is shared code: it imports only the standard library and nothing from
// gostock/internal.
package response

import (
	"encoding/json"
	"log/slog"
	"net/http"
)

// InternalServerErrorMessage is the unified safe message for HTTP 500 responses.
const InternalServerErrorMessage = "Internal server error"

// General error codes. An error that has a more specific meaning carries its
// own code (for example INSUFFICIENT_STOCK), defined by the HTTP layer that
// maps it; these are the codes for everything else.
const (
	CodeValidationError      = "VALIDATION_ERROR"       // 400
	CodeUnauthenticated      = "UNAUTHENTICATED"        // 401
	CodeForbidden            = "FORBIDDEN"              // 403
	CodeNotFound             = "NOT_FOUND"              // 404
	CodeMethodNotAllowed     = "METHOD_NOT_ALLOWED"     // 405
	CodeConflict             = "CONFLICT"               // 409
	CodeUnsupportedMediaType = "UNSUPPORTED_MEDIA_TYPE" // 415
	CodeInternalError        = "INTERNAL_ERROR"         // 500
	CodeServiceUnavailable   = "SERVICE_UNAVAILABLE"    // 503
)

// APIResponse is the envelope of every JSON response. Code is set on errors
// only and is left out of successful responses.
type APIResponse struct {
	Success bool   `json:"success"`
	Code    string `json:"code,omitempty"`
	Message string `json:"message"`
	Data    any    `json:"data"`
}

// PaginatedData is the data of a paginated list response.
type PaginatedData struct {
	Items      any            `json:"items"`
	Pagination PaginationMeta `json:"pagination"`
}

// PaginationMeta describes the page returned in PaginatedData.
type PaginationMeta struct {
	Page       int   `json:"page"`
	Limit      int   `json:"limit"`
	Total      int64 `json:"total"`
	TotalPages int   `json:"total_pages"`
}

// CodeForStatus returns the general error code of an HTTP status. Statuses
// without a general code get INTERNAL_ERROR (5xx) or CLIENT_ERROR (others).
func CodeForStatus(status int) string {
	switch status {
	case http.StatusBadRequest:
		return CodeValidationError
	case http.StatusUnauthorized:
		return CodeUnauthenticated
	case http.StatusForbidden:
		return CodeForbidden
	case http.StatusNotFound:
		return CodeNotFound
	case http.StatusMethodNotAllowed:
		return CodeMethodNotAllowed
	case http.StatusConflict:
		return CodeConflict
	case http.StatusUnsupportedMediaType:
		return CodeUnsupportedMediaType
	case http.StatusServiceUnavailable:
		return CodeServiceUnavailable
	}
	if status >= http.StatusInternalServerError {
		return CodeInternalError
	}
	return "CLIENT_ERROR"
}

// Success writes a standard success response with the given status code, message, and data.
func Success(w http.ResponseWriter, status int, message string, data any) {
	write(w, status, APIResponse{
		Success: true,
		Message: message,
		Data:    data,
	})
}

// Created writes a standard HTTP 201 Created response with the given message and payload.
func Created(w http.ResponseWriter, message string, data any) {
	Success(w, http.StatusCreated, message, data)
}

// Error writes a standard error response with the given status code, error
// code and message. An empty code is replaced by CodeForStatus(status), so an
// error response always has one. The data field is null.
func Error(w http.ResponseWriter, status int, code, message string) {
	if code == "" {
		code = CodeForStatus(status)
	}
	write(w, status, APIResponse{
		Success: false,
		Code:    code,
		Message: message,
		Data:    nil,
	})
}

// ValidationError writes a standard HTTP 400 Bad Request error response with
// field-level details in data.fields. Callers pass CodeValidationError; an
// empty code is replaced by it.
func ValidationError(w http.ResponseWriter, code, message string, fields map[string]string) {
	if code == "" {
		code = CodeValidationError
	}
	write(w, http.StatusBadRequest, APIResponse{
		Success: false,
		Code:    code,
		Message: message,
		Data: map[string]any{
			"fields": fields,
		},
	})
}

// Paginated writes a standard HTTP 200 OK response with items and pagination metadata encapsulated inside data.
func Paginated(w http.ResponseWriter, message string, items any, page, limit int, total int64) {
	totalPages := 0
	if limit > 0 {
		totalPages = int((total + int64(limit) - 1) / int64(limit))
	}
	Success(w, http.StatusOK, message, PaginatedData{
		Items: items,
		Pagination: PaginationMeta{
			Page:       page,
			Limit:      limit,
			Total:      total,
			TotalPages: totalPages,
		},
	})
}

// NoContent writes an HTTP 204 No Content response without a body.
func NoContent(w http.ResponseWriter) {
	w.WriteHeader(http.StatusNoContent)
}

// write sets Content-Type to application/json, marshals the payload, and writes the response.
// If marshaling fails, it logs the error internally and returns a safe 500 error envelope
// without leaking SQL, stack traces, or internal error details to the client.
func write(w http.ResponseWriter, status int, payload any) {
	w.Header().Set("Content-Type", "application/json")

	body, err := json.Marshal(payload)
	if err != nil {
		slog.Error("response: failed to marshal JSON", slog.String("error", err.Error()))
		w.WriteHeader(http.StatusInternalServerError)
		fallback := APIResponse{
			Success: false,
			Code:    CodeInternalError,
			Message: InternalServerErrorMessage,
			Data:    nil,
		}
		fallbackBytes, _ := json.Marshal(fallback)
		_, _ = w.Write(append(fallbackBytes, '\n'))
		return
	}

	w.WriteHeader(status)
	if _, err := w.Write(append(body, '\n')); err != nil {
		slog.Error("response: failed to write response body", slog.String("error", err.Error()))
	}
}
