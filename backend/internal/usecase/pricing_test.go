package usecase

import (
	"errors"
	"math"
	"testing"

	"gostock/internal/domain"
)

func TestLineTotal(t *testing.T) {
	tests := []struct {
		name    string
		volume  domain.Volume
		price   domain.Money
		want    domain.Money
		wantErr error
	}{
		{"ví dụ đặc tả: 0,856 m³ × 35 triệu", 856, 35_000_000, 29_960_000, nil},
		{"0,001 m³ × 1.000 đ", 1, 1_000, 1, nil},
		{"0,499 đ làm tròn xuống", 1, 499, 0, nil},
		{"0,5 đ làm tròn lên", 1, 500, 1, nil},
		{"phần lẻ ,652 làm tròn lên", 1_234, 12_345_678, 15_234_567, nil},
		{"đơn giá 0", 856, 0, 0, nil},
		{"số khối 0", 0, 35_000_000, 0, ErrInvalidVolume},
		{"số khối âm", -1, 35_000_000, 0, ErrInvalidVolume},
		{"đơn giá âm", 856, -1, 0, ErrInvalidPrice},
		{"tràn int64", 1_000_000_000, 10_000_000_000, 0, ErrAmountOverflow},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := lineTotal(tt.volume, tt.price)
			if !errors.Is(err, tt.wantErr) {
				t.Fatalf("lỗi = %v, mong đợi %v", err, tt.wantErr)
			}
			if got != tt.want {
				t.Fatalf("kết quả = %d, mong đợi %d", got, tt.want)
			}
		})
	}
}

func TestRoundInvoiceTotal(t *testing.T) {
	tests := []struct {
		name    string
		sum     domain.Money
		want    domain.Money
		wantErr error
	}{
		{"ví dụ đặc tả", 51_234_567, 51_230_000, nil},
		{"dư 4.999 làm tròn xuống", 12_344_999, 12_340_000, nil},
		{"dư 5.000 làm tròn lên", 12_345_000, 12_350_000, nil},
		{"đã tròn sẵn", 29_960_000, 29_960_000, nil},
		{"0 đồng", 0, 0, nil},
		{"4.999 đ thành 0", 4_999, 0, nil},
		{"9.999 đ thành 10.000", 9_999, 10_000, nil},
		{"số âm", -1, 0, ErrInvalidAmount},
		{"làm tròn lên bị tràn", math.MaxInt64, 0, ErrAmountOverflow},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := roundInvoiceTotal(tt.sum)
			if !errors.Is(err, tt.wantErr) {
				t.Fatalf("lỗi = %v, mong đợi %v", err, tt.wantErr)
			}
			if got != tt.want {
				t.Fatalf("kết quả = %d, mong đợi %d", got, tt.want)
			}
		})
	}
}
