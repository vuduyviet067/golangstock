package repository

import (
	"context"
	"time"

	"gostock/internal/domain"
)

type UserRepository interface {
	Create(ctx context.Context, user *domain.User) error
	GetByEmail(ctx context.Context, email string) (*domain.User, error)
	GetByID(ctx context.Context, id int64) (*domain.User, error)
}

type UserManagementRepository interface {
	GetByID(ctx context.Context, id int64) (*domain.User, error)
	List(ctx context.Context, query string, page, limit int) ([]domain.User, int64, error)
	Update(ctx context.Context, user *domain.User) error
	UpdateAndRevokeTokens(ctx context.Context, user *domain.User, revokedAt time.Time) error
}
