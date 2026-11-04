package domain

import (
	"context"
	"time"
)

type UserRepository interface {
	Create(ctx context.Context, user *User) error
	GetByEmail(ctx context.Context, email string) (*User, error)
	GetByID(ctx context.Context, id int64) (*User, error)
}

type UserManagementRepository interface {
	GetByID(ctx context.Context, id int64) (*User, error)
	List(ctx context.Context, query string, page, limit int) ([]User, int64, error)
	Update(ctx context.Context, user *User) error
	UpdateAndRevokeTokens(ctx context.Context, user *User, revokedAt time.Time) error
}
