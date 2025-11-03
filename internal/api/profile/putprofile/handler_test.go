package putprofile

import (
	"bytes"
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/gameap/gameap/internal/domain"
	"github.com/gameap/gameap/internal/repositories/inmemory"
	"github.com/gameap/gameap/internal/services"
	"github.com/gameap/gameap/pkg/api"
	"github.com/gameap/gameap/pkg/auth"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestHandler_ServeHTTP(t *testing.T) {
	tests := []struct {
		name           string
		setupAuth      func() context.Context
		setupRepo      func(*inmemory.UserRepository)
		requestBody    string
		expectedStatus int
		wantError      string
		expectSuccess  bool
		validateUser   func(t *testing.T, repo *inmemory.UserRepository)
	}{
		{
			name: "successful name update",
			setupAuth: func() context.Context {
				session := &auth.Session{
					Login: "testuser",
					Email: "test@example.com",
				}

				return auth.ContextWithSession(context.Background(), session)
			},
			setupRepo: func(repo *inmemory.UserRepository) {
				hashedPassword, _ := auth.HashPassword("password123")
				user := &domain.User{
					ID:       1,
					Login:    "testuser",
					Email:    "test@example.com",
					Password: hashedPassword,
					Name:     nil,
				}
				require.NoError(t, repo.Save(context.Background(), user))
			},
			requestBody:    `{"name": "Updated TokenName"}`,
			expectedStatus: http.StatusOK,
			expectSuccess:  true,
			validateUser: func(t *testing.T, repo *inmemory.UserRepository) {
				t.Helper()

				users, err := repo.FindAll(context.Background(), nil, nil)
				require.NoError(t, err)
				require.Len(t, users, 1)
				require.NotNil(t, users[0].Name)
				assert.Equal(t, "Updated TokenName", *users[0].Name)
			},
		},
		{
			name: "successful password update",
			setupAuth: func() context.Context {
				session := &auth.Session{
					Login: "testuser",
					Email: "test@example.com",
				}

				return auth.ContextWithSession(context.Background(), session)
			},
			setupRepo: func(repo *inmemory.UserRepository) {
				hashedPassword, _ := auth.HashPassword("oldpassword")
				user := &domain.User{
					ID:       1,
					Login:    "testuser",
					Email:    "test@example.com",
					Password: hashedPassword,
				}
				require.NoError(t, repo.Save(context.Background(), user))
			},
			requestBody:    `{"password": "newpassword123", "current_password": "oldpassword"}`,
			expectedStatus: http.StatusOK,
			expectSuccess:  true,
			validateUser: func(t *testing.T, repo *inmemory.UserRepository) {
				t.Helper()

				users, err := repo.FindAll(context.Background(), nil, nil)
				require.NoError(t, err)
				require.Len(t, users, 1)
				// Verify new password works
				err = auth.VerifyPassword(users[0].Password, "newpassword123")
				assert.NoError(t, err)
			},
		},
		{
			name: "successful name and password update",
			setupAuth: func() context.Context {
				session := &auth.Session{
					Login: "testuser",
					Email: "test@example.com",
				}

				return auth.ContextWithSession(context.Background(), session)
			},
			setupRepo: func(repo *inmemory.UserRepository) {
				hashedPassword, _ := auth.HashPassword("oldpassword")
				originalName := "Old TokenName"
				user := &domain.User{
					ID:       1,
					Login:    "testuser",
					Email:    "test@example.com",
					Password: hashedPassword,
					Name:     &originalName,
				}
				require.NoError(t, repo.Save(context.Background(), user))
			},
			requestBody:    `{"name": "New TokenName", "password": "newpassword123", "current_password": "oldpassword"}`,
			expectedStatus: http.StatusOK,
			expectSuccess:  true,
			validateUser: func(t *testing.T, repo *inmemory.UserRepository) {
				t.Helper()

				users, err := repo.FindAll(context.Background(), nil, nil)
				require.NoError(t, err)
				require.Len(t, users, 1)
				require.NotNil(t, users[0].Name)
				assert.Equal(t, "New TokenName", *users[0].Name)
				err = auth.VerifyPassword(users[0].Password, "newpassword123")
				assert.NoError(t, err)
			},
		},
		{
			name:           "user not authenticated",
			setupRepo:      func(_ *inmemory.UserRepository) {},
			requestBody:    `{"name": "Updated TokenName"}`,
			expectedStatus: http.StatusUnauthorized,
			wantError:      "user not authenticated",
			expectSuccess:  false,
		},
		{
			name: "authenticated user not found in database",
			setupAuth: func() context.Context {
				session := &auth.Session{
					Login: "nonexistent",
					Email: "nonexistent@example.com",
				}

				return auth.ContextWithSession(context.Background(), session)
			},
			setupRepo:      func(_ *inmemory.UserRepository) {},
			requestBody:    `{"name": "Updated TokenName"}`,
			expectedStatus: http.StatusNotFound,
			wantError:      "user not found",
			expectSuccess:  false,
		},
		{
			name: "invalid JSON request body",
			setupAuth: func() context.Context {
				session := &auth.Session{
					Login: "testuser",
					Email: "test@example.com",
				}

				return auth.ContextWithSession(context.Background(), session)
			},
			setupRepo: func(repo *inmemory.UserRepository) {
				user := &domain.User{
					ID:    1,
					Login: "testuser",
					Email: "test@example.com",
				}
				require.NoError(t, repo.Save(context.Background(), user))
			},
			requestBody:    `{"name": "Updated TokenName"`, // Invalid JSON
			expectedStatus: http.StatusBadRequest,
			wantError:      "invalid request",
			expectSuccess:  false,
		},
		{
			name: "name too long",
			setupAuth: func() context.Context {
				session := &auth.Session{
					Login: "testuser",
					Email: "test@example.com",
				}

				return auth.ContextWithSession(context.Background(), session)
			},
			setupRepo: func(repo *inmemory.UserRepository) {
				user := &domain.User{
					ID:    1,
					Login: "testuser",
					Email: "test@example.com",
				}
				require.NoError(t, repo.Save(context.Background(), user))
			},
			requestBody:    `{"name": "` + strings.Repeat("a", 256) + `"}`,
			expectedStatus: http.StatusBadRequest,
			wantError:      "name must not exceed 255 characters",
			expectSuccess:  false,
		},
		{
			name: "empty name",
			setupAuth: func() context.Context {
				session := &auth.Session{
					Login: "testuser",
					Email: "test@example.com",
				}

				return auth.ContextWithSession(context.Background(), session)
			},
			setupRepo: func(repo *inmemory.UserRepository) {
				user := &domain.User{
					ID:    1,
					Login: "testuser",
					Email: "test@example.com",
				}
				require.NoError(t, repo.Save(context.Background(), user))
			},
			requestBody:    `{"name": ""}`,
			expectedStatus: http.StatusBadRequest,
			wantError:      "name cannot be empty",
			expectSuccess:  false,
		},
		{
			name: "password too short",
			setupAuth: func() context.Context {
				session := &auth.Session{
					Login: "testuser",
					Email: "test@example.com",
				}

				return auth.ContextWithSession(context.Background(), session)
			},
			setupRepo: func(repo *inmemory.UserRepository) {
				hashedPassword, _ := auth.HashPassword("oldpassword")
				user := &domain.User{
					ID:       1,
					Login:    "testuser",
					Email:    "test@example.com",
					Password: hashedPassword,
				}
				require.NoError(t, repo.Save(context.Background(), user))
			},
			requestBody:    `{"password": "short", "current_password": "oldpassword"}`,
			expectedStatus: http.StatusBadRequest,
			wantError:      "password must be at least 8 characters long",
			expectSuccess:  false,
		},
		{
			name: "password change without current password",
			setupAuth: func() context.Context {
				session := &auth.Session{
					Login: "testuser",
					Email: "test@example.com",
				}

				return auth.ContextWithSession(context.Background(), session)
			},
			setupRepo: func(repo *inmemory.UserRepository) {
				hashedPassword, _ := auth.HashPassword("oldpassword")
				user := &domain.User{
					ID:       1,
					Login:    "testuser",
					Email:    "test@example.com",
					Password: hashedPassword,
				}
				require.NoError(t, repo.Save(context.Background(), user))
			},
			requestBody:    `{"password": "newpassword123"}`,
			expectedStatus: http.StatusBadRequest,
			wantError:      "current password is required for password change",
			expectSuccess:  false,
		},
		{
			name: "incorrect current password",
			setupAuth: func() context.Context {
				session := &auth.Session{
					Login: "testuser",
					Email: "test@example.com",
				}

				return auth.ContextWithSession(context.Background(), session)
			},
			setupRepo: func(repo *inmemory.UserRepository) {
				hashedPassword, _ := auth.HashPassword("oldpassword")
				user := &domain.User{
					ID:       1,
					Login:    "testuser",
					Email:    "test@example.com",
					Password: hashedPassword,
				}
				require.NoError(t, repo.Save(context.Background(), user))
			},
			requestBody:    `{"password": "newpassword123", "current_password": "wrongpassword"}`,
			expectedStatus: http.StatusBadRequest,
			wantError:      "current password is incorrect",
			expectSuccess:  false,
		},
		{
			name: "name with whitespace gets trimmed",
			setupAuth: func() context.Context {
				session := &auth.Session{
					Login: "testuser",
					Email: "test@example.com",
				}

				return auth.ContextWithSession(context.Background(), session)
			},
			setupRepo: func(repo *inmemory.UserRepository) {
				user := &domain.User{
					ID:    1,
					Login: "testuser",
					Email: "test@example.com",
				}
				require.NoError(t, repo.Save(context.Background(), user))
			},
			requestBody:    `{"name": "  Trimmed TokenName  "}`,
			expectedStatus: http.StatusOK,
			expectSuccess:  true,
			validateUser: func(t *testing.T, repo *inmemory.UserRepository) {
				t.Helper()

				users, err := repo.FindAll(context.Background(), nil, nil)
				require.NoError(t, err)
				require.Len(t, users, 1)
				require.NotNil(t, users[0].Name)
				assert.Equal(t, "Trimmed TokenName", *users[0].Name)
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			repo := inmemory.NewUserRepository()
			userService := services.NewUserService(repo)
			responder := api.NewResponder()
			handler := NewHandler(userService, responder)

			if tt.setupRepo != nil {
				tt.setupRepo(repo)
			}

			ctx := context.Background()

			if tt.setupAuth != nil {
				ctx = tt.setupAuth()
			}

			req := httptest.NewRequest(http.MethodPut, "/api/profile", bytes.NewBufferString(tt.requestBody))
			req = req.WithContext(ctx)
			req.Header.Set("Content-Type", "application/json")
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

			if tt.expectSuccess {
				var response updateProfileResponse
				require.NoError(t, json.Unmarshal(w.Body.Bytes(), &response))
				assert.Equal(t, "Profile updated successfully", response.Message)
			}

			if tt.validateUser != nil {
				tt.validateUser(t, repo)
			}
		})
	}
}

func TestUpdateProfileInput_Validate(t *testing.T) {
	tests := []struct {
		name      string
		input     updateProfileInput
		wantError string
	}{
		{
			name: "valid name only",
			input: updateProfileInput{
				Name: stringPtr("Valid TokenName"),
			},
			wantError: "",
		},
		{
			name: "valid password only",
			input: updateProfileInput{
				Password:        stringPtr("validpassword123"),
				CurrentPassword: stringPtr("currentpassword"),
			},
			wantError: "",
		},
		{
			name: "valid name and password",
			input: updateProfileInput{
				Name:            stringPtr("Valid TokenName"),
				Password:        stringPtr("validpassword123"),
				CurrentPassword: stringPtr("currentpassword"),
			},
			wantError: "",
		},
		{
			name: "empty name",
			input: updateProfileInput{
				Name: stringPtr(""),
			},
			wantError: "name cannot be empty",
		},
		{
			name: "name too long",
			input: updateProfileInput{
				Name: stringPtr(strings.Repeat("a", 256)),
			},
			wantError: "name must not exceed 255 characters",
		},
		{
			name: "password too short",
			input: updateProfileInput{
				Password:        stringPtr("short"),
				CurrentPassword: stringPtr("currentpassword"),
			},
			wantError: "password must be at least 8 characters long",
		},
		{
			name: "password too long",
			input: updateProfileInput{
				Password:        stringPtr(strings.Repeat("a", 65)),
				CurrentPassword: stringPtr("currentpassword"),
			},
			wantError: "password must not exceed 64 characters",
		},
		{
			name: "empty password",
			input: updateProfileInput{
				Password:        stringPtr(""),
				CurrentPassword: stringPtr("currentpassword"),
			},
			wantError: "password cannot be empty",
		},
		{
			name: "empty current password",
			input: updateProfileInput{
				Password:        stringPtr("validpassword123"),
				CurrentPassword: stringPtr(""),
			},
			wantError: "current password cannot be empty",
		},
		{
			name: "name with whitespace gets trimmed",
			input: updateProfileInput{
				Name: stringPtr("  Valid TokenName  "),
			},
			wantError: "",
		},
		{
			name: "whitespace only name becomes empty",
			input: updateProfileInput{
				Name: stringPtr("   "),
			},
			wantError: "name cannot be empty",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := tt.input.Validate()

			if tt.wantError != "" {
				require.Error(t, err)
				assert.Contains(t, err.Error(), tt.wantError)
			} else {
				assert.NoError(t, err)
			}

			// Test name trimming
			if tt.input.Name != nil && tt.wantError == "" && strings.Contains(tt.name, "trimmed") {
				assert.Equal(t, "Valid TokenName", *tt.input.Name)
			}
		})
	}
}

func TestNewUpdateProfileResponse(t *testing.T) {
	response := newUpdateProfileResponse()
	assert.Equal(t, "Profile updated successfully", response.Message)
}

// Helper function to create string pointers.
func stringPtr(s string) *string {
	return &s
}
