package usecase

import "gostock/internal/domain"

// allowedTransitions liệt kê các lần đổi trạng thái hợp lệ của một cục gỗ:
// trạng thái hiện tại → các trạng thái được chuyển sang.
//
//	available → sold
//	sold      → sawing, delivered
//	sawing    → delivered
//
// delivered là trạng thái cuối, không chuyển đi đâu nữa.
var allowedTransitions = map[domain.PieceStatus]map[domain.PieceStatus]bool{
	domain.PieceAvailable: {domain.PieceSold: true},
	domain.PieceSold:      {domain.PieceSawing: true, domain.PieceDelivered: true},
	domain.PieceSawing:    {domain.PieceDelivered: true},
}

// canTransition cho biết cục gỗ có được đổi từ trạng thái from sang to không.
// Giữ nguyên trạng thái, trạng thái lạ hoặc rỗng đều trả false: tra một khoá
// không có trong map cho ra map rỗng (nil), và tra tiếp trong đó cho ra false.
func canTransition(from, to domain.PieceStatus) bool {
	return allowedTransitions[from][to]
}
