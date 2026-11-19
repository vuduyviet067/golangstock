package repository

import (
	"context"

	"gostock/internal/domain"
)

type SupplierRepository interface {
	// List returns every supplier sorted by name, then ID; never nil.
	List(ctx context.Context) ([]domain.Supplier, error)
	// GetByID returns ErrSupplierNotFound when no supplier has the ID.
	GetByID(ctx context.Context, id int64) (*domain.Supplier, error)
	// Create inserts the supplier and sets its ID and timestamps.
	Create(ctx context.Context, supplier *domain.Supplier) error
	// Update writes the name and the phone (nil stores NULL), sets updated_at
	// to now and fills in both stored timestamps. It returns
	// ErrSupplierNotFound when no supplier has the ID.
	Update(ctx context.Context, supplier *domain.Supplier) error
	// Delete removes the supplier. It returns ErrSupplierNotFound when no
	// supplier has the ID and ErrInUse when another row still references it.
	Delete(ctx context.Context, id int64) error
}
