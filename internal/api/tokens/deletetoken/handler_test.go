package deletetoken_test

import (
	"context"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/gameap/gameap/internal/api/tokens/deletetoken"
	"github.com/gameap/gameap/internal/domain"
	"github.com/gameap/gameap/internal/repositories/inmemory"
	"github.com/gameap/gameap/pkg/api"
	"github.com/gameap/gameap/pkg/auth"
	"github.com/gorilla/mux"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestHandler_ServeHTTP(t *testing.T) {
	now := time.Now()

	testUser := &domain.User{
		ID:    1,
		Login: "testuser",
		Email: "test@example.com",
	}

	testToken := &domain.PersonalAccessToken{
		ID:            0, // Will be set by repository
		TokenableType: domain.EntityTypeUser,
		TokenableID:   1,
		Name:          "Test Token",
		Token:         "test-token",
		Abilities:     &[]domain.PATAbility{},
		CreatedAt:     &now,
		UpdatedAt:     &now,
	}

	otherUserToken := &domain.PersonalAccessToken{
		ID:            0, // Will be set by repository
		TokenableType: domain.EntityTypeUser,
		TokenableID:   2,
		Name:          "Other User Token",
		Token:         "other-token",
		Abilities:     &[]domain.PATAbility{},
		CreatedAt:     &now,
		UpdatedAt:     &now,
	}

	tests := []struct {
		name          string
		tokenID       string
		setupTokens   []*domain.PersonalAccessToken
		authenticated bool
		wantStatus    int
		wantDeleted   bool
	}{
		{
			name:          "successful deletion",
			tokenID:       "1",
			setupTokens:   []*domain.PersonalAccessToken{testToken},
			authenticated: true,
			wantStatus:    http.StatusNoContent,
			wantDeleted:   true,
		},
		{
			name:          "unauthenticated user",
			tokenID:       "1",
			setupTokens:   []*domain.PersonalAccessToken{testToken},
			authenticated: false,
			wantStatus:    http.StatusUnauthorized,
			wantDeleted:   false,
		},
		{
			name:          "token not found",
			tokenID:       "999",
			setupTokens:   []*domain.PersonalAccessToken{testToken},
			authenticated: true,
			wantStatus:    http.StatusNotFound,
			wantDeleted:   false,
		},
		{
			name:          "access denied for other user's token",
			tokenID:       "2",
			setupTokens:   []*domain.PersonalAccessToken{testToken, otherUserToken},
			authenticated: true,
			wantStatus:    http.StatusForbidden,
			wantDeleted:   false,
		},
		{
			name:          "invalid token id",
			tokenID:       "invalid",
			setupTokens:   []*domain.PersonalAccessToken{testToken},
			authenticated: true,
			wantStatus:    http.StatusUnprocessableEntity,
			wantDeleted:   false,
		},
		{
			name:          "empty token id",
			tokenID:       "",
			setupTokens:   []*domain.PersonalAccessToken{testToken},
			authenticated: true,
			wantStatus:    http.StatusUnprocessableEntity,
			wantDeleted:   false,
		},
		{
			name:          "negative token id",
			tokenID:       "-1",
			setupTokens:   []*domain.PersonalAccessToken{testToken},
			authenticated: true,
			wantStatus:    http.StatusUnprocessableEntity,
			wantDeleted:   false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			tokensRepo := inmemory.NewPersonalAccessTokenRepository()
			responder := api.NewResponder()

			var savedTokens []*domain.PersonalAccessToken
			for _, token := range tt.setupTokens {
				tokenCopy := *token // Make a copy to preserve original state
				err := tokensRepo.Save(context.Background(), &tokenCopy)
				require.NoError(t, err)
				savedTokens = append(savedTokens, &tokenCopy)
			}

			handler := deletetoken.NewHandler(tokensRepo, responder)

			req := httptest.NewRequest(http.MethodDelete, "/api/tokens/"+tt.tokenID, nil)
			if tt.tokenID != "" {
				req = mux.SetURLVars(req, map[string]string{"id": tt.tokenID})
			}

			if tt.authenticated {
				ctx := auth.ContextWithSession(req.Context(), &auth.Session{
					User: testUser,
				})
				req = req.WithContext(ctx)
			}

			rr := httptest.NewRecorder()

			handler.ServeHTTP(rr, req)

			assert.Equal(t, tt.wantStatus, rr.Code)

			tokens, err := tokensRepo.Find(context.Background(), nil, nil, nil)
			require.NoError(t, err)

			//nolint:nestif
			if tt.wantDeleted {
				// For successful deletion, the first saved token should be gone
				if len(savedTokens) > 0 && savedTokens[0] != nil {
					firstTokenID := savedTokens[0].ID
					for _, token := range tokens {
						assert.NotEqual(t, firstTokenID, token.ID, "Token should be deleted")
					}
				}
			} else {
				// For cases where deletion shouldn't happen, check that the first saved token still exists
				if len(savedTokens) > 0 && savedTokens[0] != nil {
					firstTokenID := savedTokens[0].ID
					foundOriginal := false
					for _, token := range tokens {
						if token.ID == firstTokenID {
							foundOriginal = true

							break
						}
					}
					assert.True(t, foundOriginal, "Original token should still exist")
				}
			}
		})
	}
}
