package http

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"gostock/internal/domain"
	"gostock/internal/handle/http/response"
	"gostock/internal/usecase"
)

func TestMapError_KnownDomainAndUseCaseErrors(t *testing.T) {
	tests := []struct {
		name        string
		inputErr    error
		wantStatus  int
		wantMessage string
	}{
		// 400 Bad Request
		{"ErrInvalidID", usecase.ErrInvalidID, http.StatusBadRequest, "invalid product ID"},
		{"ErrInvalidUserID", usecase.ErrInvalidUserID, http.StatusBadRequest, "invalid user ID"},
		{"ErrInvalidUserRole", usecase.ErrInvalidUserRole, http.StatusBadRequest, "invalid user role"},
		{"ErrEmptyUpdate", usecase.ErrEmptyUpdate, http.StatusBadRequest, "at least one field must be provided for update"},
		{"ErrInvalidUser", usecase.ErrInvalidUser, http.StatusBadRequest, "invalid user"},
		{"ErrInvalidPage", usecase.ErrInvalidPage, http.StatusBadRequest, "invalid page"},
		{"ErrInvalidLimit", usecase.ErrInvalidLimit, http.StatusBadRequest, "invalid limit"},
		{"ErrInvalidProduct", usecase.ErrInvalidProduct, http.StatusBadRequest, "Invalid product"},
		{"ErrInvalidQuantity", usecase.ErrInvalidQuantity, http.StatusBadRequest, "quantity must be greater than 0"},
		{"ErrInvalidEmail", usecase.ErrInvalidEmail, http.StatusBadRequest, "invalid email"},
		{"ErrInvalidPassword", usecase.ErrInvalidPassword, http.StatusBadRequest, "invalid password"},

		// 401 Unauthorized
		{"ErrInvalidCredentials", usecase.ErrInvalidCredentials, http.StatusUnauthorized, "invalid email or password"},
		{"ErrInvalidSession", usecase.ErrInvalidSession, http.StatusUnauthorized, "invalid or expired session"},
		{"ErrInvalidRefreshToken", domain.ErrInvalidRefreshToken, http.StatusUnauthorized, "invalid or expired refresh token"},
		{"ErrRefreshTokenNotFound", domain.ErrRefreshTokenNotFound, http.StatusUnauthorized, "invalid or expired refresh token"},
		{"ErrRefreshTokenRevoked", domain.ErrRefreshTokenRevoked, http.StatusUnauthorized, "invalid or expired refresh token"},
		{"ErrRefreshTokenExpired", domain.ErrRefreshTokenExpired, http.StatusUnauthorized, "invalid or expired refresh token"},
		{"ErrRefreshTokenReused", domain.ErrRefreshTokenReused, http.StatusUnauthorized, "invalid or expired refresh token"},

		// 404 Not Found
		{"ErrProductNotFound", domain.ErrProductNotFound, http.StatusNotFound, "Product not found"},
		{"ErrUserNotFound", domain.ErrUserNotFound, http.StatusNotFound, "user not found"},

		// 409 Conflict
		{"ErrConcurrentRefresh", domain.ErrConcurrentRefresh, http.StatusConflict, "concurrent refresh in progress"},
		{"ErrProductSKUExists", domain.ErrProductSKUExists, http.StatusConflict, "SKU already exists"},
		{"ErrEmailExists", domain.ErrEmailExists, http.StatusConflict, "email already exists"},
		{"ErrInsufficientStock", domain.ErrInsufficientStock, http.StatusConflict, "Insufficient stock"},

		// 500 Internal Server Error (server configuration or infrastructure errors)
		{"ErrInvalidTokenTTL", usecase.ErrInvalidTokenTTL, http.StatusInternalServerError, "Internal server error"},
		{"generic error", errors.New("pq: connection refused"), http.StatusInternalServerError, "Internal server error"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			mapped := MapError(tt.inputErr)
			if mapped == nil {
				t.Fatalf("expected non-nil HTTPError for %v", tt.inputErr)
			}
			if mapped.StatusCode != tt.wantStatus {
				t.Errorf("StatusCode = %d, want %d", mapped.StatusCode, tt.wantStatus)
			}
			if mapped.Message != tt.wantMessage {
				t.Errorf("Message = %q, want %q", mapped.Message, tt.wantMessage)
			}
			if !errors.Is(mapped, tt.inputErr) {
				t.Errorf("expected errors.Is(mapped, inputErr) to be true for wrapped error")
			}
			if mapped.Unwrap() != tt.inputErr {
				t.Errorf("expected Unwrap() = %v, got %v", tt.inputErr, mapped.Unwrap())
			}
		})
	}
}

func TestMapError_WrappedErrors(t *testing.T) {
	wrapped := fmt.Errorf("service layer failed: %w", domain.ErrProductNotFound)
	mapped := MapError(wrapped)

	if mapped == nil {
		t.Fatal("expected non-nil mapped error")
	}
	if mapped.StatusCode != http.StatusNotFound {
		t.Errorf("StatusCode = %d, want %d", mapped.StatusCode, http.StatusNotFound)
	}
	if mapped.Message != "Product not found" {
		t.Errorf("Message = %q, want 'Product not found'", mapped.Message)
	}
	if !errors.Is(mapped, domain.ErrProductNotFound) {
		t.Error("expected errors.Is to unwrap to domain.ErrProductNotFound")
	}
}

func TestMapError_Nil(t *testing.T) {
	if MapError(nil) != nil {
		t.Error("expected MapError(nil) to return nil")
	}
}

func TestMapError_AlreadyHTTPError(t *testing.T) {
	custom := &HTTPError{
		StatusCode: http.StatusTeapot,
		Message:    "I'm a teapot",
		Cause:      errors.New("tea is brewing"),
	}

	mapped := MapError(custom)
	if mapped != custom {
		t.Errorf("expected mapped to be the exact same pointer %+v, got %+v", custom, mapped)
	}
}

func TestWriteError_ResponseEnvelope(t *testing.T) {
	t.Run("writes standard error envelope and preserves request context", func(t *testing.T) {
		rec := httptest.NewRecorder()
		req := httptest.NewRequest(http.MethodGet, "/test", nil)
		rawDBErr := errors.New("pq: relation 'secret_table' does not exist")

		WriteError(rec, req, rawDBErr)

		if rec.Code != http.StatusInternalServerError {
			t.Fatalf("status = %d, want %d", rec.Code, http.StatusInternalServerError)
		}
		if rec.Header().Get("Content-Type") != "application/json" {
			t.Errorf("Content-Type = %q, want application/json", rec.Header().Get("Content-Type"))
		}

		var resp response.Response
		if err := json.Unmarshal(rec.Body.Bytes(), &resp); err != nil {
			t.Fatalf("failed to decode JSON: %v", err)
		}

		if resp.Success {
			t.Error("expected success = false")
		}
		if resp.Message != "Internal server error" {
			t.Errorf("Message = %q, want 'Internal server error'", resp.Message)
		}
		if resp.Data != nil {
			t.Errorf("expected data = nil, got %v", resp.Data)
		}

		// Verify internal database error details were NOT leaked in the response body
		if body := rec.Body.String(); body == "" || (body != "" && (contains(body, "relation") || contains(body, "secret_table"))) {
			t.Errorf("raw DB error details leaked in response body: %s", body)
		}
	})

	t.Run("writes field-level validation error envelope", func(t *testing.T) {
		rec := httptest.NewRecorder()
		req := httptest.NewRequest(http.MethodPost, "/test", nil)
		valErr := NewValidationError("Validation failed", map[string]string{
			"sku":  "sku is required",
			"name": "name cannot be empty",
		}, errors.New("validation failed"))

		WriteError(rec, req, valErr)

		if rec.Code != http.StatusBadRequest {
			t.Fatalf("status = %d, want %d", rec.Code, http.StatusBadRequest)
		}

		var resp response.Response
		if err := json.Unmarshal(rec.Body.Bytes(), &resp); err != nil {
			t.Fatalf("failed to decode JSON: %v", err)
		}

		if resp.Success {
			t.Error("expected success = false")
		}
		if resp.Message != "Validation failed" {
			t.Errorf("Message = %q, want 'Validation failed'", resp.Message)
		}

		dataMap, ok := resp.Data.(map[string]any)
		if !ok {
			t.Fatalf("expected data to be map, got %T", resp.Data)
		}
		fields, ok := dataMap["fields"].(map[string]any)
		if !ok {
			t.Fatalf("expected fields to be map, got %T", dataMap["fields"])
		}
		if fields["sku"] != "sku is required" || fields["name"] != "name cannot be empty" {
			t.Errorf("unexpected field errors: %+v", fields)
		}
	})

	t.Run("nil error does not write to response", func(t *testing.T) {
		rec := httptest.NewRecorder()
		req := httptest.NewRequest(http.MethodGet, "/test", nil)
		WriteError(rec, req, nil)
		if rec.Code != http.StatusOK { // default unwritten recorder status
			t.Errorf("status = %d, want %d", rec.Code, http.StatusOK)
		}
		if rec.Body.Len() != 0 {
			t.Errorf("expected empty body, got %q", rec.Body.String())
		}
	})

	t.Run("nil request does not panic and uses background context", func(t *testing.T) {
		rec := httptest.NewRecorder()
		rawDBErr := errors.New("database timeout")
		WriteError(rec, nil, rawDBErr)

		if rec.Code != http.StatusInternalServerError {
			t.Fatalf("status = %d, want %d", rec.Code, http.StatusInternalServerError)
		}
	})
}

func TestWriteError_StructuredLogging(t *testing.T) {
	t.Run("valid request ID header is included in structured logs", func(t *testing.T) {
		var logBuf bytes.Buffer
		logger := slog.New(slog.NewJSONHandler(&logBuf, nil))
		oldDefault := slog.Default()
		slog.SetDefault(logger)
		defer slog.SetDefault(oldDefault)

		rec := httptest.NewRecorder()
		req := httptest.NewRequest(http.MethodGet, "/api/v1/products", nil)
		req.Header.Set("X-Request-ID", "req-test-999")

		rawDBErr := errors.New("pq: connection closed unexpectedly")
		WriteError(rec, req, rawDBErr)

		if rec.Code != http.StatusInternalServerError {
			t.Fatalf("status = %d, want 500", rec.Code)
		}

		logOutput := logBuf.String()
		if !strings.Contains(logOutput, `"level":"ERROR"`) {
			t.Errorf("expected log to contain level ERROR, got: %s", logOutput)
		}
		if !strings.Contains(logOutput, `"msg":"internal server error"`) {
			t.Errorf("expected log to contain msg 'internal server error', got: %s", logOutput)
		}
		if !strings.Contains(logOutput, `"request_id":"req-test-999"`) {
			t.Errorf("expected log to contain request_id 'req-test-999', got: %s", logOutput)
		}
		if !strings.Contains(logOutput, `"error":"pq: connection closed unexpectedly"`) {
			t.Errorf("expected log to contain root cause error, got: %s", logOutput)
		}
	})

	t.Run("missing request ID does not break logging", func(t *testing.T) {
		var logBuf bytes.Buffer
		logger := slog.New(slog.NewJSONHandler(&logBuf, nil))
		oldDefault := slog.Default()
		slog.SetDefault(logger)
		defer slog.SetDefault(oldDefault)

		rec := httptest.NewRecorder()
		req := httptest.NewRequest(http.MethodGet, "/api/v1/products", nil)

		rawDBErr := errors.New("pq: disk full")
		WriteError(rec, req, rawDBErr)

		if rec.Code != http.StatusInternalServerError {
			t.Fatalf("status = %d, want 500", rec.Code)
		}

		logOutput := logBuf.String()
		if !strings.Contains(logOutput, `"msg":"internal server error"`) {
			t.Errorf("expected log to contain msg 'internal server error', got: %s", logOutput)
		}
		if strings.Contains(logOutput, `"request_id"`) {
			t.Errorf("expected no request_id attribute when header is missing, got: %s", logOutput)
		}
		if !strings.Contains(logOutput, `"error":"pq: disk full"`) {
			t.Errorf("expected log to contain error, got: %s", logOutput)
		}
	})

	t.Run("overlong or invalid request ID is not included", func(t *testing.T) {
		testCases := []struct {
			name   string
			header string
		}{
			{"overlong (129 chars)", strings.Repeat("a", 129)},
			{"contains newline", "req-123\nmalicious-log-injection"},
			{"contains carriage return", "req-123\rmalicious"},
			{"contains null byte", "req-123\x00extra"},
			{"contains DEL character", "req-123\x7f"},
			{"contains tab control char", "req-123\ttab"},
			{"empty header", ""},
			{"whitespace only", "   "},
		}

		for _, tc := range testCases {
			t.Run(tc.name, func(t *testing.T) {
				var logBuf bytes.Buffer
				logger := slog.New(slog.NewJSONHandler(&logBuf, nil))
				oldDefault := slog.Default()
				slog.SetDefault(logger)
				defer slog.SetDefault(oldDefault)

				rec := httptest.NewRecorder()
				req := httptest.NewRequest(http.MethodGet, "/api/v1/products", nil)
				if tc.header != "" {
					req.Header.Set("X-Request-ID", tc.header)
				}

				rawDBErr := errors.New("pq: table lock timeout")
				WriteError(rec, req, rawDBErr)

				if rec.Code != http.StatusInternalServerError {
					t.Fatalf("status = %d, want 500", rec.Code)
				}

				logOutput := logBuf.String()
				if !strings.Contains(logOutput, `"msg":"internal server error"`) {
					t.Errorf("expected error log, got: %s", logOutput)
				}
				if strings.Contains(logOutput, `"request_id"`) {
					t.Errorf("expected invalid/overlong request_id to be omitted, got: %s", logOutput)
				}
			})
		}
	})

	t.Run("context cancellation does not cause a panic", func(t *testing.T) {
		var logBuf bytes.Buffer
		logger := slog.New(slog.NewJSONHandler(&logBuf, nil))
		oldDefault := slog.Default()
		slog.SetDefault(logger)
		defer slog.SetDefault(oldDefault)

		rec := httptest.NewRecorder()
		req := httptest.NewRequest(http.MethodGet, "/api/v1/products", nil)
		ctx, cancel := context.WithCancel(req.Context())
		cancel() // immediately cancel context
		req = req.WithContext(ctx)

		rawDBErr := errors.New("client connection dropped")
		WriteError(rec, req, rawDBErr)

		if rec.Code != http.StatusInternalServerError {
			t.Fatalf("status = %d, want 500", rec.Code)
		}
		if !strings.Contains(rec.Body.String(), response.InternalServerErrorMessage) {
			t.Errorf("expected response to contain generic message, got: %s", rec.Body.String())
		}
	})

	t.Run("HTTP 400 and 401 errors are not logged", func(t *testing.T) {
		var logBuf bytes.Buffer
		logger := slog.New(slog.NewJSONHandler(&logBuf, nil))
		oldDefault := slog.Default()
		slog.SetDefault(logger)
		defer slog.SetDefault(oldDefault)

		req := httptest.NewRequest(http.MethodGet, "/api/v1/products/invalid", nil)

		rec400 := httptest.NewRecorder()
		WriteError(rec400, req, usecase.ErrInvalidID)
		if rec400.Code != http.StatusBadRequest {
			t.Fatalf("status = %d, want 400", rec400.Code)
		}

		rec401 := httptest.NewRecorder()
		WriteError(rec401, req, usecase.ErrInvalidCredentials)
		if rec401.Code != http.StatusUnauthorized {
			t.Fatalf("status = %d, want 401", rec401.Code)
		}

		if logBuf.Len() != 0 {
			t.Errorf("expected no error logs for 400/401 client errors, got: %s", logBuf.String())
		}
	})

	t.Run("HTTP 500 errors are logged and internal details are not exposed in response", func(t *testing.T) {
		var logBuf bytes.Buffer
		logger := slog.New(slog.NewJSONHandler(&logBuf, nil))
		oldDefault := slog.Default()
		slog.SetDefault(logger)
		defer slog.SetDefault(oldDefault)

		rec := httptest.NewRecorder()
		req := httptest.NewRequest(http.MethodGet, "/api/v1/products", nil)
		rawDBErr := errors.New("pq: SELECT * FROM sensitive_schema.credentials FAILED")

		WriteError(rec, req, rawDBErr)

		if rec.Code != http.StatusInternalServerError {
			t.Fatalf("status = %d, want 500", rec.Code)
		}

		// Log must contain the error
		logOutput := logBuf.String()
		if !strings.Contains(logOutput, "sensitive_schema.credentials") {
			t.Errorf("expected internal error logged in slog, got: %s", logOutput)
		}

		// Response MUST NOT leak internal details
		respBody := rec.Body.String()
		if strings.Contains(respBody, "sensitive_schema") || strings.Contains(respBody, "credentials") || strings.Contains(respBody, "SELECT") {
			t.Errorf("sensitive internal error leaked in response: %s", respBody)
		}

		var resp response.Response
		if err := json.Unmarshal(rec.Body.Bytes(), &resp); err != nil {
			t.Fatalf("failed to decode response JSON: %v", err)
		}
		if resp.Success {
			t.Error("expected success = false")
		}
		if resp.Message != response.InternalServerErrorMessage {
			t.Errorf("Message = %q, want %q", resp.Message, response.InternalServerErrorMessage)
		}
		if resp.Data != nil {
			t.Errorf("Data = %v, want nil", resp.Data)
		}
	})
}

func contains(s, substr string) bool {
	return len(s) >= len(substr) && (s == substr || len(substr) == 0 || (len(s) > 0 && len(substr) > 0 && (func() bool {
		for i := 0; i+len(substr) <= len(s); i++ {
			if s[i:i+len(substr)] == substr {
				return true
			}
		}
		return false
	})()))
}
