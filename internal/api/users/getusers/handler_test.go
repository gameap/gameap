package getusers_test

import (
	"context"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/gameap/gameap/internal/api/users/getusers"
	"github.com/gameap/gameap/internal/domain"
	"github.com/gameap/gameap/internal/repositories/inmemory"
	"github.com/gameap/gameap/internal/services"
	"github.com/gameap/gameap/pkg/api"
	"github.com/gameap/gameap/pkg/auth"
	"github.com/samber/lo"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestGetUsers(t *testing.T) {
	createdAt := time.Date(2025, 9, 19, 16, 43, 1, 0, time.UTC)
	updatedAt := time.Date(2025, 9, 18, 18, 20, 58, 0, time.UTC)

	tests := []struct {
		name    string
		users   []domain.User
		session *auth.Session
		want    string
		wantErr bool
	}{
		{
			name: "success with multiple users",
			users: []domain.User{
				{
					ID:        1,
					Login:     "admin",
					Email:     "admin@yousite.local",
					Name:      lo.ToPtr("New Name"),
					CreatedAt: &createdAt,
					UpdatedAt: &updatedAt,
				},
				{
					ID:        2,
					Login:     "Test",
					Email:     "test@gameap.com",
					Name:      lo.ToPtr("Test"),
					CreatedAt: lo.ToPtr(time.Date(2025, 9, 18, 18, 31, 12, 0, time.UTC)),
					UpdatedAt: lo.ToPtr(time.Date(2025, 9, 23, 22, 0, 14, 0, time.UTC)),
				},
			},
			session: &auth.Session{
				User: &domain.User{
					ID:    1,
					Login: "admin",
				},
			},
			want: `[
				{
					"id": 1,
					"login": "admin",
					"email": "admin@yousite.local",
					"name": "New Name",
					"created_at": "2025-09-19T16:43:01Z",
					"updated_at": "2025-09-18T18:20:58Z"
				},
				{
					"id": 2,
					"login": "Test",
					"email": "test@gameap.com",
					"name": "Test",
					"created_at": "2025-09-18T18:31:12Z",
					"updated_at": "2025-09-23T22:00:14Z"
				}
			]`,
		},
		{
			name:  "success with empty users list",
			users: []domain.User{},
			session: &auth.Session{
				User: &domain.User{
					ID:    1,
					Login: "admin",
				},
			},
			want: `[]`,
		},
		{
			name:    "unauthorized when not authenticated",
			users:   []domain.User{},
			session: &auth.Session{},
			wantErr: true,
		},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			// ARRANGE
			repo := inmemory.NewUserRepository()

			for _, user := range test.users {
				userCopy := user
				err := repo.Save(context.Background(), &userCopy)
				require.NoError(t, err)
			}

			userService := services.NewUserService(repo)
			h := getusers.NewHandler(userService, api.NewResponder())
			recorder := httptest.NewRecorder()

			req := httptest.NewRequest(http.MethodGet, "/api/users", nil)
			if test.session != nil {
				ctx := auth.ContextWithSession(req.Context(), test.session)
				req = req.WithContext(ctx)
			}

			// ACT
			h.ServeHTTP(recorder, req)

			// ASSERT
			if test.wantErr {
				assert.Equal(t, http.StatusUnauthorized, recorder.Code)
			} else {
				assert.Equal(t, http.StatusOK, recorder.Code)
				assert.JSONEq(t, test.want, recorder.Body.String())
			}
		})
	}
}
