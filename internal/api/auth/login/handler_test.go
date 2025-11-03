package login

import (
	"bytes"
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/gameap/gameap/internal/domain"
	"github.com/gameap/gameap/internal/repositories/inmemory"
	"github.com/gameap/gameap/pkg/api"
	"github.com/gameap/gameap/pkg/auth"
	"github.com/samber/lo"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestHandler_ServeHTTP(t *testing.T) {
	hashedPassword, _ := auth.HashPassword("password123")
	now := time.Now()
	testUser := &domain.User{
		ID:        1,
		Login:     "testuser",
		Email:     "test@example.com",
		Password:  hashedPassword,
		Name:      lo.ToPtr("Test User"),
		CreatedAt: &now,
		UpdatedAt: &now,
	}

	tests := []struct {
		name           string
		setupRepo      func(*inmemory.UserRepository)
		requestBody    string
		expectedStatus int
		wantError      string
		checkResponse  func(*testing.T, map[string]any)
	}{
		{
			name: "successful login with username",
			setupRepo: func(repo *inmemory.UserRepository) {
				_ = repo.Save(context.Background(), testUser)
			},
			requestBody: `{
				"login": "testuser",
				"password": "password123"
			}`,
			expectedStatus: http.StatusOK,
			checkResponse: func(t *testing.T, response map[string]any) {
				t.Helper()

				assert.Contains(t, response, "token")
				assert.NotEmpty(t, response["token"])
				assert.Contains(t, response, "expires_in")
				assert.Contains(t, response, "user")

				user, ok := response["user"].(map[string]any)
				require.True(t, ok)
				assert.Equal(t, "testuser", user["login"])
				assert.Equal(t, "test@example.com", user["email"])
			},
		},
		{
			name: "successful login with email",
			setupRepo: func(repo *inmemory.UserRepository) {
				_ = repo.Save(context.Background(), testUser)
			},
			requestBody: `{
				"email": "test@example.com",
				"password": "password123"
			}`,
			expectedStatus: http.StatusOK,
			checkResponse: func(t *testing.T, response map[string]any) {
				t.Helper()

				assert.Contains(t, response, "token")
				assert.NotEmpty(t, response["token"])
			},
		},
		{
			name: "invalid password",
			setupRepo: func(repo *inmemory.UserRepository) {
				_ = repo.Save(context.Background(), testUser)
			},
			requestBody: `{
				"login": "testuser",
				"password": "wrongpassword"
			}`,
			expectedStatus: http.StatusUnauthorized,
			wantError:      "invalid credentials",
		},
		{
			name: "user not found",
			setupRepo: func(_ *inmemory.UserRepository) {
				// Don't add any users
			},
			requestBody: `{
				"login": "nonexistent",
				"password": "password123"
			}`,
			expectedStatus: http.StatusUnauthorized,
			wantError:      "invalid credentials",
		},
		{
			name:      "missing login field",
			setupRepo: func(_ *inmemory.UserRepository) {},
			requestBody: `{
				"password": "password123"
			}`,
			expectedStatus: http.StatusUnprocessableEntity,
			wantError:      "login or email fields are required",
		},
		{
			name:      "missing password field",
			setupRepo: func(_ *inmemory.UserRepository) {},
			requestBody: `{
				"login": "testuser"
			}`,
			expectedStatus: http.StatusUnprocessableEntity,
			wantError:      "password field is required",
		},
		{
			name:      "empty login field",
			setupRepo: func(_ *inmemory.UserRepository) {},
			requestBody: `{
				"login": "",
				"password": "password123"
			}`,
			expectedStatus: http.StatusUnprocessableEntity,
			wantError:      "login or email fields are required",
		},
		{
			name:      "empty password field",
			setupRepo: func(_ *inmemory.UserRepository) {},
			requestBody: `{
				"login": "testuser",
				"password": ""
			}`,
			expectedStatus: http.StatusUnprocessableEntity,
			wantError:      "password field is required",
		},
		{
			name:           "invalid JSON body",
			setupRepo:      func(_ *inmemory.UserRepository) {},
			requestBody:    `{"invalid": json}`,
			expectedStatus: http.StatusBadRequest,
			wantError:      "invalid request",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// ARRANGE
			repo := inmemory.NewUserRepository()
			if tt.setupRepo != nil {
				tt.setupRepo(repo)
			}
			responder := api.NewResponder()
			handler := NewHandler(auth.NewJWTService([]byte("test-secret-key")), repo, responder)

			body := []byte(tt.requestBody)

			req := httptest.NewRequest(http.MethodPost, "/auth/login", bytes.NewBuffer(body))
			req.Header.Set("Content-Type", "application/json")
			w := httptest.NewRecorder()

			// ACT
			handler.ServeHTTP(w, req)

			// ASSERT
			assert.Equal(t, tt.expectedStatus, w.Code)

			var response map[string]any
			require.NoError(t, json.Unmarshal(w.Body.Bytes(), &response))

			if tt.wantError != "" {
				assert.Equal(t, "error", response["status"])
				if errorMsg, ok := response["error"].(string); !ok || !strings.Contains(errorMsg, tt.wantError) {
					t.Errorf("Expected error containing '%s', got: %v", tt.wantError, response["error"])
				}
			} else if tt.checkResponse != nil {
				tt.checkResponse(t, response)
			}
		})
	}
}

func TestHandler_MultipleUsers(t *testing.T) {
	// ARRANGE
	repo := inmemory.NewUserRepository()
	responder := api.NewResponder()
	handler := NewHandler(auth.NewJWTService([]byte("test-secret-key")), repo, responder)

	// Create multiple users
	hashedPassword1, _ := auth.HashPassword("pass1")
	hashedPassword2, _ := auth.HashPassword("pass2")
	hashedPassword3, _ := auth.HashPassword("pass3")
	now := time.Now()

	user1 := &domain.User{
		ID:        1,
		Login:     "user1",
		Email:     "user1@example.com",
		Password:  hashedPassword1,
		Name:      lo.ToPtr("User One"),
		CreatedAt: &now,
		UpdatedAt: &now,
	}

	user2 := &domain.User{
		ID:        2,
		Login:     "user2",
		Email:     "user2@example.com",
		Password:  hashedPassword2,
		Name:      lo.ToPtr("User Two"),
		CreatedAt: &now,
		UpdatedAt: &now,
	}

	user3 := &domain.User{
		ID:        3,
		Login:     "user3",
		Email:     "user3@example.com",
		Password:  hashedPassword3,
		Name:      lo.ToPtr("User Three"),
		CreatedAt: &now,
		UpdatedAt: &now,
	}

	_ = repo.Save(context.Background(), user1)
	_ = repo.Save(context.Background(), user2)
	_ = repo.Save(context.Background(), user3)

	tests := []struct {
		name     string
		login    string
		email    string
		password string
	}{
		{
			name:     "login as user1 with username",
			login:    "user1",
			password: "pass1",
		},
		{
			name:     "login as user2 with username",
			login:    "user2",
			password: "pass2",
		},
		{
			name:     "login as user1 with email",
			email:    "user1@example.com",
			password: "pass1",
		},
		{
			name:     "login as user2 with email",
			email:    "user2@example.com",
			password: "pass2",
		},
		{
			name:     "login as user3 with username",
			login:    "user3",
			password: "pass3",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// ACT
			loginData := map[string]string{
				"login":    tt.login,
				"email":    tt.email,
				"password": tt.password,
			}
			body, _ := json.Marshal(loginData)
			req := httptest.NewRequest(http.MethodPost, "/auth/login", bytes.NewBuffer(body))
			req.Header.Set("Content-Type", "application/json")
			w := httptest.NewRecorder()

			handler.ServeHTTP(w, req)

			// ASSERT
			require.Equal(t, http.StatusOK, w.Code)

			var response map[string]any
			require.NoError(t, json.Unmarshal(w.Body.Bytes(), &response))

			_, ok := response["user"].(map[string]any)
			require.True(t, ok)
		})
	}
}

func TestHandler_SpecialCharacters(t *testing.T) {
	// ARRANGE
	repo := inmemory.NewUserRepository()
	responder := api.NewResponder()
	handler := NewHandler(auth.NewJWTService([]byte("test-secret-key")), repo, responder)

	// Create user with special characters
	specialPassword := "p@$$w0rd!#%&*()"
	hashedPassword, _ := auth.HashPassword(specialPassword)
	now := time.Now()

	user := &domain.User{
		ID:        1,
		Login:     "special.user-name_123",
		Email:     "special+tag@example.com",
		Password:  hashedPassword,
		Name:      lo.ToPtr("Special User"),
		CreatedAt: &now,
		UpdatedAt: &now,
	}

	_ = repo.Save(context.Background(), user)

	tests := []struct {
		name           string
		login          string
		email          string
		password       string
		expectedStatus int
	}{
		{
			name:           "login with special characters in username",
			login:          "special.user-name_123",
			password:       specialPassword,
			expectedStatus: http.StatusOK,
		},
		{
			name:           "login with special characters in email",
			email:          "special+tag@example.com",
			password:       specialPassword,
			expectedStatus: http.StatusOK,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// ACT
			loginData := map[string]string{
				"login":    tt.login,
				"email":    tt.email,
				"password": tt.password,
			}
			body, _ := json.Marshal(loginData)
			req := httptest.NewRequest(http.MethodPost, "/auth/login", bytes.NewBuffer(body))
			req.Header.Set("Content-Type", "application/json")
			w := httptest.NewRecorder()

			handler.ServeHTTP(w, req)

			// ASSERT
			assert.Equal(t, tt.expectedStatus, w.Code)
		})
	}
}

func TestHandler_TokenValidation(t *testing.T) {
	// ARRANGE
	repo := inmemory.NewUserRepository()
	responder := api.NewResponder()
	handler := NewHandler(auth.NewJWTService([]byte("test-secret-key")), repo, responder)

	hashedPassword, _ := auth.HashPassword("testpass")
	now := time.Now()
	user := &domain.User{
		ID:        42,
		Login:     "tokenuser",
		Email:     "token@test.com",
		Password:  hashedPassword,
		Name:      lo.ToPtr("Token User"),
		CreatedAt: &now,
		UpdatedAt: &now,
	}
	_ = repo.Save(context.Background(), user)

	// ACT
	loginData := map[string]string{
		"login":    "tokenuser",
		"password": "testpass",
	}
	body, err := json.Marshal(loginData)
	require.NoError(t, err)
	req := httptest.NewRequest(http.MethodPost, "/auth/login", bytes.NewBuffer(body))
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()

	handler.ServeHTTP(w, req)

	// ASSERT
	require.Equal(t, http.StatusOK, w.Code)

	var response map[string]any
	require.NoError(t, json.Unmarshal(w.Body.Bytes(), &response))

	// Verify token structure
	token, ok := response["token"].(string)
	require.True(t, ok)
	assert.NotEmpty(t, token)

	// JWT tokens have three parts separated by dots
	parts := strings.Split(token, ".")
	assert.Len(t, parts, 3, "JWT token should have three parts")

	// Verify expires_in is reasonable (24 hours = 86400 seconds)
	expiresIn, ok := response["expires_in"].(float64)
	require.True(t, ok)
	assert.Equal(t, float64(86400), expiresIn)
}
