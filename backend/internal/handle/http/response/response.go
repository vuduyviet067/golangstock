package response

import (
	"encoding/json"
	"log/slog"
	"net/http"

	"gostock/internal/dto"
)

// InternalServerErrorMessage is the unified safe message for HTTP 500 responses.
const InternalServerErrorMessage = "Internal server error"

// Success writes a standard success response with the given status code, message, and data.
func Success(w http.ResponseWriter, status int, message string, data any) {
	write(w, status, dto.APIResponse{
		Success: true,
		Message: message,
		Data:    data,
	})
}

// Created writes a standard HTTP 201 Created response with the given message and payload.
func Created(w http.ResponseWriter, message string, data any) {
	Success(w, http.StatusCreated, message, data)
}

// Error writes a standard error response with the given status code and message.
// The data field is explicitly set to nil and serializes as null.
func Error(w http.ResponseWriter, status int, message string) {
	write(w, status, dto.APIResponse{
		Success: false,
		Message: message,
		Data:    nil,
	})
}

// ValidationError writes a standard HTTP 400 Bad Request error response with field-level details in data.
func ValidationError(w http.ResponseWriter, message string, fields map[string]string) {
	write(w, http.StatusBadRequest, dto.APIResponse{
		Success: false,
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
	Success(w, http.StatusOK, message, dto.PaginatedData{
		Items: items,
		Pagination: dto.PaginationMeta{
			Page:       page,
			Limit:      limit,
			Total:      total,
			TotalPages: totalPages,
		},
	})
}

// JSON writes any arbitrary payload as an application/json response.
func JSON(w http.ResponseWriter, status int, payload any) {
	write(w, status, payload)
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
		fallback := dto.APIResponse{
			Success: false,
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
