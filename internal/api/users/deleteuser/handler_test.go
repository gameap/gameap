package deleteuser

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/gameap/gameap/internal/domain"
	"github.com/gameap/gameap/internal/repositories/inmemory"
	"github.com/gameap/gameap/internal/services"
	"github.com/gameap/gameap/pkg/api"
	"github.com/gameap/gameap/pkg/auth"
	"github.com/gorilla/mux"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

var testUser1 = domain.User{
	ID:    1,
	Login: "admin",
	Email: "admin@example.com",
}

func TestHandler_ServeHTTP(t *testing.T) {
	tests := []struct {
		name           string
		userID         string
		setupAuth      func() context.Context
		setupRepo      func(*inmemory.UserRepository)
		expectedStatus int
		wantError      string
	}{
		{
			name:   "successful user deletion",
			userID: "2",
			setupAuth: func() context.Context {
				session := &auth.Session{
					Login: "admin",
					Email: "admin@example.com",
					User:  &testUser1,
				}

				return auth.ContextWithSession(context.Background(), session)
			},
			setupRepo: func(usersRepo *inmemory.UserRepository) {
				now := time.Now()
				name := "User To Delete"

				user := &domain.User{
					ID:        2,
					Login:     "deleteuser",
					Email:     "deleteuser@example.com",
					Name:      &name,
					CreatedAt: &now,
					UpdatedAt: &now,
				}

				require.NoError(t, usersRepo.Save(context.Background(), user))
			},
			expectedStatus: http.StatusNoContent,
		},
		{
			name:   "user not found",
			userID: "999",
			setupAuth: func() context.Context {
				session := &auth.Session{
					Login: "admin",
					Email: "admin@example.com",
					User:  &testUser1,
				}

				return auth.ContextWithSession(context.Background(), session)
			},
			setupRepo:      func(_ *inmemory.UserRepository) {},
			expectedStatus: http.StatusNotFound,
			wantError:      "user not found",
		},
		{
			name:           "user not authenticated",
			userID:         "2",
			setupRepo:      func(_ *inmemory.UserRepository) {},
			expectedStatus: http.StatusUnauthorized,
			wantError:      "user not authenticated",
		},
		{
			name:   "invalid user id",
			userID: "invalid",
			setupAuth: func() context.Context {
				session := &auth.Session{
					Login: "admin",
					Email: "admin@example.com",
					User:  &testUser1,
				}

				return auth.ContextWithSession(context.Background(), session)
			},
			setupRepo:      func(_ *inmemory.UserRepository) {},
			expectedStatus: http.StatusBadRequest,
			wantError:      "invalid user id",
		},
		{
			name:   "cannot delete yourself",
			userID: "1",
			setupAuth: func() context.Context {
				session := &auth.Session{
					Login: "admin",
					Email: "admin@example.com",
					User:  &testUser1,
				}

				return auth.ContextWithSession(context.Background(), session)
			},
			setupRepo: func(usersRepo *inmemory.UserRepository) {
				now := time.Now()
				name := "Admin User"

				user := &domain.User{
					ID:        1,
					Login:     "admin",
					Email:     "admin@example.com",
					Name:      &name,
					CreatedAt: &now,
					UpdatedAt: &now,
				}

				require.NoError(t, usersRepo.Save(context.Background(), user))
			},
			expectedStatus: http.StatusBadRequest,
			wantError:      "cannot delete yourself",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			usersRepo := inmemory.NewUserRepository()
			userService := services.NewUserService(usersRepo)
			responder := api.NewResponder()
			handler := NewHandler(userService, responder)

			if tt.setupRepo != nil {
				tt.setupRepo(usersRepo)
			}

			ctx := context.Background()
			if tt.setupAuth != nil {
				ctx = tt.setupAuth()
			}

			req := httptest.NewRequest(http.MethodDelete, "/api/users/"+tt.userID, nil)
			req = req.WithContext(ctx)
			req = mux.SetURLVars(req, map[string]string{"id": tt.userID})
			w := httptest.NewRecorder()

			handler.ServeHTTP(w, req)

			assert.Equal(t, tt.expectedStatus, w.Code)

			if tt.wantError != "" {
				var response map[string]any
				require.NoError(t, json.Unmarshal(w.Body.Bytes(), &response))
				assert.Equal(t, "error", response["status"])
				errorMsg, ok := response["error"].(string)
				require.True(t, ok)
				assert.Contains(t, errorMsg, tt.wantError)
			}

			if tt.expectedStatus == http.StatusNoContent {
				assert.Empty(t, w.Body.String())
			}
		})
	}
}

func TestHandler_UserActuallyDeleted(t *testing.T) {
	usersRepo := inmemory.NewUserRepository()
	userService := services.NewUserService(usersRepo)
	responder := api.NewResponder()
	handler := NewHandler(userService, responder)

	now := time.Now()
	name := "User To Delete"

	user := &domain.User{
		ID:        2,
		Login:     "deleteuser",
		Email:     "deleteuser@example.com",
		Name:      &name,
		CreatedAt: &now,
		UpdatedAt: &now,
	}

	require.NoError(t, usersRepo.Save(context.Background(), user))

	session := &auth.Session{
		Login: "admin",
		Email: "admin@example.com",
		User:  &testUser1,
	}
	ctx := auth.ContextWithSession(context.Background(), session)

	req := httptest.NewRequest(http.MethodDelete, "/api/users/2", nil)
	req = req.WithContext(ctx)
	req = mux.SetURLVars(req, map[string]string{"id": "2"})
	w := httptest.NewRecorder()

	handler.ServeHTTP(w, req)

	require.Equal(t, http.StatusNoContent, w.Code)

	users, err := usersRepo.Find(ctx, nil, nil, nil)
	require.NoError(t, err)
	assert.Len(t, users, 0)
}

func TestHandler_NewHandler(t *testing.T) {
	usersRepo := inmemory.NewUserRepository()
	userService := services.NewUserService(usersRepo)
	responder := api.NewResponder()

	handler := NewHandler(userService, responder)

	require.NotNil(t, handler)
	assert.Equal(t, userService, handler.userService)
	assert.Equal(t, responder, handler.responder)
}
