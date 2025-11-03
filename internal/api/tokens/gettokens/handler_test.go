package gettokens

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/gameap/gameap/internal/domain"
	"github.com/gameap/gameap/internal/filters"
	"github.com/gameap/gameap/internal/repositories/inmemory"
	"github.com/gameap/gameap/pkg/api"
	"github.com/gameap/gameap/pkg/auth"
	"github.com/pkg/errors"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

var testUser1 = domain.User{
	ID:    1,
	Login: "testuser",
	Email: "test@example.com",
}

var testUser2 = domain.User{
	ID:    2,
	Login: "usernotokens",
	Email: "notokens@example.com",
}

func TestHandler_ServeHTTP(t *testing.T) {
	tests := []struct {
		name           string
		setupAuth      func() context.Context
		setupRepo      func(*inmemory.PersonalAccessTokenRepository)
		expectedStatus int
		wantError      string
		expectTokens   bool
		expectedCount  int
	}{
		{
			name: "successful tokens retrieval",
			setupAuth: func() context.Context {
				session := &auth.Session{
					Login: "testuser",
					Email: "test@example.com",
					User:  &testUser1,
				}

				return auth.ContextWithSession(context.Background(), session)
			},
			setupRepo: func(repo *inmemory.PersonalAccessTokenRepository) {
				abilities := []domain.PATAbility{
					domain.PATAbilityServerStart,
					domain.PATAbilityServerStop,
				}
				now := time.Now()

				tokens := []*domain.PersonalAccessToken{
					{
						ID:            0, // Let repository assign ID
						TokenableType: domain.EntityTypeUser,
						TokenableID:   1,
						Name:          "Test Token 1",
						Token:         "hash_token_1",
						Abilities:     &abilities,
						LastUsedAt:    &now,
						CreatedAt:     &now,
						UpdatedAt:     &now,
					},
					{
						ID:            0, // Let repository assign ID
						TokenableType: domain.EntityTypeUser,
						TokenableID:   1,
						Name:          "Test Token 2",
						Token:         "hash_token_2",
						Abilities:     &abilities,
						LastUsedAt:    nil,
						CreatedAt:     &now,
						UpdatedAt:     &now,
					},
				}

				for _, token := range tokens {
					_ = repo.Save(context.Background(), token)
				}
			},
			expectedStatus: http.StatusOK,
			expectTokens:   true,
			expectedCount:  2,
		},
		{
			name: "user with no tokens",
			setupAuth: func() context.Context {
				session := &auth.Session{
					Login: "usernotokens",
					Email: "notokens@example.com",
					User:  &testUser2,
				}

				return auth.ContextWithSession(context.Background(), session)
			},
			setupRepo: func(repo *inmemory.PersonalAccessTokenRepository) {
				abilities := []domain.PATAbility{
					domain.PATAbilityServerStart,
				}
				now := time.Now()

				token := &domain.PersonalAccessToken{
					ID:            0, // Let repository assign ID
					TokenableType: domain.EntityTypeUser,
					TokenableID:   1, // belongs to user 1, not user 2
					Name:          "Other User Token",
					Token:         "hash_token_other",
					Abilities:     &abilities,
					LastUsedAt:    &now,
					CreatedAt:     &now,
					UpdatedAt:     &now,
				}

				_ = repo.Save(context.Background(), token)
			},
			expectedStatus: http.StatusOK,
			expectTokens:   true,
			expectedCount:  0,
		},
		{
			name: "unauthenticated user",
			setupAuth: func() context.Context {
				session := &auth.Session{}

				return auth.ContextWithSession(context.Background(), session)
			},
			setupRepo:      func(_ *inmemory.PersonalAccessTokenRepository) {},
			expectedStatus: http.StatusUnauthorized,
			wantError:      "user not authenticated",
		},
		{
			name:           "no session context",
			setupRepo:      func(_ *inmemory.PersonalAccessTokenRepository) {},
			expectedStatus: http.StatusUnauthorized,
			wantError:      "user not authenticated",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			tokensRepo := inmemory.NewPersonalAccessTokenRepository()
			responder := api.NewResponder()
			handler := NewHandler(tokensRepo, responder)

			tt.setupRepo(tokensRepo)
			ctx := context.Background()

			if tt.setupAuth != nil {
				ctx = tt.setupAuth()
			}

			req := httptest.NewRequest(http.MethodGet, "/api/tokens", nil)
			req = req.WithContext(ctx)
			rr := httptest.NewRecorder()

			handler.ServeHTTP(rr, req)

			assert.Equal(t, tt.expectedStatus, rr.Code)

			if tt.wantError != "" {
				var response map[string]any
				require.NoError(t, json.Unmarshal(rr.Body.Bytes(), &response))
				assert.Equal(t, "error", response["status"])
				errorMsg, ok := response["error"].(string)
				require.True(t, ok)
				assert.Contains(t, errorMsg, tt.wantError)

				return
			}

			if tt.expectTokens {
				var tokens []tokenResponse
				err := json.Unmarshal(rr.Body.Bytes(), &tokens)
				require.NoError(t, err)
				assert.Len(t, tokens, tt.expectedCount)

				if tt.expectedCount > 0 {
					assert.NotEmpty(t, tokens[0].ID)
					assert.NotEmpty(t, tokens[0].Name)
					assert.NotNil(t, tokens[0].Abilities)
					assert.NotNil(t, tokens[0].CreatedAt)
				}
			}
		})
	}
}

func TestHandler_ServeHTTP_RepositoryError(t *testing.T) {
	tokensRepo := &mockPersonalAccessTokenRepository{
		shouldError: true,
	}
	responder := api.NewResponder()
	handler := NewHandler(tokensRepo, responder)

	session := &auth.Session{
		Login: "testuser",
		Email: "test@example.com",
		User:  &testUser1,
	}
	ctx := auth.ContextWithSession(context.Background(), session)

	req := httptest.NewRequest(http.MethodGet, "/api/tokens", nil)
	req = req.WithContext(ctx)
	rr := httptest.NewRecorder()

	handler.ServeHTTP(rr, req)

	assert.Equal(t, http.StatusInternalServerError, rr.Code)

	var response map[string]any
	require.NoError(t, json.Unmarshal(rr.Body.Bytes(), &response))
	assert.Equal(t, "error", response["status"])
	errorMsg, ok := response["error"].(string)
	require.True(t, ok)
	assert.Contains(t, errorMsg, "Internal Server Error")
}

type mockPersonalAccessTokenRepository struct {
	shouldError bool
}

func (m *mockPersonalAccessTokenRepository) Find(
	_ context.Context,
	_ *filters.FindPersonalAccessToken,
	_ []filters.Sorting,
	_ *filters.Pagination,
) ([]domain.PersonalAccessToken, error) {
	if m.shouldError {
		return nil, errors.New("repository error")
	}

	return []domain.PersonalAccessToken{}, nil
}

func (m *mockPersonalAccessTokenRepository) Save(context.Context, *domain.PersonalAccessToken) error {
	return nil
}

func (m *mockPersonalAccessTokenRepository) Delete(context.Context, uint) error {
	return nil
}

func (m *mockPersonalAccessTokenRepository) UpdateLastUsedAt(context.Context, uint, time.Time) error {
	return nil
}
