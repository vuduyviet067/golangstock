package response

import (
	"encoding/json"
	"log"
	"net/http"
)

// Response represents the standard unified API response envelope.
type Response struct {
	Success bool   `json:"success"`
	Message string `json:"message"`
	Data    any    `json:"data"`
}

// Success writes a standard success response with the given status code, message, and data.
func Success(w http.ResponseWriter, status int, message string, data any) {
	write(w, status, Response{
		Success: true,
		Message: message,
		Data:    data,
	})
}

// Error writes a standard error response with the given status code and message.
// The data field is explicitly set to nil and serializes as null.
func Error(w http.ResponseWriter, status int, message string) {
	write(w, status, Response{
		Success: false,
		Message: message,
		Data:    nil,
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
		log.Printf("response: failed to marshal JSON: %v", err)
		w.WriteHeader(http.StatusInternalServerError)
		fallback := Response{
			Success: false,
			Message: "internal server error",
			Data:    nil,
		}
		fallbackBytes, _ := json.Marshal(fallback)
		_, _ = w.Write(append(fallbackBytes, '\n'))
		return
	}

	w.WriteHeader(status)
	if _, err := w.Write(append(body, '\n')); err != nil {
		log.Printf("response: failed to write response body: %v", err)
	}
}
