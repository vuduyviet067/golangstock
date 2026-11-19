package usecase

import (
	"testing"
	"time"

	"gostock/internal/domain"
)

func TestRefreshToken_IsWithinGracePeriod(t *testing.T) {
	now := time.Now().UTC()
	revokedAt := now
	replacedHash := "replaced-hash-123"
	gracePeriod := 15 * time.Second

	tests := []struct {
		name        string
		token       domain.RefreshToken
		gracePeriod time.Duration
		now         time.Time
		want        bool
	}{
		{
			name: "exact lower bound: elapsed == 0",
			token: domain.RefreshToken{
				RevokedAt:      &revokedAt,
				ReplacedByHash: &replacedHash,
			},
			gracePeriod: gracePeriod,
			now:         revokedAt,
			want:        true,
		},
		{
			name: "within grace period: 0 < elapsed < gracePeriod",
			token: domain.RefreshToken{
				RevokedAt:      &revokedAt,
				ReplacedByHash: &replacedHash,
			},
			gracePeriod: gracePeriod,
			now:         revokedAt.Add(5 * time.Second),
			want:        true,
		},
		{
			name: "exact upper bound: elapsed == gracePeriod",
			token: domain.RefreshToken{
				RevokedAt:      &revokedAt,
				ReplacedByHash: &replacedHash,
			},
			gracePeriod: gracePeriod,
			now:         revokedAt.Add(gracePeriod),
			want:        true,
		},
		{
			name: "negative elapsed time: now < revokedAt",
			token: domain.RefreshToken{
				RevokedAt:      &revokedAt,
				ReplacedByHash: &replacedHash,
			},
			gracePeriod: gracePeriod,
			now:         revokedAt.Add(-1 * time.Second),
			want:        false,
		},
		{
			name: "exceeded grace period: elapsed > gracePeriod",
			token: domain.RefreshToken{
				RevokedAt:      &revokedAt,
				ReplacedByHash: &replacedHash,
			},
			gracePeriod: gracePeriod,
			now:         revokedAt.Add(gracePeriod + time.Millisecond),
			want:        false,
		},
		{
			name: "way beyond grace period",
			token: domain.RefreshToken{
				RevokedAt:      &revokedAt,
				ReplacedByHash: &replacedHash,
			},
			gracePeriod: gracePeriod,
			now:         revokedAt.Add(time.Hour),
			want:        false,
		},
		{
			name: "grace period zero",
			token: domain.RefreshToken{
				RevokedAt:      &revokedAt,
				ReplacedByHash: &replacedHash,
			},
			gracePeriod: 0,
			now:         revokedAt,
			want:        false,
		},
		{
			name: "negative grace period",
			token: domain.RefreshToken{
				RevokedAt:      &revokedAt,
				ReplacedByHash: &replacedHash,
			},
			gracePeriod: -5 * time.Second,
			now:         revokedAt,
			want:        false,
		},
		{
			name: "not replaced (ReplacedByHash is nil)",
			token: domain.RefreshToken{
				RevokedAt:      &revokedAt,
				ReplacedByHash: nil,
			},
			gracePeriod: gracePeriod,
			now:         revokedAt.Add(5 * time.Second),
			want:        false,
		},
		{
			name: "not replaced (ReplacedByHash is empty string)",
			token: domain.RefreshToken{
				RevokedAt:      &revokedAt,
				ReplacedByHash: func() *string { s := ""; return &s }(),
			},
			gracePeriod: gracePeriod,
			now:         revokedAt.Add(5 * time.Second),
			want:        false,
		},
		{
			name: "revokedAt is nil",
			token: domain.RefreshToken{
				RevokedAt:      nil,
				ReplacedByHash: &replacedHash,
			},
			gracePeriod: gracePeriod,
			now:         now,
			want:        false,
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			got := IsRefreshTokenWithinGracePeriod(&tc.token, tc.gracePeriod, tc.now)
			if got != tc.want {
				t.Errorf("IsWithinGracePeriod() = %v, want %v", got, tc.want)
			}
		})
	}
}

func TestRefreshToken_StatusHelpers(t *testing.T) {
	now := time.Now().UTC()
	replaced := "new-hash"

	t.Run("IsRevoked", func(t *testing.T) {
		tokenNotRevoked := domain.RefreshToken{RevokedAt: nil}
		if IsRefreshTokenRevoked(&tokenNotRevoked) {
			t.Errorf("expected IsRevoked() = false for nil RevokedAt")
		}

		tokenRevoked := domain.RefreshToken{RevokedAt: &now}
		if !IsRefreshTokenRevoked(&tokenRevoked) {
			t.Errorf("expected IsRevoked() = true when RevokedAt is set")
		}
	})

	t.Run("IsExpired", func(t *testing.T) {
		tokenFuture := domain.RefreshToken{ExpiresAt: now.Add(time.Hour)}
		if IsRefreshTokenExpired(&tokenFuture, now) {
			t.Errorf("expected IsExpired() = false for future expiry")
		}

		tokenPast := domain.RefreshToken{ExpiresAt: now.Add(-time.Hour)}
		if !IsRefreshTokenExpired(&tokenPast, now) {
			t.Errorf("expected IsExpired() = true for past expiry")
		}

		tokenExact := domain.RefreshToken{ExpiresAt: now}
		if !IsRefreshTokenExpired(&tokenExact, now) {
			t.Errorf("expected IsExpired() = true when ExpiresAt == now")
		}
	})

	t.Run("IsReplaced", func(t *testing.T) {
		tokenNil := domain.RefreshToken{ReplacedByHash: nil}
		if IsRefreshTokenReplaced(&tokenNil) {
			t.Errorf("expected IsReplaced() = false for nil")
		}

		empty := ""
		tokenEmpty := domain.RefreshToken{ReplacedByHash: &empty}
		if IsRefreshTokenReplaced(&tokenEmpty) {
			t.Errorf("expected IsReplaced() = false for empty string")
		}

		tokenReplaced := domain.RefreshToken{ReplacedByHash: &replaced}
		if !IsRefreshTokenReplaced(&tokenReplaced) {
			t.Errorf("expected IsReplaced() = true when ReplacedByHash is set")
		}
	})
}
