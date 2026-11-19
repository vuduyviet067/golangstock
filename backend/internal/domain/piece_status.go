package domain

// PieceStatus là trạng thái của một cục gỗ.
type PieceStatus string

const (
	PieceAvailable PieceStatus = "available" // Sẵn sàng bán
	PieceSold      PieceStatus = "sold"      // Đã bán
	PieceSawing    PieceStatus = "sawing"    // Đang xẻ
	PieceDelivered PieceStatus = "delivered" // Đã giao
)
