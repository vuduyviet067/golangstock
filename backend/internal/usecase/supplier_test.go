package usecase

import (
	"context"
	"errors"
	"strings"
	"testing"

	"gostock/internal/domain"
	"gostock/internal/dto"
	"gostock/internal/repository"
)

// supplierRepoStub stores one supplier in memory and records the calls.
type supplierRepoStub struct {
	stored    *domain.Supplier
	getErr    error
	writeErr  error
	created   *domain.Supplier
	updated   *domain.Supplier
	deleteErr error
	calls     int
}

func (s *supplierRepoStub) List(ctx context.Context) ([]domain.Supplier, error) {
	s.calls++
	if s.stored == nil {
		return []domain.Supplier{}, nil
	}
	return []domain.Supplier{*s.stored}, nil
}

func (s *supplierRepoStub) GetByID(ctx context.Context, id int64) (*domain.Supplier, error) {
	s.calls++
	if s.getErr != nil {
		return nil, s.getErr
	}
	if s.stored == nil || s.stored.ID != id {
		return nil, repository.ErrSupplierNotFound
	}
	copy := *s.stored
	return &copy, nil
}

func (s *supplierRepoStub) Create(ctx context.Context, sup *domain.Supplier) error {
	s.calls++
	if s.writeErr != nil {
		return s.writeErr
	}
	sup.ID, sup.CreatedAt, sup.UpdatedAt = 9, stubTime, stubTime
	copy := *sup
	s.created = &copy
	return nil
}

func (s *supplierRepoStub) Update(ctx context.Context, sup *domain.Supplier) error {
	s.calls++
	if s.writeErr != nil {
		return s.writeErr
	}
	copy := *sup
	s.updated = &copy
	return nil
}

func (s *supplierRepoStub) Delete(ctx context.Context, id int64) error {
	s.calls++
	return s.deleteErr
}

func phoneOf(p *string) string {
	if p == nil {
		return "<nil>"
	}
	return *p
}

func TestSupplierUseCase_Create(t *testing.T) {
	tests := []struct {
		name      string
		in        dto.CreateSupplierInput
		wantPhone string
		wantErr   error
	}{
		{"phone normalized", dto.CreateSupplierInput{Name: " An Phát ", Phone: strPtr("+84 368 122 137")}, "0368122137", nil},
		{"no phone", dto.CreateSupplierInput{Name: "An Phát"}, "<nil>", nil},
		{"empty phone means none", dto.CreateSupplierInput{Name: "An Phát", Phone: strPtr("")}, "<nil>", nil},
		{"blank phone means none", dto.CreateSupplierInput{Name: "An Phát", Phone: strPtr("   ")}, "<nil>", nil},
		{"invalid phone", dto.CreateSupplierInput{Name: "An Phát", Phone: strPtr("12345")}, "", ErrInvalidPhone},
		{"blank name", dto.CreateSupplierInput{Name: "  ", Phone: strPtr("0368122137")}, "", ErrInvalidSupplier},
		{"101 characters", dto.CreateSupplierInput{Name: strings.Repeat("ư", 101)}, "", ErrInvalidSupplier},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			repo := &supplierRepoStub{}
			got, err := NewSupplierUseCase(repo).Create(context.Background(), tt.in)
			if !errors.Is(err, tt.wantErr) {
				t.Fatalf("err = %v, want %v", err, tt.wantErr)
			}
			if tt.wantErr != nil {
				if repo.calls != 0 {
					t.Error("the repository was called")
				}
				return
			}
			if repo.created.Name != "An Phát" || phoneOf(repo.created.Phone) != tt.wantPhone {
				t.Errorf("stored %q %s, want \"An Phát\" %s", repo.created.Name, phoneOf(repo.created.Phone), tt.wantPhone)
			}
			if got.ID != 9 || phoneOf(got.Phone) != tt.wantPhone {
				t.Errorf("response %+v", got)
			}
		})
	}
}

func TestSupplierUseCase_Update(t *testing.T) {
	stored := func() *domain.Supplier {
		return &domain.Supplier{ID: 4, Name: "An Phát", Phone: strPtr("0368122137"), CreatedAt: stubTime, UpdatedAt: stubTime}
	}
	tests := []struct {
		name      string
		in        dto.UpdateSupplierInput
		wantName  string
		wantPhone string
	}{
		{"name only keeps the phone", dto.UpdateSupplierInput{ID: 4, Name: strPtr(" Bình Minh ")}, "Bình Minh", "0368122137"},
		{"phone only keeps the name", dto.UpdateSupplierInput{ID: 4, Phone: strPtr("0912.345.678")}, "An Phát", "0912345678"},
		{"empty phone removes it", dto.UpdateSupplierInput{ID: 4, Phone: strPtr("")}, "An Phát", "<nil>"},
		{"both", dto.UpdateSupplierInput{ID: 4, Name: strPtr("Bình Minh"), Phone: strPtr("+84912345678")}, "Bình Minh", "0912345678"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			repo := &supplierRepoStub{stored: stored()}
			got, err := NewSupplierUseCase(repo).Update(context.Background(), tt.in)
			if err != nil {
				t.Fatal(err)
			}
			if repo.updated.Name != tt.wantName || phoneOf(repo.updated.Phone) != tt.wantPhone {
				t.Errorf("stored %q %s, want %q %s", repo.updated.Name, phoneOf(repo.updated.Phone), tt.wantName, tt.wantPhone)
			}
			if got.Name != tt.wantName || phoneOf(got.Phone) != tt.wantPhone || got.ID != 4 {
				t.Errorf("response %+v", got)
			}
		})
	}

	errTests := []struct {
		name      string
		in        dto.UpdateSupplierInput
		repo      *supplierRepoStub
		wantErr   error
		wantCalls int
	}{
		{"ID 0", dto.UpdateSupplierInput{ID: 0, Name: strPtr("x")}, &supplierRepoStub{stored: stored()}, ErrInvalidSupplierID, 0},
		{"no field", dto.UpdateSupplierInput{ID: 4}, &supplierRepoStub{stored: stored()}, ErrEmptyUpdate, 0},
		{"blank name", dto.UpdateSupplierInput{ID: 4, Name: strPtr(" ")}, &supplierRepoStub{stored: stored()}, ErrInvalidSupplier, 0},
		{"invalid phone", dto.UpdateSupplierInput{ID: 4, Phone: strPtr("0036812213")}, &supplierRepoStub{stored: stored()}, ErrInvalidPhone, 0},
		{"not found", dto.UpdateSupplierInput{ID: 5, Name: strPtr("x")}, &supplierRepoStub{stored: stored()}, ErrSupplierNotFound, 1},
		{"deleted before the update", dto.UpdateSupplierInput{ID: 4, Name: strPtr("x")},
			&supplierRepoStub{stored: stored(), writeErr: repository.ErrSupplierNotFound}, ErrSupplierNotFound, 2},
	}
	for _, tt := range errTests {
		t.Run(tt.name, func(t *testing.T) {
			_, err := NewSupplierUseCase(tt.repo).Update(context.Background(), tt.in)
			if !errors.Is(err, tt.wantErr) {
				t.Fatalf("err = %v, want %v", err, tt.wantErr)
			}
			if tt.repo.calls != tt.wantCalls {
				t.Errorf("repository calls = %d, want %d", tt.repo.calls, tt.wantCalls)
			}
		})
	}
}

func TestSupplierUseCase_ListAndDelete(t *testing.T) {
	list, err := NewSupplierUseCase(&supplierRepoStub{}).List(context.Background())
	if err != nil || list == nil || len(list) != 0 {
		t.Errorf("empty List = %#v, %v; want a non-nil empty slice", list, err)
	}

	for _, tt := range []struct {
		name    string
		id      int64
		repoErr error
		wantErr error
	}{
		{"deleted", 4, nil, nil},
		{"ID 0", 0, nil, ErrInvalidSupplierID},
		{"not found", 4, repository.ErrSupplierNotFound, ErrSupplierNotFound},
		{"in use", 4, repository.ErrInUse, ErrInUse},
	} {
		t.Run(tt.name, func(t *testing.T) {
			err := NewSupplierUseCase(&supplierRepoStub{deleteErr: tt.repoErr}).Delete(context.Background(), tt.id)
			if !errors.Is(err, tt.wantErr) {
				t.Fatalf("err = %v, want %v", err, tt.wantErr)
			}
		})
	}
}
