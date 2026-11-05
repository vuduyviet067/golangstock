package domain

import (
	"testing"
	"time"
)

func TestRefreshToken_IsWithinGracePeriod(t *testing.T) {
	now := time.Now().UTC()
	revokedAt := now
	replacedHash := "replaced-hash-123"
	gracePeriod := 15 * time.Second

	tests := []struct {
		name        string
		token       RefreshToken
		gracePeriod time.Duration
		now         time.Time
		want        bool
	}{
		{
			name: "exact lower bound: elapsed == 0",
			token: RefreshToken{
				RevokedAt:      &revokedAt,
				ReplacedByHash: &replacedHash,
			},
			gracePeriod: gracePeriod,
			now:         revokedAt,
			want:        true,
		},
		{
			name: "within grace period: 0 < elapsed < gracePeriod",
			token: RefreshToken{
				RevokedAt:      &revokedAt,
				ReplacedByHash: &replacedHash,
			},
			gracePeriod: gracePeriod,
			now:         revokedAt.Add(5 * time.Second),
			want:        true,
		},
		{
			name: "exact upper bound: elapsed == gracePeriod",
			token: RefreshToken{
				RevokedAt:      &revokedAt,
				ReplacedByHash: &replacedHash,
			},
			gracePeriod: gracePeriod,
			now:         revokedAt.Add(gracePeriod),
			want:        true,
		},
		{
			name: "negative elapsed time: now < revokedAt",
			token: RefreshToken{
				RevokedAt:      &revokedAt,
				ReplacedByHash: &replacedHash,
			},
			gracePeriod: gracePeriod,
			now:         revokedAt.Add(-1 * time.Second),
			want:        false,
		},
		{
			name: "exceeded grace period: elapsed > gracePeriod",
			token: RefreshToken{
				RevokedAt:      &revokedAt,
				ReplacedByHash: &replacedHash,
			},
			gracePeriod: gracePeriod,
			now:         revokedAt.Add(gracePeriod + time.Millisecond),
			want:        false,
		},
		{
			name: "way beyond grace period",
			token: RefreshToken{
				RevokedAt:      &revokedAt,
				ReplacedByHash: &replacedHash,
			},
			gracePeriod: gracePeriod,
			now:         revokedAt.Add(time.Hour),
			want:        false,
		},
		{
			name: "grace period zero",
			token: RefreshToken{
				RevokedAt:      &revokedAt,
				ReplacedByHash: &replacedHash,
			},
			gracePeriod: 0,
			now:         revokedAt,
			want:        false,
		},
		{
			name: "negative grace period",
			token: RefreshToken{
				RevokedAt:      &revokedAt,
				ReplacedByHash: &replacedHash,
			},
			gracePeriod: -5 * time.Second,
			now:         revokedAt,
			want:        false,
		},
		{
			name: "not replaced (ReplacedByHash is nil)",
			token: RefreshToken{
				RevokedAt:      &revokedAt,
				ReplacedByHash: nil,
			},
			gracePeriod: gracePeriod,
			now:         revokedAt.Add(5 * time.Second),
			want:        false,
		},
		{
			name: "not replaced (ReplacedByHash is empty string)",
			token: RefreshToken{
				RevokedAt:      &revokedAt,
				ReplacedByHash: func() *string { s := ""; return &s }(),
			},
			gracePeriod: gracePeriod,
			now:         revokedAt.Add(5 * time.Second),
			want:        false,
		},
		{
			name: "revokedAt is nil",
			token: RefreshToken{
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
			got := tc.token.IsWithinGracePeriod(tc.gracePeriod, tc.now)
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
		tokenNotRevoked := RefreshToken{RevokedAt: nil}
		if tokenNotRevoked.IsRevoked() {
			t.Errorf("expected IsRevoked() = false for nil RevokedAt")
		}

		tokenRevoked := RefreshToken{RevokedAt: &now}
		if !tokenRevoked.IsRevoked() {
			t.Errorf("expected IsRevoked() = true when RevokedAt is set")
		}
	})

	t.Run("IsExpired", func(t *testing.T) {
		tokenFuture := RefreshToken{ExpiresAt: now.Add(time.Hour)}
		if tokenFuture.IsExpired(now) {
			t.Errorf("expected IsExpired() = false for future expiry")
		}

		tokenPast := RefreshToken{ExpiresAt: now.Add(-time.Hour)}
		if !tokenPast.IsExpired(now) {
			t.Errorf("expected IsExpired() = true for past expiry")
		}

		tokenExact := RefreshToken{ExpiresAt: now}
		if !tokenExact.IsExpired(now) {
			t.Errorf("expected IsExpired() = true when ExpiresAt == now")
		}
	})

	t.Run("IsReplaced", func(t *testing.T) {
		tokenNil := RefreshToken{ReplacedByHash: nil}
		if tokenNil.IsReplaced() {
			t.Errorf("expected IsReplaced() = false for nil")
		}

		empty := ""
		tokenEmpty := RefreshToken{ReplacedByHash: &empty}
		if tokenEmpty.IsReplaced() {
			t.Errorf("expected IsReplaced() = false for empty string")
		}

		tokenReplaced := RefreshToken{ReplacedByHash: &replaced}
		if !tokenReplaced.IsReplaced() {
			t.Errorf("expected IsReplaced() = true when ReplacedByHash is set")
		}
	})
}
