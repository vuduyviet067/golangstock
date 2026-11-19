package http

import (
	"context"
	"net/http"
	"net/http/httptest"
	"testing"

	"gostock/internal/domain"
)

// A user repository that returns (nil, nil) yields the existing 404 response
// through the current error mapping, with no write (Step 4.2).
func TestUserHandler_RepositoryReturnsNilNil(t *testing.T) {
	const notFound = `{"success":false,"code":"USER_NOT_FOUND","message":"user not found","data":null}`

	newRepo := func(t *testing.T) *mockUserRepoForHandler {
		return &mockUserRepoForHandler{
			getByIDFn: func(ctx context.Context, id int64) (*domain.User, error) { return nil, nil },
			updateFn: func(ctx context.Context, u *domain.User) error {
				t.Error("no update may follow a lookup that found nothing")
				return nil
			},
		}
	}

	t.Run("get", func(t *testing.T) {
		rec := httptest.NewRecorder()
		setupUserHandler(newRepo(t)).GetByID(rec, httptest.NewRequest(http.MethodGet, "/api/v1/users/7", nil))

		contractAssertResponse(t, rec, http.StatusNotFound, notFound)
	})

	for name, body := range map[string]string{
		"update email": `{"email":"new@example.com"}`,
		"update role":  `{"role":"admin"}`,
	} {
		t.Run(name, func(t *testing.T) {
			rec := httptest.NewRecorder()
			setupUserHandler(newRepo(t)).Update(rec, contractJSONRequest(http.MethodPatch, "/api/v1/users/7", body))

			contractAssertResponse(t, rec, http.StatusNotFound, notFound)
		})
	}
}
