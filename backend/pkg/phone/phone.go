// Package phone chuẩn hoá số điện thoại Việt Nam về một dạng duy nhất, ví dụ
// "+84 368 122 137" và "0368.122.137" đều thành "0368122137".
package phone

import (
	"errors"
	"strings"
)

// ErrInvalid là lỗi khi chuỗi không phải số điện thoại Việt Nam hợp lệ.
var ErrInvalid = errors.New("invalid Vietnamese phone number")

// separators bỏ các ký tự người dùng hay gõ để ngăn cách: dấu cách, dấu chấm,
// gạch ngang và ngoặc tròn.
var separators = strings.NewReplacer(" ", "", ".", "", "-", "", "(", "", ")", "")

// Normalize trả số điện thoại dạng 0xxxxxxxxx (10 hoặc 11 chữ số).
func Normalize(raw string) (string, error) {
	// 1. Bỏ ký tự ngăn cách.
	s := separators.Replace(raw)

	// 2. Đổi mã nước 84 thành số 0 ở đầu.
	switch {
	case strings.HasPrefix(s, "+84"):
		s = "0" + s[len("+84"):]
	case strings.HasPrefix(s, "84") && len(s) >= 11:
		// Chỉ đổi khi đủ dài, vì số nội địa không bao giờ bắt đầu bằng 84;
		// "84368122137" (11 chữ số) là 0368122137 viết kèm mã nước.
		s = "0" + s[len("84"):]
	}

	// 3. Chỉ còn chữ số 0–9 (dấu + của mã nước khác cũng bị từ chối ở đây).
	for i := 0; i < len(s); i++ {
		if s[i] < '0' || s[i] > '9' {
			return "", ErrInvalid
		}
	}

	// 4. Bắt đầu bằng 0, chữ số thứ hai từ 1 đến 9, tổng 10 hoặc 11 chữ số.
	if len(s) != 10 && len(s) != 11 {
		return "", ErrInvalid
	}
	if s[0] != '0' || s[1] == '0' {
		return "", ErrInvalid
	}
	return s, nil
}
