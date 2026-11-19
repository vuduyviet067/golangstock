package usecase

import (
	"testing"

	"gostock/internal/domain"
)

func TestCanTransition(t *testing.T) {
	all := []domain.PieceStatus{
		domain.PieceAvailable, domain.PieceSold, domain.PieceSawing, domain.PieceDelivered,
	}
	allowed := map[[2]domain.PieceStatus]bool{
		{domain.PieceAvailable, domain.PieceSold}:   true,
		{domain.PieceSold, domain.PieceSawing}:      true,
		{domain.PieceSold, domain.PieceDelivered}:   true,
		{domain.PieceSawing, domain.PieceDelivered}: true,
	}

	for _, from := range all {
		for _, to := range all {
			want := allowed[[2]domain.PieceStatus{from, to}]
			t.Run(string(from)+"->"+string(to), func(t *testing.T) {
				if got := canTransition(from, to); got != want {
					t.Fatalf("canTransition(%s, %s) = %v, mong đợi %v", from, to, got, want)
				}
			})
		}
	}

	if canTransition("lost", domain.PieceSold) {
		t.Fatal("trạng thái nguồn lạ phải bị từ chối")
	}
	if canTransition(domain.PieceAvailable, "") {
		t.Fatal("trạng thái đích rỗng phải bị từ chối")
	}
}
