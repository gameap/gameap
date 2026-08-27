package getusers_test

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/gameap/gameap/internal/api/users/getusers"
	"github.com/gameap/gameap/internal/domain"
	"github.com/gameap/gameap/internal/repositories/inmemory"
	"github.com/gameap/gameap/internal/services"
	"github.com/gameap/gameap/pkg/api"
	"github.com/gameap/gameap/pkg/auth"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// The user list is how an external provisioner (a billing panel) resolves a
// customer to a panel account. Without a filter it would have to download
// every user on every order, so these cases guard the narrow lookups.
func TestGetUsersFiltering(t *testing.T) {
	t.Parallel()

	seed := []domain.User{
		{ID: 1, Login: "admin", Email: "admin@example.com"},
		{ID: 2, Login: "alice", Email: "alice@example.com"},
		{ID: 3, Login: "bob", Email: "bob@example.com"},
	}

	tests := []struct {
		name       string
		query      string
		wantLogins []string
		wantStatus int
	}{
		{
			name:       "no_filter_returns_everyone",
			query:      "",
			wantLogins: []string{"admin", "alice", "bob"},
		},
		{
			name:       "by_email",
			query:      "?filter[email]=alice@example.com",
			wantLogins: []string{"alice"},
		},
		{
			name:  "by_email_accepts_several_spellings",
			query: "?filter[email]=Alice@Example.com&filter[email]=alice@example.com",
			// Matching is exact, so a caller unsure about the stored casing
			// sends both spellings and gets the single stored account back.
			wantLogins: []string{"alice"},
		},
		{
			name:       "by_login",
			query:      "?filter[login]=bob",
			wantLogins: []string{"bob"},
		},
		{
			name:       "by_id",
			query:      "?filter[id]=1&filter[id]=3",
			wantLogins: []string{"admin", "bob"},
		},
		{
			name:       "filters_combine_with_and",
			query:      "?filter[login]=bob&filter[email]=alice@example.com",
			wantLogins: []string{},
		},
		{
			name:       "unknown_email_is_empty_not_an_error",
			query:      "?filter[email]=nobody@example.com",
			wantLogins: []string{},
		},
		{
			name:       "pagination_is_opt_in",
			query:      "?page[size]=2",
			wantLogins: []string{"admin", "alice"},
		},
		{
			name:       "pagination_second_page",
			query:      "?page[size]=2&page[number]=2",
			wantLogins: []string{"bob"},
		},
		{
			name:       "invalid_page_is_rejected",
			query:      "?page[size]=0",
			wantStatus: http.StatusBadRequest,
		},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			t.Parallel()

			// ARRANGE
			repo := inmemory.NewUserRepository()
			for _, user := range seed {
				userCopy := user
				require.NoError(t, repo.Save(context.Background(), &userCopy))
			}

			h := getusers.NewHandler(services.NewUserService(repo), api.NewResponder())
			recorder := httptest.NewRecorder()

			req := httptest.NewRequest(http.MethodGet, "/api/users"+test.query, nil)
			req = req.WithContext(auth.ContextWithSession(req.Context(), &auth.Session{
				User: &domain.User{ID: 1, Login: "admin"},
			}))

			// ACT
			h.ServeHTTP(recorder, req)

			// ASSERT
			if test.wantStatus != 0 {
				assert.Equal(t, test.wantStatus, recorder.Code)

				return
			}

			require.Equal(t, http.StatusOK, recorder.Code, recorder.Body.String())

			var got []struct {
				Login string `json:"login"`
			}
			require.NoError(t, json.Unmarshal(recorder.Body.Bytes(), &got))

			logins := make([]string, 0, len(got))
			for _, user := range got {
				logins = append(logins, user.Login)
			}

			assert.ElementsMatch(t, test.wantLogins, logins)
		})
	}
}
