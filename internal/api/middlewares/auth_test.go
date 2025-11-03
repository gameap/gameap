package middlewares

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
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

const testJWTSecret = "test-secret-key-for-testing"

func TestAuthMiddleware_Middleware(t *testing.T) {
	// Setup test user
	userRepo := inmemory.NewUserRepository()
	tokenRepo := inmemory.NewPersonalAccessTokenRepository()
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
	_ = userRepo.Save(context.Background(), testUser)

	// Setup JWT service and generate token
	jwtService := auth.NewJWTService([]byte(testJWTSecret))
	validToken, _ := jwtService.GenerateTokenForUser(testUser, 24*time.Hour)
	expiredToken, _ := jwtService.GenerateTokenForUser(testUser, -1*time.Hour)
	invalidUserToken, _ := jwtService.GenerateTokenForUser(&domain.User{ID: 999, Login: "invalid"}, 24*time.Hour)

	tests := []struct {
		name       string
		authHeader string
		queryParam string
		cookie     *http.Cookie
		wantStatus int
		wantUser   bool
		wantError  string
	}{
		{
			name:       "valid token in Authorization header",
			authHeader: "Bearer " + validToken,
			wantStatus: http.StatusOK,
			wantUser:   true,
		},
		{
			name:       "valid token without Bearer prefix",
			authHeader: validToken,
			wantStatus: http.StatusUnauthorized,
			wantUser:   false,
			wantError:  "missing authentication token",
		},
		{
			name:       "valid token in query parameter",
			queryParam: validToken,
			wantStatus: http.StatusOK,
			wantUser:   true,
		},
		{
			name: "valid token in cookie",
			cookie: &http.Cookie{
				Name:  "token",
				Value: validToken,
			},
			wantStatus: http.StatusOK,
			wantUser:   true,
		},
		{
			name:       "missing token",
			wantStatus: http.StatusUnauthorized,
			wantUser:   false,
			wantError:  "missing authentication token",
		},
		{
			name:       "expired token",
			authHeader: "Bearer " + expiredToken,
			wantStatus: http.StatusUnauthorized,
			wantUser:   false,
			wantError:  "invalid or expired token",
		},
		{
			name:       "invalid token format",
			authHeader: "Bearer invalid-token",
			wantStatus: http.StatusUnauthorized,
			wantUser:   false,
			wantError:  "invalid or expired token",
		},
		{
			name:       "token for non-existent user",
			authHeader: "Bearer " + invalidUserToken,
			wantStatus: http.StatusUnauthorized,
			wantUser:   false,
			wantError:  "user not found",
		},
		{
			name:       "malformed Authorization header",
			authHeader: "InvalidScheme " + validToken,
			wantStatus: http.StatusUnauthorized,
			wantUser:   false,
			wantError:  "missing authentication token",
		},
		{
			name:       "empty Bearer token",
			authHeader: "Bearer ",
			wantStatus: http.StatusUnauthorized,
			wantUser:   false,
			wantError:  "missing authentication token",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// Setup middleware
			responder := api.NewResponder()
			authMiddleware := NewAuthMiddleware(auth.NewJWTService([]byte(testJWTSecret)), userRepo, tokenRepo, responder)

			var session *auth.Session

			// Create test handler that will be protected
			testHandler := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				session = auth.SessionFromContext(r.Context())
				w.WriteHeader(http.StatusOK)
				_, _ = w.Write([]byte("success"))
			})

			// Apply middleware
			protectedHandler := authMiddleware.Middleware(testHandler)

			// Create request
			url := "/protected"
			if tt.queryParam != "" {
				url += "?token=" + tt.queryParam
			}
			req := httptest.NewRequest(http.MethodGet, url, nil)
			if tt.authHeader != "" {
				req.Header.Set("Authorization", tt.authHeader)
			}
			if tt.cookie != nil {
				req.AddCookie(tt.cookie)
			}

			// Execute request
			w := httptest.NewRecorder()
			protectedHandler.ServeHTTP(w, req)

			// Assert status
			assert.Equal(t, tt.wantStatus, w.Code)

			if tt.wantUser {
				require.NotNil(t, session)
				assert.Equal(t, testUser.Login, session.Login)
				assert.Equal(t, testUser.Email, session.Email)
			} else {
				assert.Nil(t, session)
			}

			if tt.wantError != "" {
				var response map[string]any
				require.NoError(t, json.Unmarshal(w.Body.Bytes(), &response))
				assert.Equal(t, "error", response["status"])
				assert.Contains(t, response["error"], tt.wantError)
			}
		})
	}
}

func TestAuthMiddleware_OptionalMiddleware(t *testing.T) {
	// Setup test user
	userRepo := inmemory.NewUserRepository()
	tokenRepo := inmemory.NewPersonalAccessTokenRepository()
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
	_ = userRepo.Save(context.Background(), testUser)

	// Setup JWT service and generate tokens
	jwtService := auth.NewJWTService([]byte(testJWTSecret))
	validToken, _ := jwtService.GenerateTokenForUser(testUser, 24*time.Hour)
	expiredToken, _ := jwtService.GenerateTokenForUser(testUser, -1*time.Hour)

	tests := []struct {
		name       string
		authHeader string
		wantStatus int
		wantError  string
		wantUser   bool
	}{
		{
			name:       "valid token",
			authHeader: "Bearer " + validToken,
			wantStatus: http.StatusOK,
			wantUser:   true,
		},
		{
			name:       "no token - should still pass",
			authHeader: "",
			wantStatus: http.StatusOK,
			wantUser:   false,
		},
		{
			name:       "expired token - should still pass but without user",
			authHeader: "Bearer " + expiredToken,
			wantStatus: http.StatusOK,
			wantUser:   false,
		},
		{
			name:       "invalid token - shouldn't pass",
			authHeader: "Bearer invalid-token",
			wantStatus: http.StatusUnauthorized,
			wantError:  "invalid or expired token",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// Setup middleware
			responder := api.NewResponder()
			authMiddleware := NewAuthMiddleware(auth.NewJWTService([]byte(testJWTSecret)), userRepo, tokenRepo, responder)

			var session *auth.Session
			testHandler := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				// capturedUser, _ = GetUserFromContext(r.Context())
				session = auth.SessionFromContext(r.Context())
				w.WriteHeader(http.StatusOK)
				_, _ = w.Write([]byte("success"))
			})

			// Apply optional middleware
			protectedHandler := authMiddleware.OptionalMiddleware(testHandler)

			// Create request
			req := httptest.NewRequest(http.MethodGet, "/optional", nil)
			if tt.authHeader != "" {
				req.Header.Set("Authorization", tt.authHeader)
			}

			// Execute request
			w := httptest.NewRecorder()
			protectedHandler.ServeHTTP(w, req)

			// Assert
			assert.Equal(t, tt.wantStatus, w.Code)

			if tt.wantError == "" {
				assert.Equal(t, "success", w.Body.String())
			} else {
				assert.Contains(t, w.Body.String(), tt.wantError)
			}

			if tt.wantUser {
				assert.NotNil(t, session)
				assert.Equal(t, testUser.Login, session.Login)
				assert.Equal(t, testUser.Email, session.Email)
			} else {
				assert.Nil(t, session)
			}
		})
	}
}

func TestTokenExtractionPriority(t *testing.T) {
	// Setup
	userRepo := inmemory.NewUserRepository()
	tokenRepo := inmemory.NewPersonalAccessTokenRepository()
	hashedPassword, _ := auth.HashPassword("password123")
	now := time.Now()
	testUser1 := &domain.User{
		ID:        1,
		Login:     "user1",
		Email:     "user1@example.com",
		Password:  hashedPassword,
		Name:      lo.ToPtr("User1"),
		CreatedAt: &now,
		UpdatedAt: &now,
	}
	_ = userRepo.Save(context.Background(), testUser1)

	// Do not save user2 to simulate non-existent users
	testUser2 := &domain.User{
		ID:        2,
		Login:     "user2",
		Email:     "user2@example.com",
		Password:  hashedPassword,
		Name:      lo.ToPtr("User2"),
		CreatedAt: &now,
		UpdatedAt: &now,
	}

	// Do not save user3 to simulate non-existent users
	testUser3 := &domain.User{
		ID:        3,
		Login:     "user3",
		Email:     "user3@example.com",
		Password:  hashedPassword,
		Name:      lo.ToPtr("User3"),
		CreatedAt: &now,
		UpdatedAt: &now,
	}

	jwtService := auth.NewJWTService([]byte(testJWTSecret))
	tokenHeader, _ := jwtService.GenerateTokenForUser(testUser1, 24*time.Hour)
	tokenQuery, _ := jwtService.GenerateTokenForUser(testUser2, 24*time.Hour)
	tokenCookie, _ := jwtService.GenerateTokenForUser(testUser3, 24*time.Hour)

	responder := api.NewResponder()
	authMiddleware := NewAuthMiddleware(auth.NewJWTService([]byte(testJWTSecret)), userRepo, tokenRepo, responder)

	// Test handler
	testHandler := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		session := auth.SessionFromContext(r.Context())
		if session != nil {
			response := map[string]string{"login": session.Login}
			require.NoError(t, json.NewEncoder(w).Encode(response))
		} else {
			w.WriteHeader(http.StatusUnauthorized)
			_, _ = w.Write([]byte("no user in context"))
		}
	})

	protectedHandler := authMiddleware.Middleware(testHandler)

	// Test 1: Authorization header should have highest priority
	req := httptest.NewRequest(http.MethodGet, "/test?token="+tokenQuery, nil)
	req.Header.Set("Authorization", "Bearer "+tokenHeader)
	req.AddCookie(&http.Cookie{Name: "token", Value: tokenCookie})

	w := httptest.NewRecorder()
	protectedHandler.ServeHTTP(w, req)

	var response map[string]string
	_ = json.Unmarshal(w.Body.Bytes(), &response)
	assert.Equal(t, "user1", response["login"], "Authorization header should have highest priority")

	// Test 2: Query parameter should be used when no Authorization header
	req = httptest.NewRequest(http.MethodGet, "/test?token="+tokenQuery, nil)
	req.AddCookie(&http.Cookie{Name: "token", Value: tokenCookie})

	w = httptest.NewRecorder()
	protectedHandler.ServeHTTP(w, req)

	// Since user with ID 2 doesn't exist, this should fail
	assert.Equal(t, http.StatusUnauthorized, w.Code)

	// Test 3: Cookie should be used when no Authorization header or query param
	req = httptest.NewRequest(http.MethodGet, "/test", nil)
	req.AddCookie(&http.Cookie{Name: "token", Value: tokenCookie})

	w = httptest.NewRecorder()
	protectedHandler.ServeHTTP(w, req)

	// Since user with ID 3 doesn't exist, this should fail
	assert.Equal(t, http.StatusUnauthorized, w.Code)
}
