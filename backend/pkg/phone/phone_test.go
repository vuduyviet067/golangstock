package phone_test

import (
	"errors"
	"testing"

	"gostock/pkg/phone"
)

func TestNormalize(t *testing.T) {
	valid := []struct {
		name, raw, want string
	}{
		{"đã chuẩn", "0368122137", "0368122137"},
		{"có dấu cách", "0368 122 137", "0368122137"},
		{"có dấu chấm", "0368.122.137", "0368122137"},
		{"có gạch ngang", "0368-122-137", "0368122137"},
		{"dấu cách hai đầu", "  0368122137  ", "0368122137"},
		{"+84 có dấu cách", "+84 368 122 137", "0368122137"},
		{"+84 liền", "+84368122137", "0368122137"},
		{"84 không có dấu +", "84368122137", "0368122137"},
		{"số cố định 11 chữ số", "(0222) 3123 456", "02223123456"},
	}
	for _, tt := range valid {
		t.Run(tt.name, func(t *testing.T) {
			got, err := phone.Normalize(tt.raw)
			if err != nil {
				t.Fatalf("Normalize(%q) lỗi %v", tt.raw, err)
			}
			if got != tt.want {
				t.Fatalf("Normalize(%q) = %q, mong đợi %q", tt.raw, got, tt.want)
			}
		})
	}

	invalid := []struct {
		name, raw string
	}{
		{"rỗng", ""},
		{"quá ngắn", "12345"},
		{"có chữ cái", "0368122137a"},
		{"+84 rồi vẫn có số 0", "+84 0368122137"},
		{"mã nước khác", "+1 202 555 0123"},
		{"12 chữ số", "036812213799"},
		{"chữ số thứ hai là 0", "0036812213"},
		{"chỉ có +84", "+84"},
	}
	for _, tt := range invalid {
		t.Run(tt.name, func(t *testing.T) {
			got, err := phone.Normalize(tt.raw)
			if !errors.Is(err, phone.ErrInvalid) {
				t.Fatalf("Normalize(%q) = %q, %v; mong đợi lỗi ErrInvalid", tt.raw, got, err)
			}
		})
	}
}
