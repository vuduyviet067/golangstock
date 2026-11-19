package usecase

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"testing"
	"time"

	"gostock/internal/domain"
	"gostock/internal/dto"
	"gostock/internal/repository"
)

// woodTypeRepoStub records the calls and returns the configured error.
type woodTypeRepoStub struct {
	list    []domain.WoodType
	err     error
	created *domain.WoodType
	updated *domain.WoodType
	deleted int64
	calls   int
}

var stubTime = time.Date(2026, 11, 20, 8, 0, 0, 0, time.UTC)

func (s *woodTypeRepoStub) List(ctx context.Context) ([]domain.WoodType, error) {
	s.calls++
	return s.list, s.err
}

func (s *woodTypeRepoStub) GetByID(ctx context.Context, id int64) (*domain.WoodType, error) {
	s.calls++
	return nil, s.err
}

func (s *woodTypeRepoStub) Create(ctx context.Context, w *domain.WoodType) error {
	s.calls++
	if s.err != nil {
		return s.err
	}
	w.ID, w.CreatedAt, w.UpdatedAt = 7, stubTime, stubTime
	copy := *w
	s.created = &copy
	return nil
}

func (s *woodTypeRepoStub) Update(ctx context.Context, w *domain.WoodType) error {
	s.calls++
	if s.err != nil {
		return s.err
	}
	w.CreatedAt, w.UpdatedAt = stubTime, stubTime.Add(time.Hour)
	copy := *w
	s.updated = &copy
	return nil
}

func (s *woodTypeRepoStub) Delete(ctx context.Context, id int64) error {
	s.calls++
	s.deleted = id
	return s.err
}

func strPtr(s string) *string { return &s }

func TestWoodTypeUseCase_List(t *testing.T) {
	repo := &woodTypeRepoStub{list: []domain.WoodType{{ID: 2, Name: "Gỗ lim"}, {ID: 1, Name: "Gỗ sến"}}}
	got, err := NewWoodTypeUseCase(repo).List(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	if len(got) != 2 || got[0].ID != 2 || got[1].Name != "Gỗ sến" {
		t.Errorf("List = %+v, want the repository's order", got)
	}

	empty, err := NewWoodTypeUseCase(&woodTypeRepoStub{list: []domain.WoodType{}}).List(context.Background())
	if err != nil || empty == nil || len(empty) != 0 {
		t.Errorf("empty List = %#v, %v; want a non-nil empty slice", empty, err)
	}
}

func TestWoodTypeUseCase_Create(t *testing.T) {
	repo := &woodTypeRepoStub{}
	got, err := NewWoodTypeUseCase(repo).Create(context.Background(), dto.CreateWoodTypeInput{Name: "  Gỗ lim  "})
	if err != nil {
		t.Fatal(err)
	}
	if repo.created == nil || repo.created.Name != "Gỗ lim" {
		t.Fatalf("stored %+v, want the trimmed name", repo.created)
	}
	want := dto.WoodTypeResponse{ID: 7, Name: "Gỗ lim", CreatedAt: stubTime, UpdatedAt: stubTime}
	if got != want {
		t.Errorf("Create = %+v, want %+v", got, want)
	}

	// 100 characters (not bytes) is the limit: "ỗ" is 3 bytes.
	if _, err := NewWoodTypeUseCase(&woodTypeRepoStub{}).Create(context.Background(),
		dto.CreateWoodTypeInput{Name: strings.Repeat("ỗ", 100)}); err != nil {
		t.Errorf("100 characters: %v", err)
	}

	for name, raw := range map[string]string{
		"empty":          "",
		"blank":          " \t ",
		"101 characters": strings.Repeat("ỗ", 101),
	} {
		t.Run(name, func(t *testing.T) {
			repo := &woodTypeRepoStub{}
			if _, err := NewWoodTypeUseCase(repo).Create(context.Background(), dto.CreateWoodTypeInput{Name: raw}); !errors.Is(err, ErrInvalidWoodType) {
				t.Fatalf("err = %v, want ErrInvalidWoodType", err)
			}
			if repo.calls != 0 {
				t.Error("the repository was called")
			}
		})
	}

	dbErr := errors.New("connection reset")
	if _, err := NewWoodTypeUseCase(&woodTypeRepoStub{err: dbErr}).Create(context.Background(),
		dto.CreateWoodTypeInput{Name: "Gỗ lim"}); !errors.Is(err, dbErr) {
		t.Errorf("repository error: err = %v", err)
	}
}

func TestWoodTypeUseCase_Update(t *testing.T) {
	repo := &woodTypeRepoStub{}
	got, err := NewWoodTypeUseCase(repo).Update(context.Background(), dto.UpdateWoodTypeInput{ID: 3, Name: strPtr(" Gỗ sến ")})
	if err != nil {
		t.Fatal(err)
	}
	if repo.updated == nil || repo.updated.ID != 3 || repo.updated.Name != "Gỗ sến" {
		t.Fatalf("updated %+v", repo.updated)
	}
	if got.Name != "Gỗ sến" || !got.UpdatedAt.Equal(stubTime.Add(time.Hour)) {
		t.Errorf("Update = %+v, want the stored timestamps", got)
	}

	tests := []struct {
		name    string
		in      dto.UpdateWoodTypeInput
		repoErr error
		wantErr error
	}{
		{"ID 0", dto.UpdateWoodTypeInput{ID: 0, Name: strPtr("x")}, nil, ErrInvalidWoodTypeID},
		{"no field", dto.UpdateWoodTypeInput{ID: 3}, nil, ErrEmptyUpdate},
		{"blank name", dto.UpdateWoodTypeInput{ID: 3, Name: strPtr("  ")}, nil, ErrInvalidWoodType},
		{"not found", dto.UpdateWoodTypeInput{ID: 3, Name: strPtr("x")}, repository.ErrWoodTypeNotFound, ErrWoodTypeNotFound},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			_, err := NewWoodTypeUseCase(&woodTypeRepoStub{err: tt.repoErr}).Update(context.Background(), tt.in)
			if !errors.Is(err, tt.wantErr) {
				t.Fatalf("err = %v, want %v", err, tt.wantErr)
			}
		})
	}
}

func TestWoodTypeUseCase_Delete(t *testing.T) {
	repo := &woodTypeRepoStub{}
	if err := NewWoodTypeUseCase(repo).Delete(context.Background(), 5); err != nil || repo.deleted != 5 {
		t.Fatalf("Delete: %v, deleted %d", err, repo.deleted)
	}

	tests := []struct {
		name    string
		id      int64
		repoErr error
		wantErr error
	}{
		{"ID 0", 0, nil, ErrInvalidWoodTypeID},
		{"negative ID", -1, nil, ErrInvalidWoodTypeID},
		{"not found", 5, repository.ErrWoodTypeNotFound, ErrWoodTypeNotFound},
		{"in use", 5, repository.ErrInUse, ErrInUse},
		{"wrapped in use", 5, fmt.Errorf("%w: detail", repository.ErrInUse), ErrInUse},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := NewWoodTypeUseCase(&woodTypeRepoStub{err: tt.repoErr}).Delete(context.Background(), tt.id)
			if !errors.Is(err, tt.wantErr) {
				t.Fatalf("err = %v, want %v", err, tt.wantErr)
			}
			if tt.repoErr != nil && errors.Is(err, tt.repoErr) {
				t.Errorf("err %v still matches the repository error", err)
			}
		})
	}
}

// The new repository errors convert to the application errors with the same
// message (TestFromRepository covers the older ones).
func TestFromRepository_Catalogs(t *testing.T) {
	for _, p := range []struct{ repo, app error }{
		{repository.ErrWoodTypeNotFound, ErrWoodTypeNotFound},
		{repository.ErrSupplierNotFound, ErrSupplierNotFound},
		{repository.ErrInUse, ErrInUse},
	} {
		got := fromRepository(p.repo)
		if got != p.app || got.Error() != p.repo.Error() {
			t.Errorf("fromRepository(%v) = %v, want the application error with the same message", p.repo, got)
		}
		if wrapped := fromRepository(fmt.Errorf("%w: detail", p.repo)); !errors.Is(wrapped, p.app) || errors.Is(wrapped, p.repo) {
			t.Errorf("wrapped %v: got %v", p.repo, wrapped)
		}
	}
}
