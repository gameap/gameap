package middlewares

import (
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/gameap/gameap/internal/domain"
	"github.com/gameap/gameap/internal/repositories/inmemory"
	"github.com/gameap/gameap/pkg/api"
	"github.com/gameap/gameap/pkg/auth"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestPersonalAccessMiddleware_Middleware(t *testing.T) {
	// Setup
	tokenRepo := inmemory.NewPersonalAccessTokenRepository()
	responder := api.NewResponder()
	middleware := NewPersonalAccessMiddleware(tokenRepo, responder)

	// Create a test handler that will be wrapped by the middleware
	testHandler := http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte("success"))
	})

	tests := []struct {
		name              string
		session           *auth.Session
		requiredAbilities []domain.PATAbility
		wantStatus        int
		wantBody          string
	}{
		{
			name: "non_token_session_should_pass_through",
			session: &auth.Session{
				User: &domain.User{ID: 1, Login: "testuser"},
			},
			requiredAbilities: []domain.PATAbility{"read:users"},
			wantStatus:        http.StatusOK,
			wantBody:          "success",
		},
		{
			name:              "nil_session_should_pass_through",
			session:           nil,
			requiredAbilities: []domain.PATAbility{"read:users"},
			wantStatus:        http.StatusOK,
			wantBody:          "success",
		},
		{
			name: "token_session_with_nil_abilities_should_be_forbidden",
			session: &auth.Session{
				Token: &domain.PersonalAccessToken{
					ID:        1,
					Abilities: nil,
				},
			},
			requiredAbilities: []domain.PATAbility{"read:users"},
			wantStatus:        http.StatusForbidden,
			wantBody:          `{"status":"error","error":"token abilities are not configured","message":"token abilities are not configured","http_code":403}` + "\n",
		},
		{
			name: "token_session_with_empty_abilities_when_abilities_required_should_be_forbidden",
			session: &auth.Session{
				Token: &domain.PersonalAccessToken{
					ID:        1,
					Abilities: &[]domain.PATAbility{},
				},
			},
			requiredAbilities: []domain.PATAbility{"read:users"},
			wantStatus:        http.StatusForbidden,
			wantBody:          `{"status":"error","error":"insufficient token abilities","message":"insufficient token abilities","http_code":403}` + "\n",
		},
		{
			name: "token_session_with_insufficient_abilities_should_be_forbidden",
			session: &auth.Session{
				Token: &domain.PersonalAccessToken{
					ID: 1,
					Abilities: &[]domain.PATAbility{
						"read:users",
					},
				},
			},
			requiredAbilities: []domain.PATAbility{"read:users", "write:users"},
			wantStatus:        http.StatusForbidden,
			wantBody:          `{"status":"error","error":"insufficient token abilities","message":"insufficient token abilities","http_code":403}` + "\n",
		},
		{
			name: "token_session_with_different_abilities_should_be_forbidden",
			session: &auth.Session{
				Token: &domain.PersonalAccessToken{
					ID: 1,
					Abilities: &[]domain.PATAbility{
						"read:servers",
						"write:servers",
					},
				},
			},
			requiredAbilities: []domain.PATAbility{"read:users"},
			wantStatus:        http.StatusForbidden,
			wantBody:          `{"status":"error","error":"missing required ability: read:users","message":"missing required ability: read:users","http_code":403}` + "\n",
		},
		{
			name: "token_session_with_matching_abilities_should_pass",
			session: &auth.Session{
				Token: &domain.PersonalAccessToken{
					ID: 1,
					Abilities: &[]domain.PATAbility{
						"read:users",
						"write:users",
					},
				},
			},
			requiredAbilities: []domain.PATAbility{"read:users", "write:users"},
			wantStatus:        http.StatusOK,
			wantBody:          "success",
		},
		{
			name: "token_session_with_superset_of_required_abilities_should_pass",
			session: &auth.Session{
				Token: &domain.PersonalAccessToken{
					ID: 1,
					Abilities: &[]domain.PATAbility{
						"read:users",
						"write:users",
						"delete:users",
						"read:servers",
					},
				},
			},
			requiredAbilities: []domain.PATAbility{"read:users", "write:users"},
			wantStatus:        http.StatusOK,
			wantBody:          "success",
		},
		{
			name: "token_session_with_single_matching_ability_should_pass",
			session: &auth.Session{
				Token: &domain.PersonalAccessToken{
					ID: 1,
					Abilities: &[]domain.PATAbility{
						"read:users",
					},
				},
			},
			requiredAbilities: []domain.PATAbility{"read:users"},
			wantStatus:        http.StatusOK,
			wantBody:          "success",
		},
		{
			name: "token_session_with_empty_required_abilities_should_pass",
			session: &auth.Session{
				Token: &domain.PersonalAccessToken{
					ID: 1,
					Abilities: &[]domain.PATAbility{
						"read:users",
					},
				},
			},
			requiredAbilities: []domain.PATAbility{},
			wantStatus:        http.StatusOK,
			wantBody:          "success",
		},
		{
			name: "token_session_with_nil_abilities_and_no_required_abilities_should_be_forbidden",
			session: &auth.Session{
				Token: &domain.PersonalAccessToken{
					ID:        1,
					Abilities: nil,
				},
			},
			requiredAbilities: []domain.PATAbility{},
			wantStatus:        http.StatusForbidden,
			wantBody:          `{"status":"error","error":"token abilities are not configured","message":"token abilities are not configured","http_code":403}` + "\n",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// Create request
			req := httptest.NewRequest(http.MethodGet, "/test", nil)

			// Add session to context if provided
			if tt.session != nil {
				ctx := auth.ContextWithSession(req.Context(), tt.session)
				req = req.WithContext(ctx)
			}

			// Create response recorder
			rr := httptest.NewRecorder()

			// Wrap handler with middleware
			handler := middleware.Middleware(testHandler, tt.requiredAbilities)

			// Execute request
			handler.ServeHTTP(rr, req)

			// Assert response
			assert.Equal(t, tt.wantStatus, rr.Code, "unexpected status code")
			assert.Equal(t, tt.wantBody, rr.Body.String(), "unexpected response body")
		})
	}
}

func TestPersonalAccessMiddleware_Middleware_ContextPropagation(t *testing.T) {
	tokenRepo := inmemory.NewPersonalAccessTokenRepository()
	responder := api.NewResponder()
	middleware := NewPersonalAccessMiddleware(tokenRepo, responder)

	// Test that context values are properly propagated through the middleware
	testHandler := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		// Check if the session is still in the context
		session := auth.SessionFromContext(r.Context())
		require.NotNil(t, session)
		require.NotNil(t, session.User)
		assert.Equal(t, uint(1), session.User.ID)
		assert.Equal(t, "testuser", session.User.Login)

		w.WriteHeader(http.StatusOK)
	})

	req := httptest.NewRequest(http.MethodGet, "/test", nil)

	// Add a non-token session to context
	session := &auth.Session{
		User: &domain.User{ID: 1, Login: "testuser"},
	}
	ctx := auth.ContextWithSession(req.Context(), session)
	req = req.WithContext(ctx)

	rr := httptest.NewRecorder()
	handler := middleware.Middleware(testHandler, []domain.PATAbility{"read:users"})
	handler.ServeHTTP(rr, req)

	assert.Equal(t, http.StatusOK, rr.Code)
}

func TestNewPersonalAccessMiddleware(t *testing.T) {
	tokenRepo := inmemory.NewPersonalAccessTokenRepository()
	responder := api.NewResponder()
	middleware := NewPersonalAccessMiddleware(tokenRepo, responder)

	assert.NotNil(t, middleware)
	assert.NotNil(t, middleware.tokenRepo)
	assert.NotNil(t, middleware.responder)
}

func TestPersonalAccessMiddleware_EdgeCases(t *testing.T) {
	tokenRepo := inmemory.NewPersonalAccessTokenRepository()
	responder := api.NewResponder()
	middleware := NewPersonalAccessMiddleware(tokenRepo, responder)

	testHandler := http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusOK)
	})

	t.Run("duplicate_required_abilities_causes_forbidden_due_to_length_check", func(t *testing.T) {
		session := &auth.Session{
			Token: &domain.PersonalAccessToken{
				ID: 1,
				Abilities: &[]domain.PATAbility{
					"read:users",
					"write:users",
				},
			},
		}

		req := httptest.NewRequest(http.MethodGet, "/test", nil)
		ctx := auth.ContextWithSession(req.Context(), session)
		req = req.WithContext(ctx)

		rr := httptest.NewRecorder()

		// Pass duplicate required abilities - this will fail because the middleware
		// checks if len(token.Abilities) < len(requiredAbilities)
		// With duplicates: len(["read:users", "write:users"]) < len(["read:users", "read:users", "write:users"])
		// 2 < 3 = true, so it returns forbidden
		handler := middleware.Middleware(testHandler, []domain.PATAbility{
			"read:users",
			"read:users",
			"write:users",
		})

		handler.ServeHTTP(rr, req)
		// Due to the length check in the middleware, this will be forbidden
		assert.Equal(t, http.StatusForbidden, rr.Code)
	})

	t.Run("token_with_zero_id_is_not_token_session", func(t *testing.T) {
		session := &auth.Session{
			Token: &domain.PersonalAccessToken{
				ID: 0, // Zero ID means it's not a valid token session
				Abilities: &[]domain.PATAbility{
					"read:users",
				},
			},
		}

		req := httptest.NewRequest(http.MethodGet, "/test", nil)
		ctx := auth.ContextWithSession(req.Context(), session)
		req = req.WithContext(ctx)

		rr := httptest.NewRecorder()
		handler := middleware.Middleware(testHandler, []domain.PATAbility{"read:users"})

		handler.ServeHTTP(rr, req)

		// Should pass through because IsTokenSession() returns false for ID == 0
		assert.Equal(t, http.StatusOK, rr.Code)
	})
}
