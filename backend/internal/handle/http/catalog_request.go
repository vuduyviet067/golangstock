package http

import (
	"encoding/json"
	"io"
	"net/http"
	"strconv"
	"strings"

	"gostock/pkg/response"
)

// Request helpers of the catalog handlers (wood types, suppliers).

// catalogID reads the positive integer path parameter "id". Otherwise it
// writes 400 with code and message and returns false.
func catalogID(w http.ResponseWriter, r *http.Request, code, message string) (int64, bool) {
	rawID := pathParam(r, "id")
	if rawID == "" || strings.Contains(rawID, "/") {
		response.Error(w, http.StatusBadRequest, code, message)
		return 0, false
	}
	id, err := strconv.ParseInt(rawID, 10, 64)
	if err != nil || id <= 0 {
		response.Error(w, http.StatusBadRequest, code, message)
		return 0, false
	}
	return id, true
}

// decodeJSONObject decodes a body of exactly one JSON object without unknown
// fields into dst. Otherwise it writes 415 (not application/json) or 400 and
// returns false.
func decodeJSONObject(w http.ResponseWriter, r *http.Request, dst any) bool {
	if !EnforceJSONContentType(w, r) {
		return false
	}
	r.Body = http.MaxBytesReader(w, r.Body, 1<<20)

	decoder := json.NewDecoder(r.Body)
	decoder.DisallowUnknownFields()
	if err := decoder.Decode(dst); err != nil {
		response.Error(w, http.StatusBadRequest, response.CodeValidationError, "invalid JSON request")
		return false
	}
	var extra any
	if err := decoder.Decode(&extra); err != io.EOF {
		response.Error(w, http.StatusBadRequest, response.CodeValidationError, "request must contain exactly one JSON object")
		return false
	}
	return true
}
