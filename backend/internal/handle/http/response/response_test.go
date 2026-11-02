package response

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
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

	var res Response
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
	Error(rec, http.StatusNotFound, "Product not found")

	if rec.Code != http.StatusNotFound {
		t.Fatalf("expected status %d, got %d", http.StatusNotFound, rec.Code)
	}

	contentType := rec.Header().Get("Content-Type")
	if contentType != "application/json" {
		t.Fatalf("expected Content-Type application/json, got %q", contentType)
	}

	bodyStr := strings.TrimSpace(rec.Body.String())
	expected := `{"success":false,"message":"Product not found","data":null}`
	if bodyStr != expected {
		t.Errorf("expected body %s, got %s", expected, bodyStr)
	}

	var res Response
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

func TestJSON(t *testing.T) {
	rec := httptest.NewRecorder()
	payload := map[string]string{"custom": "value"}
	JSON(rec, http.StatusAccepted, payload)

	if rec.Code != http.StatusAccepted {
		t.Fatalf("expected status %d, got %d", http.StatusAccepted, rec.Code)
	}
	if rec.Header().Get("Content-Type") != "application/json" {
		t.Fatalf("expected Content-Type application/json")
	}

	var result map[string]string
	if err := json.Unmarshal(rec.Body.Bytes(), &result); err != nil {
		t.Fatalf("failed to decode JSON: %v", err)
	}
	if result["custom"] != "value" {
		t.Errorf("expected custom=value, got %v", result)
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
	expected := `{"success":false,"message":"internal server error","data":null}`
	if bodyStr != expected {
		t.Errorf("expected body %s, got %s", expected, bodyStr)
	}
}

func TestHeaderPreservation(t *testing.T) {
	rec := httptest.NewRecorder()
	rec.Header().Set("Allow", "GET, POST")
	rec.Header().Set("X-Custom-Header", "present")

	Error(rec, http.StatusMethodNotAllowed, "method not allowed")

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
