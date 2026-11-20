package response

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"regexp"
	"strings"
	"testing"
)

type sampleProduct struct {
	ID       int64  `json:"id"`
	SKU      string `json:"sku"`
	Name     string `json:"name"`
	Price    int64  `json:"price"`
	Quantity int    `json:"quantity"`
}

func TestSuccess(t *testing.T) {
	rec := httptest.NewRecorder()
	product := sampleProduct{
		ID:       1,
		SKU:      "KB-001",
		Name:     "Mechanical Keyboard",
		Price:    1200000,
		Quantity: 10,
	}

	Success(rec, http.StatusOK, "Product retrieved successfully", product)

	if rec.Code != http.StatusOK {
		t.Fatalf("expected status %d, got %d", http.StatusOK, rec.Code)
	}

	contentType := rec.Header().Get("Content-Type")
	if contentType != "application/json" {
		t.Fatalf("expected Content-Type application/json, got %q", contentType)
	}

	var res APIResponse
	if err := json.Unmarshal(rec.Body.Bytes(), &res); err != nil {
		t.Fatalf("failed to decode response JSON: %v", err)
	}

	if !res.Success {
		t.Errorf("expected success to be true, got false")
	}
	if res.Message != "Product retrieved successfully" {
		t.Errorf("expected message 'Product retrieved successfully', got %q", res.Message)
	}

	dataBytes, _ := json.Marshal(res.Data)
	var decodedProduct sampleProduct
	if err := json.Unmarshal(dataBytes, &decodedProduct); err != nil {
		t.Fatalf("failed to decode product data: %v", err)
	}
	if decodedProduct != product {
		t.Errorf("expected data %+v, got %+v", product, decodedProduct)
	}
}

func TestSuccess_CreatedWithNilData(t *testing.T) {
	rec := httptest.NewRecorder()
	Success(rec, http.StatusCreated, "Resource created", nil)

	if rec.Code != http.StatusCreated {
		t.Fatalf("expected status %d, got %d", http.StatusCreated, rec.Code)
	}

	bodyStr := strings.TrimSpace(rec.Body.String())
	expected := `{"success":true,"message":"Resource created","data":null}`
	if bodyStr != expected {
		t.Errorf("expected body %s, got %s", expected, bodyStr)
	}
}

func TestError(t *testing.T) {
	rec := httptest.NewRecorder()
	Error(rec, http.StatusNotFound, "PRODUCT_NOT_FOUND", "Product not found")

	if rec.Code != http.StatusNotFound {
		t.Fatalf("expected status %d, got %d", http.StatusNotFound, rec.Code)
	}

	contentType := rec.Header().Get("Content-Type")
	if contentType != "application/json" {
		t.Fatalf("expected Content-Type application/json, got %q", contentType)
	}

	bodyStr := strings.TrimSpace(rec.Body.String())
	expected := `{"success":false,"code":"PRODUCT_NOT_FOUND","message":"Product not found","data":null}`
	if bodyStr != expected {
		t.Errorf("expected body %s, got %s", expected, bodyStr)
	}

	var res APIResponse
	if err := json.Unmarshal(rec.Body.Bytes(), &res); err != nil {
		t.Fatalf("failed to decode response JSON: %v", err)
	}

	if res.Success {
		t.Errorf("expected success to be false, got true")
	}
	if res.Message != "Product not found" {
		t.Errorf("expected message 'Product not found', got %q", res.Message)
	}
	if res.Data != nil {
		t.Errorf("expected data to be nil, got %v", res.Data)
	}
}

func TestNoContent(t *testing.T) {
	rec := httptest.NewRecorder()
	NoContent(rec)

	if rec.Code != http.StatusNoContent {
		t.Fatalf("expected status %d, got %d", http.StatusNoContent, rec.Code)
	}
	if rec.Body.Len() != 0 {
		t.Errorf("expected empty body, got %q", rec.Body.String())
	}
}

func TestMarshalErrorFallback(t *testing.T) {
	rec := httptest.NewRecorder()
	// Functions cannot be serialized to JSON
	unmarshalable := make(chan int)

	Success(rec, http.StatusOK, "Will fail", unmarshalable)

	if rec.Code != http.StatusInternalServerError {
		t.Fatalf("expected fallback status %d, got %d", http.StatusInternalServerError, rec.Code)
	}

	bodyStr := strings.TrimSpace(rec.Body.String())
	expected := `{"success":false,"code":"INTERNAL_ERROR","message":"` + InternalServerErrorMessage + `","data":null}`
	if bodyStr != expected {
		t.Errorf("expected body %s, got %s", expected, bodyStr)
	}
}

func TestHeaderPreservation(t *testing.T) {
	rec := httptest.NewRecorder()
	rec.Header().Set("Allow", "GET, POST")
	rec.Header().Set("X-Custom-Header", "present")

	Error(rec, http.StatusMethodNotAllowed, CodeMethodNotAllowed, "method not allowed")

	if rec.Code != http.StatusMethodNotAllowed {
		t.Fatalf("expected status %d, got %d", http.StatusMethodNotAllowed, rec.Code)
	}
	if rec.Header().Get("Allow") != "GET, POST" {
		t.Errorf("expected Allow header preserved, got %q", rec.Header().Get("Allow"))
	}
	if rec.Header().Get("X-Custom-Header") != "present" {
		t.Errorf("expected X-Custom-Header preserved, got %q", rec.Header().Get("X-Custom-Header"))
	}
	if rec.Header().Get("Content-Type") != "application/json" {
		t.Errorf("expected Content-Type application/json")
	}
}

func TestCreated(t *testing.T) {
	rec := httptest.NewRecorder()
	item := map[string]any{"id": 123, "name": "Item"}
	Created(rec, "Resource created successfully", item)

	if rec.Code != http.StatusCreated {
		t.Fatalf("expected status %d, got %d", http.StatusCreated, rec.Code)
	}

	var res APIResponse
	if err := json.Unmarshal(rec.Body.Bytes(), &res); err != nil {
		t.Fatalf("failed to decode response: %v", err)
	}
	if !res.Success {
		t.Error("expected success=true")
	}
	if res.Message != "Resource created successfully" {
		t.Errorf("expected message 'Resource created successfully', got %q", res.Message)
	}
}

func TestValidationError(t *testing.T) {
	rec := httptest.NewRecorder()
	fields := map[string]string{
		"email":    "invalid email",
		"password": "password too short",
	}

	ValidationError(rec, CodeValidationError, "Validation failed", fields)

	if rec.Code != http.StatusBadRequest {
		t.Fatalf("expected status %d, got %d", http.StatusBadRequest, rec.Code)
	}

	var res APIResponse
	if err := json.Unmarshal(rec.Body.Bytes(), &res); err != nil {
		t.Fatalf("failed to decode response: %v", err)
	}
	if res.Success {
		t.Error("expected success=false")
	}
	if res.Code != CodeValidationError {
		t.Errorf("expected code %q, got %q", CodeValidationError, res.Code)
	}
	if res.Message != "Validation failed" {
		t.Errorf("expected message 'Validation failed', got %q", res.Message)
	}

	dataMap, ok := res.Data.(map[string]any)
	if !ok {
		t.Fatalf("expected data to be map, got %T", res.Data)
	}
	fieldsMap, ok := dataMap["fields"].(map[string]any)
	if !ok {
		t.Fatalf("expected fields to be map, got %T", dataMap["fields"])
	}
	if fieldsMap["email"] != "invalid email" || fieldsMap["password"] != "password too short" {
		t.Errorf("unexpected fields content: %+v", fieldsMap)
	}
}

func TestPaginated(t *testing.T) {
	rec := httptest.NewRecorder()
	items := []string{"item1", "item2"}

	Paginated(rec, "Items retrieved", items, 1, 10, 25)

	if rec.Code != http.StatusOK {
		t.Fatalf("expected status %d, got %d", http.StatusOK, rec.Code)
	}

	var res APIResponse
	if err := json.Unmarshal(rec.Body.Bytes(), &res); err != nil {
		t.Fatalf("failed to decode response: %v", err)
	}
	if !res.Success {
		t.Error("expected success=true")
	}

	dataBytes, _ := json.Marshal(res.Data)
	var paginated PaginatedData
	if err := json.Unmarshal(dataBytes, &paginated); err != nil {
		t.Fatalf("failed to decode paginated data: %v", err)
	}
	if paginated.Pagination.Page != 1 || paginated.Pagination.Limit != 10 || paginated.Pagination.Total != 25 || paginated.Pagination.TotalPages != 3 {
		t.Errorf("unexpected pagination meta: %+v", paginated.Pagination)
	}
}

// ---------------------------------------------------------------------------
// The code field.
// ---------------------------------------------------------------------------

// decodeRaw returns the top-level members of the response body.
func decodeRaw(t *testing.T, rec *httptest.ResponseRecorder) map[string]json.RawMessage {
	t.Helper()
	var raw map[string]json.RawMessage
	if err := json.Unmarshal(rec.Body.Bytes(), &raw); err != nil {
		t.Fatalf("decode %s: %v", rec.Body.String(), err)
	}
	return raw
}

func TestCode_PresentOnErrorsAbsentOnSuccess(t *testing.T) {
	cases := []struct {
		name     string
		write    func(w http.ResponseWriter)
		wantCode string // "" means the member must be absent
	}{
		{"Success", func(w http.ResponseWriter) { Success(w, http.StatusOK, "ok", map[string]int{"n": 1}) }, ""},
		{"Success with nil data", func(w http.ResponseWriter) { Success(w, http.StatusOK, "ok", nil) }, ""},
		{"Created", func(w http.ResponseWriter) { Created(w, "created", nil) }, ""},
		{"Paginated", func(w http.ResponseWriter) { Paginated(w, "list", []int{}, 1, 10, 0) }, ""},
		{"Error", func(w http.ResponseWriter) { Error(w, http.StatusConflict, "SKU_EXISTS", "SKU already exists") }, "SKU_EXISTS"},
		{"ValidationError", func(w http.ResponseWriter) {
			ValidationError(w, CodeValidationError, "invalid", map[string]string{"f": "bad"})
		}, CodeValidationError},
		{"marshal fallback", func(w http.ResponseWriter) { Success(w, http.StatusOK, "x", make(chan int)) }, CodeInternalError},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			rec := httptest.NewRecorder()
			tc.write(rec)
			raw := decodeRaw(t, rec)
			got, present := raw["code"]
			if tc.wantCode == "" {
				if present {
					t.Fatalf("successful response has a code member: %s", rec.Body.String())
				}
				return
			}
			if !present {
				t.Fatalf("error response has no code member: %s", rec.Body.String())
			}
			if want := `"` + tc.wantCode + `"`; string(got) != want {
				t.Errorf("code = %s, want %s", got, want)
			}
		})
	}
}

func TestCode_EnvelopeMemberOrder(t *testing.T) {
	rec := httptest.NewRecorder()
	Error(rec, http.StatusUnauthorized, CodeUnauthenticated, "authentication required")
	want := `{"success":false,"code":"UNAUTHENTICATED","message":"authentication required","data":null}`
	if got := strings.TrimSpace(rec.Body.String()); got != want {
		t.Errorf("body = %s, want %s", got, want)
	}
}

func TestCode_EmptyCodeFallsBackToStatus(t *testing.T) {
	cases := []struct {
		status int
		want   string
	}{
		{http.StatusBadRequest, CodeValidationError},
		{http.StatusUnauthorized, CodeUnauthenticated},
		{http.StatusForbidden, CodeForbidden},
		{http.StatusNotFound, CodeNotFound},
		{http.StatusMethodNotAllowed, CodeMethodNotAllowed},
		{http.StatusConflict, CodeConflict},
		{http.StatusUnsupportedMediaType, CodeUnsupportedMediaType},
		{http.StatusInternalServerError, CodeInternalError},
		{http.StatusServiceUnavailable, CodeServiceUnavailable},
		{http.StatusBadGateway, CodeInternalError},
		{http.StatusTeapot, "CLIENT_ERROR"},
	}
	for _, tc := range cases {
		rec := httptest.NewRecorder()
		Error(rec, tc.status, "", "message")
		var res APIResponse
		if err := json.Unmarshal(rec.Body.Bytes(), &res); err != nil {
			t.Fatal(err)
		}
		if res.Code != tc.want {
			t.Errorf("status %d: code = %q, want %q", tc.status, res.Code, tc.want)
		}
		if got := CodeForStatus(tc.status); got != tc.want {
			t.Errorf("CodeForStatus(%d) = %q, want %q", tc.status, got, tc.want)
		}
	}

	rec := httptest.NewRecorder()
	ValidationError(rec, "", "invalid", map[string]string{"f": "bad"})
	var res APIResponse
	if err := json.Unmarshal(rec.Body.Bytes(), &res); err != nil {
		t.Fatal(err)
	}
	if res.Code != CodeValidationError {
		t.Errorf("ValidationError with empty code: code = %q, want %q", res.Code, CodeValidationError)
	}
}

func TestCode_GeneralCodesAreUpperSnake(t *testing.T) {
	upperSnake := regexp.MustCompile(`^[A-Z][A-Z0-9]*(_[A-Z0-9]+)*$`)
	for _, code := range []string{CodeValidationError, CodeUnauthenticated, CodeForbidden, CodeNotFound,
		CodeMethodNotAllowed, CodeConflict, CodeUnsupportedMediaType, CodeInternalError, CodeServiceUnavailable,
		CodeForStatus(http.StatusTeapot)} {
		if !upperSnake.MatchString(code) {
			t.Errorf("code %q is not UPPER_SNAKE", code)
		}
	}
}

// The logout body pinned by the contract tests, built with the envelope. The
// mapper and use case tests asserted it with dto.APIResponse before the
// envelope moved here.
func TestSuccess_LogoutEnvelope(t *testing.T) {
	got, err := json.Marshal(APIResponse{Success: true, Message: "logged out", Data: map[string]string{"message": "logged out"}})
	if err != nil {
		t.Fatal(err)
	}
	if want := `{"success":true,"message":"logged out","data":{"message":"logged out"}}`; string(got) != want {
		t.Errorf("JSON mismatch\n got: %s\nwant: %s", got, want)
	}
}
