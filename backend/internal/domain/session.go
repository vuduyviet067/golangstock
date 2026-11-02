package domain

import (
	"errors"
	"time"
)

var ErrSessionNotFound = errors.New("session not found")

type Session struct {
	ID        int64
	UserID    int64
	TokenHash string
	ExpiresAt time.Time
	CreatedAt time.Time
}
