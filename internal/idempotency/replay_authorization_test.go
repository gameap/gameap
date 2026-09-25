package idempotency_test

import (
	"net/http"
	"sync/atomic"
	"testing"

	"github.com/gameap/gameap/internal/idempotency"
	"github.com/gameap/gameap/pkg/api"
	"github.com/pkg/errors"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

type authorizedReplayHandler struct {
	http.Handler

	authorize func(*http.Request) error
}

func (h authorizedReplayHandler) AuthorizeIdempotencyReplay(r *http.Request) error {
	return h.authorize(r)
}

func TestMiddleware_ReauthorizesBeforeReturningStoredHeadersAndBody(t *testing.T) {
	t.Parallel()

	var calls atomic.Int32
	checks := 0
	denied := false
	next := authorizedReplayHandler{
		Handler: createHandler(&calls),
		authorize: func(r *http.Request) error {
			checks++
			_, bounded := r.Context().Deadline()
			assert.True(t, bounded)
			if denied {
				return api.NewError(http.StatusForbidden, "server access revoked")
			}

			return nil
		},
	}
	h := newTestMiddleware(t, nil, nil).Middleware(next)
	request := func() *http.Request {
		return newKeyedRequest(http.MethodPost, "/api/servers/1/start", "start", "", 1)
	}

	first := serve(h, request())
	require.Equal(t, http.StatusCreated, first.Code)
	assert.Zero(t, checks, "initial requests use the handler's normal authorization")

	denied = true
	rejected := serve(h, request())
	assert.Equal(t, http.StatusForbidden, rejected.Code)
	assert.Empty(t, rejected.Header().Get(idempotency.HeaderReplayed))
	assert.Empty(t, rejected.Header().Get("Location"))
	assert.NotContains(t, rejected.Body.String(), "serverId")
	assert.Equal(t, 1, checks)

	denied = false
	restored := serve(h, request())
	assert.Equal(t, first.Body.String(), restored.Body.String())
	assert.Equal(t, "true", restored.Header().Get(idempotency.HeaderReplayed))
	assert.Equal(t, 2, checks)
	assert.Equal(t, int32(1), calls.Load(), "a denied replay must preserve the original outcome")
}

func TestMiddleware_replay_authorization_failures(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name        string
		refusal     error
		wantStatus  int
		wantMessage string
		wantRetry   string
	}{
		{
			name:        "refusal_is_passed_through",
			refusal:     api.NewNotFoundError("server not found"),
			wantStatus:  http.StatusNotFound,
			wantMessage: "server not found",
		},
		{
			name:        "check_that_cannot_run_is_unavailable",
			refusal:     errors.New("rbac backend is down"),
			wantStatus:  http.StatusServiceUnavailable,
			wantMessage: "Service Unavailable",
			wantRetry:   "5",
		},
		{
			name:        "server_error_status_is_unavailable",
			refusal:     api.WrapHTTPError(errors.New("failed to find server"), http.StatusInternalServerError),
			wantStatus:  http.StatusServiceUnavailable,
			wantMessage: "Service Unavailable",
			wantRetry:   "5",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			var (
				calls   atomic.Int32
				refused atomic.Bool
			)
			next := authorizedReplayHandler{
				Handler: createHandler(&calls),
				authorize: func(*http.Request) error {
					if refused.Load() {
						return tt.refusal
					}

					return nil
				},
			}
			h := newTestMiddleware(t, nil, nil).Middleware(next)
			request := func() *http.Request {
				return newKeyedRequest(http.MethodPost, "/api/servers/1/start", "start", "", 1)
			}

			first := serve(h, request())
			require.Equal(t, http.StatusCreated, first.Code)

			refused.Store(true)
			denied := serve(h, request())

			assert.Equal(t, tt.wantStatus, denied.Code)
			assert.Equal(t, tt.wantMessage, errorMessage(t, denied))
			assert.Equal(t, tt.wantRetry, denied.Header().Get("Retry-After"))
			assert.Empty(t, denied.Header().Get(idempotency.HeaderReplayed))
			assert.NotContains(t, denied.Body.String(), "serverId")

			refused.Store(false)
			replayed := serve(h, request())

			assert.Equal(t, http.StatusCreated, replayed.Code)
			assert.Equal(t, first.Body.String(), replayed.Body.String())
			assert.Equal(t, "true", replayed.Header().Get(idempotency.HeaderReplayed))
			assert.Equal(t, int32(1), calls.Load(), "a refused replay must not run the request")
		})
	}
}
