package ws

import (
	"context"
	"errors"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/gameap/gameap/internal/domain"
	"github.com/gameap/gameap/pkg/api"
	"github.com/gameap/gameap/pkg/auth"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestSessionFromRequest(t *testing.T) {
	t.Parallel()
	authedSession := &auth.Session{
		ID:    "sess-1",
		Login: "tester",
		User:  &domain.User{ID: 7, Login: "tester"},
	}

	tests := []struct {
		name           string
		ctxSession     *auth.Session
		wantSession    *auth.Session
		wantError      string
		wantHTTPStatus int
	}{
		{
			name:        "authenticated_session",
			ctxSession:  authedSession,
			wantSession: authedSession,
		},
		{
			name:           "no_session_in_context",
			ctxSession:     nil,
			wantError:      "user not authenticated",
			wantHTTPStatus: http.StatusUnauthorized,
		},
		{
			name:           "session_without_user",
			ctxSession:     &auth.Session{ID: "sess-2"},
			wantError:      "user not authenticated",
			wantHTTPStatus: http.StatusUnauthorized,
		},
		{
			name:           "session_with_zero_user_id",
			ctxSession:     &auth.Session{User: &domain.User{ID: 0}},
			wantError:      "user not authenticated",
			wantHTTPStatus: http.StatusUnauthorized,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			req := httptest.NewRequest(http.MethodGet, "/api/ws/tasks/1", nil)
			if tt.ctxSession != nil {
				req = req.WithContext(auth.ContextWithSession(req.Context(), tt.ctxSession))
			}

			session, err := SessionFromRequest(req)

			if tt.wantError != "" {
				require.Error(t, err)
				assert.Nil(t, session)
				assert.Contains(t, err.Error(), tt.wantError)

				var wrapped *api.WrappedError
				require.True(t, errors.As(err, &wrapped), "error must be *api.WrappedError")
				assert.Equal(t, tt.wantHTTPStatus, wrapped.HTTPStatus())

				return
			}

			require.NoError(t, err)
			assert.Equal(t, tt.wantSession, session)
		})
	}
}

func TestSessionFromContext(t *testing.T) {
	t.Parallel()
	authedSession := &auth.Session{
		User: &domain.User{ID: 12, Login: "alice"},
	}

	tests := []struct {
		name        string
		ctxSession  *auth.Session
		wantSession *auth.Session
		wantError   string
	}{
		{
			name:        "authenticated_session",
			ctxSession:  authedSession,
			wantSession: authedSession,
		},
		{
			name:       "no_session_in_context",
			ctxSession: nil,
			wantError:  "user not authenticated",
		},
		{
			name:       "session_without_user",
			ctxSession: &auth.Session{},
			wantError:  "user not authenticated",
		},
		{
			name:       "session_with_zero_user_id",
			ctxSession: &auth.Session{User: &domain.User{ID: 0}},
			wantError:  "user not authenticated",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			ctx := context.Background()
			if tt.ctxSession != nil {
				ctx = auth.ContextWithSession(ctx, tt.ctxSession)
			}

			session, err := SessionFromContext(ctx)

			if tt.wantError != "" {
				require.Error(t, err)
				assert.Nil(t, session)
				assert.Contains(t, err.Error(), tt.wantError)

				return
			}

			require.NoError(t, err)
			assert.Equal(t, tt.wantSession, session)
		})
	}
}
