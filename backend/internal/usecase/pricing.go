package usecase

import (
	"errors"
	"math"

	"gostock/internal/domain"
)

var (
	ErrInvalidVolume  = errors.New("volume must be greater than 0")
	ErrInvalidPrice   = errors.New("unit price must not be negative")
	ErrInvalidAmount  = errors.New("amount must not be negative")
	ErrAmountOverflow = errors.New("amount is too large")
)

const (
	// volumeScale: domain.Volume tính bằng 0,001 m³, nên 1 m³ = 1000.
	volumeScale = 1000
	// invoiceRoundingStep: tổng hoá đơn làm tròn đến 10.000 đ.
	invoiceRoundingStep = 10_000
)

// lineTotal tính thành tiền của một dòng hoá đơn: số khối × đơn giá.
//
// volume tính bằng 0,001 m³ nên volume × unitPrice lớn gấp 1000 lần số đồng
// thật; chia cho 1000 và làm tròn đến đồng, phần lẻ từ 0,5 đồng trở lên làm
// tròn lên. Ví dụ 856 (0,856 m³) × 35.000.000 đ = 29.960.000 đ.
func lineTotal(volume domain.Volume, unitPrice domain.Money) (domain.Money, error) {
	if volume <= 0 {
		return 0, ErrInvalidVolume
	}
	if unitPrice < 0 {
		return 0, ErrInvalidPrice
	}
	v, p := int64(volume), int64(unitPrice)
	const half = volumeScale / 2 // 500: nửa đơn vị, dùng để làm tròn

	// Kiểm tra tràn TRƯỚC khi nhân: phép tính bên dưới cần v*p + 500 ≤ MaxInt64,
	// tương đương v ≤ (MaxInt64 - 500) / p (với p > 0). Khi p = 0 thì không tràn.
	if p > 0 && v > (math.MaxInt64-half)/p {
		return 0, ErrAmountOverflow
	}

	// Làm tròn: cộng nửa đơn vị rồi chia lấy phần nguyên.
	// Ví dụ 0,5 đ → (500 + 500) / 1000 = 1; 0,499 đ → (499 + 500) / 1000 = 0.
	return domain.Money((v*p + half) / volumeScale), nil
}

// roundInvoiceTotal làm tròn tổng hoá đơn đến 10.000 đ gần nhất: phần dư dưới
// 5.000 đ bỏ đi, từ 5.000 đ trở lên làm tròn lên. Ví dụ 51.234.567 đ → 51.230.000 đ.
func roundInvoiceTotal(sum domain.Money) (domain.Money, error) {
	if sum < 0 {
		return 0, ErrInvalidAmount
	}
	s := int64(sum)
	remainder := s % invoiceRoundingStep // phần lẻ dưới 10.000 đ
	roundedDown := s - remainder

	if remainder < invoiceRoundingStep/2 {
		return domain.Money(roundedDown), nil
	}

	// Làm tròn lên: kiểm tra tràn trước khi cộng thêm 10.000.
	if roundedDown > math.MaxInt64-invoiceRoundingStep {
		return 0, ErrAmountOverflow
	}
	return domain.Money(roundedDown + invoiceRoundingStep), nil
}
