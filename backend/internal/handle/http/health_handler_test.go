package http

import (
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

// GET /health is a liveness probe: it reports that the HTTP process answers
// and says nothing about PostgreSQL. HealthHandler has no dependencies, so
// these tests run without any database.

func TestHealthHandler_Liveness(t *testing.T) {
	rec := httptest.NewRecorder()
	HealthHandler(rec, httptest.NewRequest(http.MethodGet, "/health", nil))

	contractAssertResponse(t, rec, http.StatusOK,
		`{"success":true,"message":"health check passed","data":{"app":"GoStock","status":"ok"}}`)
	if strings.Contains(rec.Body.String(), "database") {
		t.Errorf("liveness body must not claim a database state: %s", rec.Body.String())
	}
}

func TestHealthHandler_RejectsOtherMethods(t *testing.T) {
	for _, method := range []string{http.MethodPost, http.MethodPut, http.MethodDelete} {
		t.Run(method, func(t *testing.T) {
			rec := httptest.NewRecorder()
			HealthHandler(rec, httptest.NewRequest(method, "/health", nil))

			contractAssertResponse(t, rec, http.StatusMethodNotAllowed,
				`{"success":false,"message":"method not allowed","data":null}`)
			if got := rec.Header().Get("Allow"); got != http.MethodGet {
				t.Errorf("Allow = %q, want %q", got, http.MethodGet)
			}
		})
	}
}
