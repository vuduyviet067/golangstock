package usecase

import (
	"context"
	"errors"
	"strings"

	"gostock/internal/domain"
	"gostock/internal/dto"
	"gostock/internal/mapper"
	"gostock/internal/repository"
	"gostock/pkg/phone"
)

var (
	ErrInvalidSupplierID = errors.New("invalid supplier ID")
	ErrInvalidSupplier   = errors.New("invalid supplier")
	ErrInvalidPhone      = errors.New("invalid phone number")
)

type SupplierUseCase struct {
	repo repository.SupplierRepository
}

func NewSupplierUseCase(repo repository.SupplierRepository) *SupplierUseCase {
	return &SupplierUseCase{repo: repo}
}

// normalizeSupplierPhone turns a phone sent by the client into the value to
// store: nil for a blank number (no phone), the normalized number otherwise.
// A number that is not a valid Vietnamese phone number is ErrInvalidPhone.
func normalizeSupplierPhone(raw string) (*string, error) {
	if strings.TrimSpace(raw) == "" {
		return nil, nil
	}
	normalized, err := phone.Normalize(raw)
	if err != nil {
		return nil, ErrInvalidPhone
	}
	return &normalized, nil
}

// List returns every supplier sorted by name, never nil (JSON []).
func (uc *SupplierUseCase) List(ctx context.Context) ([]dto.SupplierResponse, error) {
	suppliers, err := uc.repo.List(ctx)
	if err != nil {
		return nil, fromRepository(err)
	}
	return mapper.ToSupplierResponses(suppliers), nil
}

// Create stores a new supplier. The name is trimmed and must have 1 to 100
// characters. The phone is optional (nil or blank means none); when given it
// is stored normalized, e.g. "+84 368 122 137" as "0368122137".
func (uc *SupplierUseCase) Create(ctx context.Context, in dto.CreateSupplierInput) (dto.SupplierResponse, error) {
	name, ok := normalizeCatalogName(in.Name)
	if !ok {
		return dto.SupplierResponse{}, ErrInvalidSupplier
	}

	var phoneNumber *string
	if in.Phone != nil {
		var err error
		if phoneNumber, err = normalizeSupplierPhone(*in.Phone); err != nil {
			return dto.SupplierResponse{}, err
		}
	}

	supplier := domain.Supplier{Name: name, Phone: phoneNumber}
	if err := uc.repo.Create(ctx, &supplier); err != nil {
		return dto.SupplierResponse{}, fromRepository(err)
	}
	return mapper.ToSupplierResponse(supplier), nil
}

// Update edits the fields that are given (non-nil) and keeps the others; an
// input with no field is ErrEmptyUpdate. A blank phone removes the stored
// number. Every given field is validated before the supplier is read.
func (uc *SupplierUseCase) Update(ctx context.Context, in dto.UpdateSupplierInput) (dto.SupplierResponse, error) {
	if in.ID <= 0 {
		return dto.SupplierResponse{}, ErrInvalidSupplierID
	}
	if in.Name == nil && in.Phone == nil {
		return dto.SupplierResponse{}, ErrEmptyUpdate
	}

	var name string
	if in.Name != nil {
		var ok bool
		if name, ok = normalizeCatalogName(*in.Name); !ok {
			return dto.SupplierResponse{}, ErrInvalidSupplier
		}
	}
	var phoneNumber *string
	if in.Phone != nil {
		var err error
		if phoneNumber, err = normalizeSupplierPhone(*in.Phone); err != nil {
			return dto.SupplierResponse{}, err
		}
	}

	// Read the stored supplier and apply only the fields that were sent.
	supplier, err := uc.repo.GetByID(ctx, in.ID)
	if err != nil {
		return dto.SupplierResponse{}, fromRepository(err)
	}
	if supplier == nil {
		return dto.SupplierResponse{}, ErrSupplierNotFound
	}
	if in.Name != nil {
		supplier.Name = name
	}
	if in.Phone != nil {
		supplier.Phone = phoneNumber // nil when the client sent "" (remove)
	}

	if err := uc.repo.Update(ctx, supplier); err != nil {
		return dto.SupplierResponse{}, fromRepository(err)
	}
	return mapper.ToSupplierResponse(*supplier), nil
}

// Delete removes the supplier for good. A supplier still referenced by
// another row is ErrInUse.
func (uc *SupplierUseCase) Delete(ctx context.Context, id int64) error {
	if id <= 0 {
		return ErrInvalidSupplierID
	}
	return fromRepository(uc.repo.Delete(ctx, id))
}
