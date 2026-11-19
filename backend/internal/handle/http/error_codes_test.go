package http

import (
	"errors"
	"fmt"
	"net/http"
	"net/http/httptest"
	"regexp"
	"strings"
	"testing"

	"gostock/internal/dto"
	"gostock/internal/usecase"
	"gostock/pkg/response"
)

// mappedCodes pins the error code of every use case error MapError knows.
var mappedCodes = []struct {
	err  error
	code string
}{
	{usecase.ErrInvalidID, "INVALID_PRODUCT_ID"},
	{usecase.ErrInvalidUserID, "INVALID_USER_ID"},
	{usecase.ErrInvalidUserRole, "INVALID_USER_ROLE"},
	{usecase.ErrEmptyUpdate, "EMPTY_UPDATE"},
	{usecase.ErrInvalidUser, "INVALID_USER"},
	{usecase.ErrInvalidPage, "INVALID_PAGE"},
	{usecase.ErrInvalidLimit, "INVALID_LIMIT"},
	{usecase.ErrInvalidProduct, "INVALID_PRODUCT"},
	{usecase.ErrInvalidQuantity, "INVALID_QUANTITY"},
	{usecase.ErrInvalidEmail, "INVALID_EMAIL"},
	{usecase.ErrInvalidPassword, "INVALID_PASSWORD"},
	{usecase.ErrInvalidChartFrom, "VALIDATION_ERROR"},
	{usecase.ErrInvalidChartTo, "VALIDATION_ERROR"},
	{usecase.ErrChartRangeReversed, "VALIDATION_ERROR"},
	{usecase.ErrChartRangeTooLong, "VALIDATION_ERROR"},
	{usecase.ErrInvalidGranularity, "VALIDATION_ERROR"},
	{usecase.ErrInvalidTimezone, "VALIDATION_ERROR"},
	{usecase.ErrInvalidCredentials, "INVALID_CREDENTIALS"},
	{usecase.ErrInvalidSession, "INVALID_SESSION"},
	{usecase.ErrInvalidActor, "UNAUTHENTICATED"},
	{usecase.ErrInvalidRefreshToken, "INVALID_REFRESH_TOKEN"},
	{usecase.ErrRefreshTokenNotFound, "INVALID_REFRESH_TOKEN"},
	{usecase.ErrRefreshTokenRevoked, "INVALID_REFRESH_TOKEN"},
	{usecase.ErrRefreshTokenExpired, "INVALID_REFRESH_TOKEN"},
	{usecase.ErrRefreshTokenReused, "INVALID_REFRESH_TOKEN"},
	{usecase.ErrProductNotFound, "PRODUCT_NOT_FOUND"},
	{usecase.ErrUserNotFound, "USER_NOT_FOUND"},
	{usecase.ErrConcurrentRefresh, "CONCURRENT_REFRESH"},
	{usecase.ErrProductSKUExists, "SKU_EXISTS"},
	{usecase.ErrEmailExists, "EMAIL_EXISTS"},
	{usecase.ErrInsufficientStock, "INSUFFICIENT_STOCK"},
	{usecase.ErrInvalidTokenTTL, "INTERNAL_ERROR"},
	{errors.New("pq: connection refused"), "INTERNAL_ERROR"},
}

func TestMapError_Codes(t *testing.T) {
	for _, tc := range mappedCodes {
		t.Run(tc.err.Error(), func(t *testing.T) {
			if got := MapError(tc.err).Code; got != tc.code {
				t.Errorf("Code = %q, want %q", got, tc.code)
			}
			wrapped := fmt.Errorf("layer: %w", tc.err)
			if got := MapError(wrapped).Code; got != tc.code {
				t.Errorf("wrapped: Code = %q, want %q", got, tc.code)
			}
		})
	}
}

func TestMapError_CodesAreUpperSnake(t *testing.T) {
	upperSnake := regexp.MustCompile(`^[A-Z][A-Z0-9]*(_[A-Z0-9]+)*$`)
	for _, tc := range mappedCodes {
		if code := MapError(tc.err).Code; !upperSnake.MatchString(code) {
			t.Errorf("%v: code %q is not UPPER_SNAKE", tc.err, code)
		}
	}
}

func TestWriteError_BodyHasCode(t *testing.T) {
	tests := []struct {
		name       string
		err        error
		wantStatus int
		wantBody   string
	}{
		{"specific code", usecase.ErrInsufficientStock, http.StatusConflict,
			`{"success":false,"code":"INSUFFICIENT_STOCK","message":"Insufficient stock","data":null}`},
		{"validation with fields", usecase.ErrInvalidTimezone, http.StatusBadRequest,
			`{"success":false,"code":"VALIDATION_ERROR","message":"invalid query parameters","data":{"fields":{"tz":"must be an IANA time zone name"}}}`},
		{"internal error", errors.New("boom"), http.StatusInternalServerError,
			`{"success":false,"code":"INTERNAL_ERROR","message":"Internal server error","data":null}`},
		{"HTTPError without code gets the general code of its status",
			&dto.HTTPError{StatusCode: http.StatusNotFound, Message: "gone"}, http.StatusNotFound,
			`{"success":false,"code":"NOT_FOUND","message":"gone","data":null}`},
		{"HTTPError with an unlisted status",
			&dto.HTTPError{StatusCode: http.StatusTeapot, Message: "I'm a teapot"}, http.StatusTeapot,
			`{"success":false,"code":"CLIENT_ERROR","message":"I'm a teapot","data":null}`},
		{"HTTPError keeps its own code",
			&dto.HTTPError{StatusCode: http.StatusConflict, Code: "CUSTOM_CONFLICT", Message: "busy"}, http.StatusConflict,
			`{"success":false,"code":"CUSTOM_CONFLICT","message":"busy","data":null}`},
		{"validation HTTPError without code",
			NewValidationError("invalid", map[string]string{"f": "bad"}, nil), http.StatusBadRequest,
			`{"success":false,"code":"VALIDATION_ERROR","message":"invalid","data":{"fields":{"f":"bad"}}}`},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			rec := httptest.NewRecorder()
			WriteError(rec, httptest.NewRequest(http.MethodGet, "/", nil), tt.err)
			if rec.Code != tt.wantStatus {
				t.Errorf("status = %d, want %d", rec.Code, tt.wantStatus)
			}
			if got := strings.TrimSpace(rec.Body.String()); got != tt.wantBody {
				t.Errorf("body = %s, want %s", got, tt.wantBody)
			}
		})
	}
}

// Handlers that report an error themselves use the same code as MapError for
// the same message.
func TestHandlerCodesMatchMapError(t *testing.T) {
	pairs := []struct {
		handlerCode string
		err         error
	}{
		{codeInvalidProductID, usecase.ErrInvalidID},
		{codeInvalidUserID, usecase.ErrInvalidUserID},
		{codeInvalidPage, usecase.ErrInvalidPage},
		{codeInvalidLimit, usecase.ErrInvalidLimit},
		{response.CodeUnauthenticated, usecase.ErrInvalidActor},
	}
	for _, p := range pairs {
		if got := MapError(p.err).Code; got != p.handlerCode {
			t.Errorf("%v: MapError code %q, handler code %q", p.err, got, p.handlerCode)
		}
	}
}
