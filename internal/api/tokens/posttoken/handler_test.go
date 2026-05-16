package posttoken_test

import (
	"bytes"
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/gameap/gameap/internal/api/tokens/posttoken"
	"github.com/gameap/gameap/internal/audit"
	"github.com/gameap/gameap/internal/domain"
	"github.com/gameap/gameap/internal/filters"
	"github.com/gameap/gameap/internal/rbac"
	"github.com/gameap/gameap/internal/repositories/inmemory"
	"github.com/gameap/gameap/internal/services"
	"github.com/gameap/gameap/pkg/api"
	"github.com/gameap/gameap/pkg/auth"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// auditCapture is a concurrency-safe audit.Logger that records every event
// the handler emits (mirrors router_security_auditlog_test.go).
type auditCapture struct {
	mu     sync.Mutex
	events []audit.Event
}

func (a *auditCapture) Record(_ context.Context, e audit.Event) {
	a.mu.Lock()
	defer a.mu.Unlock()
	a.events = append(a.events, e)
}

func (a *auditCapture) snapshot() []audit.Event {
	a.mu.Lock()
	defer a.mu.Unlock()

	return append([]audit.Event(nil), a.events...)
}

func findEvent(events []audit.Event, t audit.EventType) (audit.Event, bool) {
	for _, e := range events {
		if e.Type == t {
			return e, true
		}
	}

	return audit.Event{}, false
}

func countEvents(events []audit.Event, t audit.EventType) int {
	n := 0
	for _, e := range events {
		if e.Type == t {
			n++
		}
	}

	return n
}

func extraString(e audit.Event, key string) (string, bool) {
	for _, a := range e.Extra {
		if a.Key == key {
			return a.Value.String(), true
		}
	}

	return "", false
}

func TestHandler_ServeHTTP(t *testing.T) {
	tests := []struct {
		name             string
		requestBody      any
		setupAuth        func(context.Context) context.Context
		setupRBAC        func(*inmemory.RBACRepository)
		wantStatus       int
		validateResponse func(*testing.T, map[string]any)
		wantErr          string
	}{
		{
			name: "successful token creation",
			requestBody: map[string]any{
				"token_name": "Test API Token",
				"abilities": []string{
					"server:start",
					"server:stop",
					"server:restart",
				},
			},
			setupAuth: func(ctx context.Context) context.Context {
				return auth.ContextWithSession(ctx, &auth.Session{
					Login: "testuser",
					Email: "test@example.com",
					User: &domain.User{
						ID:    1,
						Login: "testuser",
						Email: "test@example.com",
					},
				})
			},
			setupRBAC: func(rbacRepo *inmemory.RBACRepository) {
				// Setup admin role
				role := &domain.Role{
					ID:   1,
					Name: "admin",
				}
				_ = rbacRepo.SaveRole(context.Background(), role)

				// Setup admin ability
				ability := &domain.Ability{
					ID:   1,
					Name: domain.AbilityNameAdminRolesPermissions,
				}
				_ = rbacRepo.SaveAbility(context.Background(), ability)

				// Create permission linking role to ability
				entityType := domain.EntityTypeRole
				permission := &domain.Permission{
					AbilityID:  ability.ID,
					EntityID:   &role.ID,
					EntityType: &entityType,
					Ability:    ability,
				}
				_ = rbacRepo.SavePermission(context.Background(), permission)

				// Assign role to user
				assignedRole := &domain.AssignedRole{
					RoleID:     role.ID,
					EntityID:   2,
					EntityType: domain.EntityTypeUser,
				}
				_ = rbacRepo.SaveAssignedRole(context.Background(), assignedRole)
			},
			wantStatus: http.StatusOK,
			validateResponse: func(t *testing.T, response map[string]any) {
				t.Helper()

				assert.NotEmpty(t, response["token"])
				assert.Len(t, response["token"].(string), 50)
				assert.True(t, strings.HasPrefix(response["token"].(string), "1|"))
			},
		},
		{
			name: "creation with single ability",
			requestBody: map[string]any{
				"token_name": "Minimal Token",
				"abilities":  []string{"server:console"},
			},
			setupAuth: func(ctx context.Context) context.Context {
				return auth.ContextWithSession(ctx, &auth.Session{
					Login: "testuser",
					Email: "test@example.com",
					User: &domain.User{
						ID:    1,
						Login: "testuser",
						Email: "test@example.com",
					},
				})
			},
			wantStatus: http.StatusOK,
			validateResponse: func(t *testing.T, response map[string]any) {
				t.Helper()

				assert.NotEmpty(t, response["token"])
				assert.Len(t, response["token"].(string), 50)
				assert.True(t, strings.HasPrefix(response["token"].(string), "1|"))
			},
		},
		{
			name: "admin creating token with admin abilities",
			requestBody: map[string]any{
				"token_name": "Admin Token",
				"abilities": []string{
					"admin:server:create",
					"admin:gdaemon-task:read",
				},
			},
			setupAuth: func(ctx context.Context) context.Context {
				return auth.ContextWithSession(ctx, &auth.Session{
					Login: "admin",
					Email: "admin@example.com",
					User: &domain.User{
						ID:    2,
						Login: "admin",
						Email: "admin@example.com",
					},
				})
			},
			setupRBAC: func(rbacRepo *inmemory.RBACRepository) {
				// Setup admin role
				role := &domain.Role{
					ID:   1,
					Name: "admin",
				}
				_ = rbacRepo.SaveRole(context.Background(), role)

				// Setup admin ability
				ability := &domain.Ability{
					ID:   1,
					Name: domain.AbilityNameAdminRolesPermissions,
				}
				_ = rbacRepo.SaveAbility(context.Background(), ability)

				// Create permission linking role to ability
				entityType := domain.EntityTypeRole
				permission := &domain.Permission{
					AbilityID:  ability.ID,
					EntityID:   &role.ID,
					EntityType: &entityType,
					Ability:    ability,
				}
				_ = rbacRepo.SavePermission(context.Background(), permission)

				// Assign role to user
				assignedRole := &domain.AssignedRole{
					RoleID:     role.ID,
					EntityID:   2,
					EntityType: domain.EntityTypeUser,
				}
				_ = rbacRepo.SaveAssignedRole(context.Background(), assignedRole)
			},
			wantStatus: http.StatusOK,
			validateResponse: func(t *testing.T, response map[string]any) {
				t.Helper()

				assert.NotEmpty(t, response["token"])
				assert.Len(t, response["token"].(string), 50)
				assert.True(t, strings.HasPrefix(response["token"].(string), "1|"))
			},
		},
		{
			name: "non-admin creating token with mixed abilities fails",
			requestBody: map[string]any{
				"token_name": "Mixed Token",
				"abilities": []string{
					"server:start",
					"admin:server:create",
					"server:stop",
				},
			},
			setupAuth: func(ctx context.Context) context.Context {
				return auth.ContextWithSession(ctx, &auth.Session{
					Login: "user",
					Email: "user@example.com",
					User: &domain.User{
						ID:    4,
						Login: "user",
						Email: "user@example.com",
					},
				})
			},
			setupRBAC: func(rbacRepo *inmemory.RBACRepository) {
				// Regular user without admin permissions
				role := &domain.Role{
					ID:   3,
					Name: "user",
				}
				_ = rbacRepo.SaveRole(context.Background(), role)

				// Assign role to user
				assignedRole := &domain.AssignedRole{
					RoleID:     role.ID,
					EntityID:   4,
					EntityType: domain.EntityTypeUser,
				}
				_ = rbacRepo.SaveAssignedRole(context.Background(), assignedRole)
			},
			wantStatus: http.StatusUnprocessableEntity,
			wantErr:    "admin abilities require admin role",
		},
		{
			name: "non-admin creating token with admin abilities fails",
			requestBody: map[string]any{
				"token_name": "Admin Token",
				"abilities": []string{
					"admin:server:create",
					"admin:gdaemon-task:read",
				},
			},
			setupAuth: func(ctx context.Context) context.Context {
				return auth.ContextWithSession(ctx, &auth.Session{
					Login: "user",
					Email: "user@example.com",
					User: &domain.User{
						ID:    3,
						Login: "user",
						Email: "user@example.com",
					},
				})
			},
			setupRBAC: func(rbacRepo *inmemory.RBACRepository) {
				// Setup regular user role without admin permissions
				role := &domain.Role{
					ID:   2,
					Name: "user",
				}
				_ = rbacRepo.SaveRole(context.Background(), role)

				// Assign role to user
				assignedRole := &domain.AssignedRole{
					RoleID:     role.ID,
					EntityID:   3,
					EntityType: domain.EntityTypeUser,
				}
				_ = rbacRepo.SaveAssignedRole(context.Background(), assignedRole)
			},
			wantStatus: http.StatusUnprocessableEntity,
			wantErr:    "admin abilities require admin role",
		},
		{
			name: "token session",
			requestBody: map[string]any{
				"token_name": "Admin Token",
				"abilities": []string{
					"admin:server:create",
					"admin:gdaemon-task:read",
				},
			},
			setupAuth: func(ctx context.Context) context.Context {
				return auth.ContextWithSession(ctx, &auth.Session{
					Login: "admin",
					Email: "admin@example.com",
					User: &domain.User{
						ID:    2,
						Login: "admin",
						Email: "admin@example.com",
					},
					Token: &domain.PersonalAccessToken{
						ID: 1,
					},
				})
			},
			wantStatus: http.StatusForbidden,
			wantErr:    `token sessions cannot create new tokens`,
		},
		{
			name: "unauthenticated request",
			requestBody: map[string]any{
				"token_name": "Test Token",
				"abilities":  []string{"server:start"},
			},
			setupAuth: func(ctx context.Context) context.Context {
				return ctx
			},
			wantStatus: http.StatusUnauthorized,
			wantErr:    "user not authenticated",
		},
		{
			name: "missing name field",
			requestBody: map[string]any{
				"abilities": []string{"server:start"},
			},
			setupAuth: func(ctx context.Context) context.Context {
				return auth.ContextWithSession(ctx, &auth.Session{
					Login: "testuser",
					Email: "test@example.com",
					User: &domain.User{
						ID:    1,
						Login: "testuser",
						Email: "test@example.com",
					},
				})
			},
			wantStatus: http.StatusUnprocessableEntity,
			wantErr:    "name field is required",
		},
		{
			name: "name too long",
			requestBody: map[string]any{
				"token_name": string(make([]byte, 256)),
				"abilities":  []string{"server:start"},
			},
			setupAuth: func(ctx context.Context) context.Context {
				return auth.ContextWithSession(ctx, &auth.Session{
					Login: "testuser",
					Email: "test@example.com",
					User: &domain.User{
						ID:    1,
						Login: "testuser",
						Email: "test@example.com",
					},
				})
			},
			wantStatus: http.StatusUnprocessableEntity,
			wantErr:    "name must not exceed 255 characters",
		},
		{
			name: "missing abilities field",
			requestBody: map[string]any{
				"token_name": "Test Token",
			},
			setupAuth: func(ctx context.Context) context.Context {
				return auth.ContextWithSession(ctx, &auth.Session{
					Login: "testuser",
					Email: "test@example.com",
					User: &domain.User{
						ID:    1,
						Login: "testuser",
						Email: "test@example.com",
					},
				})
			},
			wantStatus: http.StatusUnprocessableEntity,
			wantErr:    "abilities field is required",
		},
		{
			name: "empty abilities array",
			requestBody: map[string]any{
				"token_name": "Test Token",
				"abilities":  []string{},
			},
			setupAuth: func(ctx context.Context) context.Context {
				return auth.ContextWithSession(ctx, &auth.Session{
					Login: "testuser",
					Email: "test@example.com",
					User: &domain.User{
						ID:    1,
						Login: "testuser",
						Email: "test@example.com",
					},
				})
			},
			wantStatus: http.StatusUnprocessableEntity,
			wantErr:    "at least one ability must be provided",
		},
		{
			name: "invalid ability",
			requestBody: map[string]any{
				"token_name": "Test Token",
				"abilities":  []string{"invalid:ability"},
			},
			setupAuth: func(ctx context.Context) context.Context {
				return auth.ContextWithSession(ctx, &auth.Session{
					Login: "testuser",
					Email: "test@example.com",
					User: &domain.User{
						ID:    1,
						Login: "testuser",
						Email: "test@example.com",
					},
				})
			},
			wantStatus: http.StatusUnprocessableEntity,
			wantErr:    "invalid ability provided",
		},
		{
			name: "duplicate abilities",
			requestBody: map[string]any{
				"token_name": "Test Token",
				"abilities": []string{
					"server:start",
					"server:stop",
					"server:start",
				},
			},
			setupAuth: func(ctx context.Context) context.Context {
				return auth.ContextWithSession(ctx, &auth.Session{
					Login: "testuser",
					Email: "test@example.com",
					User: &domain.User{
						ID:    1,
						Login: "testuser",
						Email: "test@example.com",
					},
				})
			},
			wantStatus: http.StatusUnprocessableEntity,
			wantErr:    "duplicate abilities are not allowed",
		},
		{
			name: "too many abilities",
			requestBody: map[string]any{
				"token_name": "Test Token",
				"abilities":  generateManyAbilities(101),
			},
			setupAuth: func(ctx context.Context) context.Context {
				return auth.ContextWithSession(ctx, &auth.Session{
					Login: "testuser",
					Email: "test@example.com",
					User: &domain.User{
						ID:    1,
						Login: "testuser",
						Email: "test@example.com",
					},
				})
			},
			wantStatus: http.StatusUnprocessableEntity,
			wantErr:    "too many abilities provided",
		},
		{
			name:        "invalid JSON",
			requestBody: "invalid json",
			setupAuth: func(ctx context.Context) context.Context {
				return auth.ContextWithSession(ctx, &auth.Session{
					Login: "testuser",
					Email: "test@example.com",
					User: &domain.User{
						ID:    1,
						Login: "testuser",
						Email: "test@example.com",
					},
				})
			},
			wantStatus: http.StatusBadRequest,
			wantErr:    "invalid request body",
		},
		{
			name:        "empty request body",
			requestBody: map[string]any{},
			setupAuth: func(ctx context.Context) context.Context {
				return auth.ContextWithSession(ctx, &auth.Session{
					Login: "testuser",
					Email: "test@example.com",
					User: &domain.User{
						ID:    1,
						Login: "testuser",
						Email: "test@example.com",
					},
				})
			},
			wantStatus: http.StatusUnprocessableEntity,
			wantErr:    "name field is required",
		},
		{
			name: "mixed valid and invalid abilities",
			requestBody: map[string]any{
				"token_name": "Test Token",
				"abilities": []string{
					"server:start",
					"invalid:ability",
					"server:stop",
				},
			},
			setupAuth: func(ctx context.Context) context.Context {
				return auth.ContextWithSession(ctx, &auth.Session{
					Login: "testuser",
					Email: "test@example.com",
					User: &domain.User{
						ID:    1,
						Login: "testuser",
						Email: "test@example.com",
					},
				})
			},
			wantStatus: http.StatusUnprocessableEntity,
			wantErr:    "invalid ability provided",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			tokenRepo := inmemory.NewPersonalAccessTokenRepository()
			rbacRepo := inmemory.NewRBACRepository()
			if tt.setupRBAC != nil {
				tt.setupRBAC(rbacRepo)
			}
			rbacService := rbac.NewRBAC(services.NewNilTransactionManager(), rbacRepo, 0)
			responder := api.NewResponder()

			handler := posttoken.NewHandler(tokenRepo, rbacService, responder, nil)

			var body []byte
			if str, ok := tt.requestBody.(string); ok {
				body = []byte(str)
			} else {
				var err error
				body, err = json.Marshal(tt.requestBody)
				require.NoError(t, err)
			}

			req := httptest.NewRequest(http.MethodPost, "/api/tokens", bytes.NewReader(body))
			req.Header.Set("Content-Type", "application/json")

			if tt.setupAuth != nil {
				req = req.WithContext(tt.setupAuth(req.Context()))
			}

			rr := httptest.NewRecorder()

			handler.ServeHTTP(rr, req)

			assert.Equal(t, tt.wantStatus, rr.Code)

			var response map[string]any
			err := json.Unmarshal(rr.Body.Bytes(), &response)
			require.NoError(t, err)

			//nolint:nestif
			if tt.wantErr != "" {
				if message, ok := response["message"].(string); ok {
					assert.Contains(t, message, tt.wantErr)
				} else if errorField, exists := response["error"].(map[string]any); exists {
					if msg, ok := errorField["message"].(string); ok {
						assert.Contains(t, msg, tt.wantErr)
					}
				} else {
					// Debug: print response when we can't find the error message
					t.Logf("Response: %+v", response)
				}
			} else if tt.validateResponse != nil {
				tt.validateResponse(t, response)
			}
		})
	}
}

func TestHandler_TokenUniqueness(t *testing.T) {
	tokenRepo := inmemory.NewPersonalAccessTokenRepository()
	rbacRepo := inmemory.NewRBACRepository()
	rbacService := rbac.NewRBAC(services.NewNilTransactionManager(), rbacRepo, 0)
	responder := api.NewResponder()

	handler := posttoken.NewHandler(tokenRepo, rbacService, responder, nil)

	ctx := auth.ContextWithSession(context.Background(), &auth.Session{
		Login: "testuser",
		Email: "test@example.com",
		User: &domain.User{
			ID:    1,
			Login: "testuser",
			Email: "test@example.com",
		},
	})

	generatedTokens := make(map[string]bool)

	for range 10 {
		body, err := json.Marshal(map[string]any{
			"token_name": "Test Token",
			"abilities":  []string{"server:start"},
		})
		require.NoError(t, err)

		req := httptest.NewRequest(http.MethodPost, "/api/tokens", bytes.NewReader(body))
		req.Header.Set("Content-Type", "application/json")
		req = req.WithContext(ctx)

		rr := httptest.NewRecorder()
		handler.ServeHTTP(rr, req)

		assert.Equal(t, http.StatusOK, rr.Code)

		var response map[string]any
		err = json.Unmarshal(rr.Body.Bytes(), &response)
		require.NoError(t, err)

		token, ok := response["token"].(string)
		require.True(t, ok)

		assert.False(t, generatedTokens[token], "Token should be unique")
		generatedTokens[token] = true
	}
}

func TestHandler_ConcurrentTokenCreation(t *testing.T) {
	tokenRepo := inmemory.NewPersonalAccessTokenRepository()
	rbacRepo := inmemory.NewRBACRepository()
	rbacService := rbac.NewRBAC(services.NewNilTransactionManager(), rbacRepo, 0)
	responder := api.NewResponder()

	handler := posttoken.NewHandler(tokenRepo, rbacService, responder, nil)

	ctx := auth.ContextWithSession(context.Background(), &auth.Session{
		Login: "testuser",
		Email: "test@example.com",
		User: &domain.User{
			ID:    1,
			Login: "testuser",
			Email: "test@example.com",
		},
	})

	const numGoroutines = 10
	done := make(chan bool, numGoroutines)

	for i := range numGoroutines {
		go func(_ int) {
			body, err := json.Marshal(map[string]any{
				"token_name": "Concurrent Token",
				"abilities":  []string{"server:start"},
			})
			require.NoError(t, err)

			req := httptest.NewRequest(http.MethodPost, "/api/tokens", bytes.NewReader(body))
			req.Header.Set("Content-Type", "application/json")
			req = req.WithContext(ctx)

			rr := httptest.NewRecorder()
			handler.ServeHTTP(rr, req)

			assert.Equal(t, http.StatusOK, rr.Code)
			done <- true
		}(i)
	}

	for range numGoroutines {
		<-done
	}

	tokens, err := tokenRepo.Find(ctx, nil, nil, nil)
	require.NoError(t, err)
	assert.Len(t, tokens, numGoroutines)
}

func TestHandler_TokenStorageVerification(t *testing.T) {
	tokenRepo := inmemory.NewPersonalAccessTokenRepository()
	rbacRepo := inmemory.NewRBACRepository()
	rbacService := rbac.NewRBAC(services.NewNilTransactionManager(), rbacRepo, 0)
	responder := api.NewResponder()

	handler := posttoken.NewHandler(tokenRepo, rbacService, responder, nil)

	ctx := auth.ContextWithSession(context.Background(), &auth.Session{
		Login: "testuser",
		Email: "test@example.com",
		User: &domain.User{
			ID:    1,
			Login: "testuser",
			Email: "test@example.com",
		},
	})

	body, err := json.Marshal(map[string]any{
		"token_name": "Storage Test Token",
		"abilities": []string{
			"server:start",
			"server:stop",
		},
	})
	require.NoError(t, err)

	req := httptest.NewRequest(http.MethodPost, "/api/tokens", bytes.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	req = req.WithContext(ctx)

	rr := httptest.NewRecorder()
	handler.ServeHTTP(rr, req)

	assert.Equal(t, http.StatusOK, rr.Code)

	var response map[string]any
	err = json.Unmarshal(rr.Body.Bytes(), &response)
	require.NoError(t, err)

	plainToken := response["token"].(string)
	sha256Token := sha256.Sum256([]byte(strings.TrimPrefix(plainToken, "1|")))

	tokens, err := tokenRepo.Find(ctx, &filters.FindPersonalAccessToken{
		Tokens: []string{hex.EncodeToString(sha256Token[:])},
	}, nil, nil)
	require.NoError(t, err)
	require.Len(t, tokens, 1)

	storedToken := tokens[0]
	assert.Equal(t, "Storage Test Token", storedToken.Name)
	assert.Equal(t, domain.EntityTypeUser, storedToken.TokenableType)
	assert.Equal(t, uint(1), storedToken.TokenableID)
	assert.NotEqual(t, plainToken, storedToken.Token)
	assert.Len(t, storedToken.Token, 64)
	assert.NotNil(t, storedToken.Abilities)
	assert.Len(t, *storedToken.Abilities, 2)
	assert.Nil(t, storedToken.LastUsedAt)
	assert.NotNil(t, storedToken.CreatedAt)
	assert.NotNil(t, storedToken.UpdatedAt)

	assert.WithinDuration(t, time.Now(), *storedToken.CreatedAt, 2*time.Second)
	assert.WithinDuration(t, time.Now(), *storedToken.UpdatedAt, 2*time.Second)
}

func generateManyAbilities(count int) []string {
	abilities := []string{"server:start", "server:stop"}
	for i := 2; i < count; i++ {
		abilities = append(abilities, abilities[i%2])
	}

	return abilities
}

// ---------------------------------------------------------------------------
// Security audit-trail tests.
//
// OWASP API Security Top 10:2023:
//   - API8:2023 Security Misconfiguration — issuing a personal access token
//     is a credential-issuance event that must be recorded (OWASP ASVS
//     §7.2.1) so token sprawl is auditable. The audit record must never
//     contain the plaintext token value (OWASP ASVS §7.1.1 redaction).
//
// Reference: https://owasp.org/API-Security/editions/2023/
// ---------------------------------------------------------------------------

// TestHandler_Audit_SuccessfulTokenCreationIsRecorded covers OWASP
// API8:2023. A successful PAT creation must emit exactly one token.pat.create
// event with outcome success, category token_op, the created token id as the
// resource, the acting user as the actor, and must NOT leak the plaintext
// token value into any audit field.
func TestHandler_Audit_SuccessfulTokenCreationIsRecorded(t *testing.T) {
	// ARRANGE
	tokenRepo := inmemory.NewPersonalAccessTokenRepository()
	rbacRepo := inmemory.NewRBACRepository()
	rbacService := rbac.NewRBAC(services.NewNilTransactionManager(), rbacRepo, 0)
	recorder := &auditCapture{}
	handler := posttoken.NewHandler(tokenRepo, rbacService, api.NewResponder(), recorder)

	body, err := json.Marshal(map[string]any{
		"token_name": "CI Token",
		"abilities":  []string{"server:start", "server:stop"},
	})
	require.NoError(t, err)

	req := httptest.NewRequest(http.MethodPost, "/api/tokens", bytes.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	req = req.WithContext(auth.ContextWithSession(context.Background(), &auth.Session{
		Login: "tokenuser",
		Email: "token@example.com",
		User:  &domain.User{ID: 7, Login: "tokenuser", Email: "token@example.com"},
	}))
	w := httptest.NewRecorder()

	// ACT
	handler.ServeHTTP(w, req)

	// ASSERT
	require.Equal(t, http.StatusOK, w.Code, "token creation must succeed; body=%s", w.Body.String())

	var response map[string]any
	require.NoError(t, json.Unmarshal(w.Body.Bytes(), &response))
	plainToken, ok := response["token"].(string)
	require.True(t, ok)
	require.NotEmpty(t, plainToken)

	events := recorder.snapshot()
	require.Equal(t, 1, countEvents(events, audit.EventPATCreate),
		"exactly one token.pat.create event must be emitted per successful creation")

	ev, ok := findEvent(events, audit.EventPATCreate)
	require.True(t, ok, "a successful PAT creation must leave a token.pat.create audit event")
	assert.Equal(t, audit.OutcomeSuccess, ev.Outcome, "a completed sensitive op records success")
	assert.Equal(t, audit.CategoryTokenOp, ev.Category)
	assert.Equal(t, "token", ev.ResourceType)
	assert.Equal(t, "create", ev.Action)
	assert.NotEmpty(t, ev.ResourceID, "the created token id must be recorded as resource_id")
	assert.Equal(t, uint(7), ev.ActorID, "the creating user must be attributed as the actor")
	assert.Equal(t, audit.AuthMethodSession, ev.AuthMethod)

	name, hasName := extraString(ev, "token_name")
	require.True(t, hasName, "the token name must be recorded for forensics")
	assert.Equal(t, "CI Token", name)

	// The plaintext credential must never appear anywhere in the audit record.
	assert.NotEqual(t, plainToken, ev.ResourceID,
		"resource_id must be the token id, never the secret value")
	tokenName, _ := extraString(ev, "token_name")
	assert.NotContains(t, tokenName, plainToken)
	for _, a := range ev.Extra {
		assert.NotContains(t, a.Value.String(), plainToken,
			"no Extra attr may contain the plaintext token value")
	}
}

// TestHandler_Audit_RejectedTokenCreationIsNotRecorded covers OWASP
// API8:2023. A creation rejected before persistence (token session) must NOT
// emit a token.pat.create event — the audit trail must not claim a token was
// issued when none was.
func TestHandler_Audit_RejectedTokenCreationIsNotRecorded(t *testing.T) {
	// ARRANGE
	tokenRepo := inmemory.NewPersonalAccessTokenRepository()
	rbacRepo := inmemory.NewRBACRepository()
	rbacService := rbac.NewRBAC(services.NewNilTransactionManager(), rbacRepo, 0)
	recorder := &auditCapture{}
	handler := posttoken.NewHandler(tokenRepo, rbacService, api.NewResponder(), recorder)

	body, err := json.Marshal(map[string]any{
		"token_name": "Nested Token",
		"abilities":  []string{"server:start"},
	})
	require.NoError(t, err)

	req := httptest.NewRequest(http.MethodPost, "/api/tokens", bytes.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	req = req.WithContext(auth.ContextWithSession(context.Background(), &auth.Session{
		Login: "tokenuser",
		Email: "token@example.com",
		User:  &domain.User{ID: 7, Login: "tokenuser"},
		Token: &domain.PersonalAccessToken{ID: 1},
	}))
	w := httptest.NewRecorder()

	// ACT
	handler.ServeHTTP(w, req)

	// ASSERT
	require.Equal(t, http.StatusForbidden, w.Code,
		"a token session must not be allowed to mint new tokens; body=%s", w.Body.String())
	assert.Equal(t, 0, countEvents(recorder.snapshot(), audit.EventPATCreate),
		"a rejected creation must not be recorded as a successful token.pat.create")
}
