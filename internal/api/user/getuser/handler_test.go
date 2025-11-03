package getuser

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/gameap/gameap/internal/domain"
	"github.com/gameap/gameap/pkg/api"
	"github.com/gameap/gameap/pkg/auth"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestHandler_ServeHTTP(t *testing.T) {
	tests := []struct {
		name           string
		setupAuth      func() context.Context
		expectedStatus int
		wantError      string
		expectUser     bool
	}{
		{
			name: "successful user retrieval",
			setupAuth: func() context.Context {
				now := time.Now()
				userName := "Test User"
				user := &domain.User{
					ID:        1,
					Login:     "testuser",
					Email:     "test@example.com",
					Name:      &userName,
					CreatedAt: &now,
					UpdatedAt: &now,
				}

				session := &auth.Session{
					Login: "testuser",
					Email: "test@example.com",
					User:  user,
				}

				return auth.ContextWithSession(context.Background(), session)
			},
			expectedStatus: http.StatusOK,
			expectUser:     true,
		},
		{
			name:           "user not authenticated",
			setupAuth:      context.Background,
			expectedStatus: http.StatusUnauthorized,
			wantError:      "user not authenticated",
			expectUser:     false,
		},
		{
			name: "user with minimal fields",
			setupAuth: func() context.Context {
				user := &domain.User{
					ID:    2,
					Login: "minimaluser",
					Email: "minimal@example.com",
				}

				session := &auth.Session{
					Login: "minimaluser",
					Email: "minimal@example.com",
					User:  user,
				}

				return auth.ContextWithSession(context.Background(), session)
			},
			expectedStatus: http.StatusOK,
			expectUser:     true,
		},
		{
			name: "user with all fields populated",
			setupAuth: func() context.Context {
				now := time.Now()
				userName := "John Doe"
				user := &domain.User{
					ID:        3,
					Login:     "johndoe",
					Email:     "john@example.com",
					Name:      &userName,
					CreatedAt: &now,
					UpdatedAt: &now,
				}

				session := &auth.Session{
					Login: "johndoe",
					Email: "john@example.com",
					User:  user,
				}

				return auth.ContextWithSession(context.Background(), session)
			},
			expectedStatus: http.StatusOK,
			expectUser:     true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			responder := api.NewResponder()
			handler := NewHandler(responder)

			ctx := tt.setupAuth()
			req := httptest.NewRequest(http.MethodGet, "/api/user", nil)
			req = req.WithContext(ctx)
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

			if tt.expectUser {
				var userResp userResponse
				require.NoError(t, json.Unmarshal(w.Body.Bytes(), &userResp))
				assert.NotZero(t, userResp.ID)
				assert.NotEmpty(t, userResp.Login)
				assert.NotEmpty(t, userResp.Email)
			}
		})
	}
}

func TestHandler_UserResponseFields(t *testing.T) {
	responder := api.NewResponder()
	handler := NewHandler(responder)

	now := time.Now()
	userName := "Jane Smith"
	user := &domain.User{
		ID:        10,
		Login:     "janesmith",
		Email:     "jane@example.com",
		Name:      &userName,
		CreatedAt: &now,
		UpdatedAt: &now,
	}

	session := &auth.Session{
		Login: "janesmith",
		Email: "jane@example.com",
		User:  user,
	}
	ctx := auth.ContextWithSession(context.Background(), session)

	req := httptest.NewRequest(http.MethodGet, "/api/user", nil)
	req = req.WithContext(ctx)
	w := httptest.NewRecorder()

	handler.ServeHTTP(w, req)

	require.Equal(t, http.StatusOK, w.Code)

	var userResp userResponse
	require.NoError(t, json.Unmarshal(w.Body.Bytes(), &userResp))

	assert.Equal(t, uint(10), userResp.ID)
	assert.Equal(t, "janesmith", userResp.Login)
	assert.Equal(t, "jane@example.com", userResp.Email)
	require.NotNil(t, userResp.Name)
	assert.Equal(t, "Jane Smith", *userResp.Name)
	require.NotNil(t, userResp.CreatedAt)
	require.NotNil(t, userResp.UpdatedAt)
}

func TestHandler_UserResponseWithNilFields(t *testing.T) {
	responder := api.NewResponder()
	handler := NewHandler(responder)

	user := &domain.User{
		ID:        5,
		Login:     "testuser",
		Email:     "test@example.com",
		Name:      nil,
		CreatedAt: nil,
		UpdatedAt: nil,
	}

	session := &auth.Session{
		Login: "testuser",
		Email: "test@example.com",
		User:  user,
	}
	ctx := auth.ContextWithSession(context.Background(), session)

	req := httptest.NewRequest(http.MethodGet, "/api/user", nil)
	req = req.WithContext(ctx)
	w := httptest.NewRecorder()

	handler.ServeHTTP(w, req)

	require.Equal(t, http.StatusOK, w.Code)

	var userResp userResponse
	require.NoError(t, json.Unmarshal(w.Body.Bytes(), &userResp))

	assert.Equal(t, uint(5), userResp.ID)
	assert.Equal(t, "testuser", userResp.Login)
	assert.Equal(t, "test@example.com", userResp.Email)
	assert.Nil(t, userResp.Name)
	assert.Nil(t, userResp.CreatedAt)
	assert.Nil(t, userResp.UpdatedAt)
}

func TestHandler_NewHandler(t *testing.T) {
	responder := api.NewResponder()

	handler := NewHandler(responder)

	require.NotNil(t, handler)
	assert.Equal(t, responder, handler.responder)
}

func TestNewUserResponseFromUser(t *testing.T) {
	now := time.Now()
	userName := "Test User"
	user := &domain.User{
		ID:        1,
		Login:     "testuser",
		Email:     "test@example.com",
		Name:      &userName,
		CreatedAt: &now,
		UpdatedAt: &now,
	}

	response := newUserResponseFromUser(user)

	assert.Equal(t, uint(1), response.ID)
	assert.Equal(t, "testuser", response.Login)
	assert.Equal(t, "test@example.com", response.Email)
	require.NotNil(t, response.Name)
	assert.Equal(t, "Test User", *response.Name)
	require.NotNil(t, response.CreatedAt)
	assert.Equal(t, now, *response.CreatedAt)
	require.NotNil(t, response.UpdatedAt)
	assert.Equal(t, now, *response.UpdatedAt)
}

func TestNewUserResponseFromUserWithNilFields(t *testing.T) {
	user := &domain.User{
		ID:        1,
		Login:     "testuser",
		Email:     "test@example.com",
		Name:      nil,
		CreatedAt: nil,
		UpdatedAt: nil,
	}

	response := newUserResponseFromUser(user)

	assert.Equal(t, uint(1), response.ID)
	assert.Equal(t, "testuser", response.Login)
	assert.Equal(t, "test@example.com", response.Email)
	assert.Nil(t, response.Name)
	assert.Nil(t, response.CreatedAt)
	assert.Nil(t, response.UpdatedAt)
}
