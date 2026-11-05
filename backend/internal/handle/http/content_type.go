package http

import (
	"mime"
	"net/http"

	"gostock/internal/handle/http/response"
)

const jsonMediaType = "application/json"

// EnforceJSONContentType verifies that the request contains a Content-Type header
// specifying application/json (with optional parameters like charset=utf-8).
// If the Content-Type is missing, malformed, or not application/json, it responds
// with HTTP 415 Unsupported Media Type using the standard error response envelope
// and returns false. Otherwise, it returns true.
func EnforceJSONContentType(w http.ResponseWriter, r *http.Request) bool {
	ct := r.Header.Get("Content-Type")
	if ct == "" {
		response.Error(w, http.StatusUnsupportedMediaType, "Content-Type must be application/json")
		return false
	}

	mediaType, _, err := mime.ParseMediaType(ct)
	if err != nil || mediaType != jsonMediaType {
		response.Error(w, http.StatusUnsupportedMediaType, "Content-Type must be application/json")
		return false
	}

	return true
}
