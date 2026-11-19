package usecase

import (
	"errors"
	"testing"
	"time"
)

func TestFormatInvoiceNumber(t *testing.T) {
	vn, err := time.LoadLocation("Asia/Ho_Chi_Minh")
	if err != nil {
		t.Fatal(err)
	}
	day := time.Date(2026, 11, 19, 9, 0, 0, 0, vn)

	tests := []struct {
		name    string
		at      time.Time
		seq     int
		want    string
		wantErr error
	}{
		{"số đầu tiên trong ngày", day, 1, "20261119-01", nil},
		{"một chữ số thêm 0", day, 9, "20261119-09", nil},
		{"số 99", day, 99, "20261119-99", nil},
		{"từ 100 thành 3 chữ số", day, 100, "20261119-100", nil},
		{"23:59 giờ Việt Nam vẫn là ngày 19", time.Date(2026, 11, 19, 23, 59, 0, 0, vn), 1, "20261119-01", nil},
		{"17:30 UTC đã sang ngày 20 ở Việt Nam", time.Date(2026, 11, 19, 17, 30, 0, 0, time.UTC), 1, "20261120-01", nil},
		{"số thứ tự 0", day, 0, "", ErrInvalidSequence},
		{"số thứ tự âm", day, -1, "", ErrInvalidSequence},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := formatInvoiceNumber(tt.at, tt.seq)
			if !errors.Is(err, tt.wantErr) {
				t.Fatalf("lỗi = %v, mong đợi %v", err, tt.wantErr)
			}
			if got != tt.want {
				t.Fatalf("kết quả = %q, mong đợi %q", got, tt.want)
			}
		})
	}
}
