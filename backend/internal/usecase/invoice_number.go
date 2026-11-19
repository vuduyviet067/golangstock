package usecase

import (
	"errors"
	"fmt"
	"time"
)

var ErrInvalidSequence = errors.New("invoice sequence must be at least 1")

// invoiceTimeZone là múi giờ dùng để xác định "ngày" của số hoá đơn.
const invoiceTimeZone = "Asia/Ho_Chi_Minh"

// formatInvoiceNumber tạo số hoá đơn dạng YYYYMMDD-NN, ví dụ "20261119-01".
// Ngày lấy theo giờ Việt Nam; seq là số thứ tự trong ngày, bắt đầu từ 1,
// có ít nhất 2 chữ số (9 → "09", 100 → "100").
func formatInvoiceNumber(at time.Time, seq int) (string, error) {
	if seq < 1 {
		return "", ErrInvalidSequence
	}

	// Đổi sang giờ Việt Nam trước khi lấy ngày: 17:30 UTC ngày 19 đã là
	// 00:30 ngày 20 ở Việt Nam (UTC+7), nên hoá đơn thuộc ngày 20.
	loc, err := time.LoadLocation(invoiceTimeZone)
	if err != nil {
		return "", fmt.Errorf("load time zone %s: %w", invoiceTimeZone, err)
	}
	day := at.In(loc).Format("20060102") // mẫu định dạng của Go: 2006=năm, 01=tháng, 02=ngày

	return fmt.Sprintf("%s-%02d", day, seq), nil
}
